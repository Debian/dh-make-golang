package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"
	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/go/vcs"
)

func passthroughEnv() []string {
	var relevantVariables = []string{
		"HOME",
		"PATH",
		"HTTP_PROXY", "http_proxy",
		"HTTPS_PROXY", "https_proxy",
		"ALL_PROXY", "all_proxy",
		"NO_PROXY", "no_proxy",
		"GIT_PROXY_COMMAND",
		"GIT_HTTP_PROXY_AUTHMETHOD",
	}
	var result []string
	for _, variable := range relevantVariables {
		if value, ok := os.LookupEnv(variable); ok {
			result = append(result, fmt.Sprintf("%s=%s", variable, value))
		}
	}
	return result
}

func findVendorDirs(dir string) ([]string, error) {
	var vendorDirs []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info != nil && !info.IsDir() {
			return nil // nothing to do for anything but directories
		}
		if info.Name() == ".git" ||
			info.Name() == ".hg" ||
			info.Name() == ".bzr" {
			return filepath.SkipDir
		}
		if info.Name() == "vendor" {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			vendorDirs = append(vendorDirs, rel)
		}
		return nil
	})
	return vendorDirs, err
}

// upstream describes the upstream repo we are about to package.
type upstream struct {
	tarPath    string   // path to the generated orig tarball
	version    string   // Debian package version number, e.g. 0.0~git20180204.1d24609-1
	firstMain  string   // import path of the first main package within repo, if any
	vendorDirs []string // all vendor sub directories, relative to the repo directory
	repoDeps   []string // the repository paths of all dependencies (e.g. github.com/zyedidia/glob)
}

func (u *upstream) get(gopath, repo, rev string) error {
	done := make(chan struct{})
	defer close(done)
	go progressSize("go get", filepath.Join(gopath, "src"), done)

	rr, err := vcs.RepoRootForImportPath(repo, false)
	if err != nil {
		return err
	}
	dir := filepath.Join(gopath, "src", rr.Root)
	if rev != "" {
		return rr.VCS.CreateAtRev(dir, rr.Repo, rev)
	}
	return rr.VCS.Create(dir, rr.Repo)
}

func (u *upstream) tar(gopath, repo string) error {
	f, err := ioutil.TempFile("", "dh-make-golang")
	if err != nil {
		return err
	}
	u.tarPath = f.Name()
	f.Close()
	base := filepath.Base(repo)
	dir := filepath.Dir(repo)
	cmd := exec.Command(
		"tar",
		"cJf",
		u.tarPath,
		"--exclude=.git",
		"--exclude=Godeps",
		fmt.Sprintf("--exclude=%s/debian", base),
		base)
	cmd.Dir = filepath.Join(gopath, "src", dir)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// findMains finds main packages within the repo (useful to auto-detect the
// package type).
func (u *upstream) findMains(gopath, repo string) error {
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}} {{.Name}}", repo+"/...")
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		fmt.Sprintf("GOPATH=%s", gopath),
	}, passthroughEnv()...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("%v: %v", cmd.Args, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.Contains(line, "/vendor/") ||
			strings.Contains(line, "/Godeps/") ||
			strings.Contains(line, "/samples/") ||
			strings.Contains(line, "/examples/") ||
			strings.Contains(line, "/example/") {
			continue
		}
		if strings.HasSuffix(line, " main") {
			u.firstMain = strings.TrimSuffix(line, " main")
			break
		}
	}
	return nil
}

func (u *upstream) findDependencies(gopath, repo string) error {
	log.Printf("Determining dependencies\n")

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}\n{{join .TestImports \"\\n\"}}\n{{join .XTestImports \"\\n\"}}", repo+"/...")
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		fmt.Sprintf("GOPATH=%s", gopath),
	}, passthroughEnv()...)

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("%v: %v", cmd.Args, err)
	}

	godependencies := make(map[string]bool)
	for _, p := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if p == "" {
			continue // skip separators between import types
		}
		// Strip packages that are included in the repository we are packaging.
		if strings.HasPrefix(p, repo+"/") || p == repo {
			continue
		}
		if p == "C" {
			// TODO: maybe parse the comments to figure out C deps from pkg-config files?
		} else {
			godependencies[p] = true
		}
	}

	if len(godependencies) == 0 {
		return nil
	}

	// Remove all packages which are in the standard lib.
	cmd = exec.Command("go", "list", "std")
	cmd.Dir = filepath.Join(gopath, "src", repo)
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		fmt.Sprintf("GOPATH=%s", gopath),
	}, passthroughEnv()...)

	out, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("%v: %v", cmd.Args, err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		delete(godependencies, line)
	}

	// Resolve all packages to the root of their repository.
	roots := make(map[string]bool)
	for dep := range godependencies {
		rr, err := vcs.RepoRootForImportPath(dep, false)
		if err != nil {
			log.Printf("Could not determine repo path for import path %q: %v\n", dep, err)
			continue
		}

		roots[rr.Root] = true
	}

	u.repoDeps = make([]string, 0, len(godependencies))
	for root := range roots {
		u.repoDeps = append(u.repoDeps, root)
	}

	return nil
}

func makeUpstreamSourceTarball(repo, revision string) (*upstream, error) {
	gopath, err := ioutil.TempDir("", "dh-make-golang")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(gopath)
	repoDir := filepath.Join(gopath, "src", repo)

	var u upstream

	log.Printf("Downloading %q\n", repo+"/...")
	if err := u.get(gopath, repo, revision); err != nil {
		return nil, err
	}

	// Verify early this repository uses git (we call pkgVersionFromGit later):
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
		return nil, fmt.Errorf("Not a git repository, dh-make-golang currently only supports git")
	}

	if _, err := os.Stat(filepath.Join(repoDir, "debian")); err == nil {
		log.Printf("WARNING: ignoring debian/ directory that came with the upstream sources\n")
	}

	u.vendorDirs, err = findVendorDirs(repoDir)
	if err != nil {
		return nil, err
	}
	if len(u.vendorDirs) > 0 {
		log.Printf("Deleting upstream vendor/ directories")
		for _, dir := range u.vendorDirs {
			if err := os.RemoveAll(filepath.Join(repoDir, dir)); err != nil {
				return nil, err
			}
		}
	}

	log.Printf("Determining upstream version number\n")

	u.version, err = pkgVersionFromGit(repoDir)
	if err != nil {
		return nil, err
	}

	log.Printf("Package version is %q\n", u.version)

	if err := u.findMains(gopath, repo); err != nil {
		return nil, err
	}

	if err := u.findDependencies(gopath, repo); err != nil {
		return nil, err
	}

	if err := u.tar(gopath, repo); err != nil {
		return nil, err
	}

	return &u, nil
}

func runGitCommandIn(dir string, arg ...string) error {
	cmd := exec.Command("git", arg...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func createGitRepository(debsrc, gopkg, orig string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(wd, debsrc)
	if err := os.Mkdir(dir, 0755); err != nil {
		return "", err
	}

	if err := runGitCommandIn(dir, "init"); err != nil {
		return dir, err
	}

	if debianName := getDebianName(); debianName != "TODO" {
		if err := runGitCommandIn(dir, "config", "user.name", debianName); err != nil {
			return dir, err
		}
	}

	if debianEmail := getDebianEmail(); debianEmail != "TODO" {
		if err := runGitCommandIn(dir, "config", "user.email", debianEmail); err != nil {
			return dir, err
		}
	}

	if err := runGitCommandIn(dir, "config", "push.default", "matching"); err != nil {
		return dir, err
	}

	if err := runGitCommandIn(dir, "config", "--add", "remote.origin.push", "+refs/heads/*:refs/heads/*"); err != nil {
		return dir, err
	}

	if err := runGitCommandIn(dir, "config", "--add", "remote.origin.push", "+refs/tags/*:refs/tags/*"); err != nil {
		return dir, err
	}

	cmd := exec.Command("gbp", "import-orig", "--pristine-tar", "--no-interactive", filepath.Join(wd, orig))
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return dir, err
	}

	{
		f, err := os.OpenFile(filepath.Join(dir, ".gitignore"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return dir, err
		}
		// Beginning newline in case the file already exists and lacks a newline
		// (not all editors enforce a newline at the end of the file):
		if _, err := f.Write([]byte("\n.pc\n")); err != nil {
			return dir, err
		}
		if err := f.Close(); err != nil {
			return dir, err
		}
	}

	if err := runGitCommandIn(dir, "add", ".gitignore"); err != nil {
		return dir, err
	}

	if err := runGitCommandIn(dir, "commit", "-m", "Ignore quilt dir .pc via .gitignore"); err != nil {
		return dir, err
	}

	return dir, nil
}

// normalize program/source name into Debian standard[1]
// https://www.debian.org/doc/debian-policy/ch-controlfields.html#s-f-Source
// Package names (both source and binary, see Package, Section 5.6.7) must
// consist only of lower case letters (a-z), digits (0-9), plus (+) and minus
// (-) signs, and periods (.). They must be at least two characters long and
// must start with an alphanumeric character.
func normalizeDebianProgramName(str string) string {
	lowerDigitPlusMinusDot := func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z' || '0' <= r && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r == '.' || r == '+' || r == '-':
			return r
		case r == '_':
			return '-'
		}
		return -1
	}

	safe := strings.Trim(strings.Map(lowerDigitPlusMinusDot, str), "-")
	if len(safe) < 2 {
		return "TODO"
	}

	return safe
}

// This follows https://fedoraproject.org/wiki/PackagingDrafts/Go#Package_Names
func debianNameFromGopkg(gopkg, t string, allowUnknownHoster bool) string {
	parts := strings.Split(gopkg, "/")

	if t == "program" {
		return normalizeDebianProgramName(parts[len(parts)-1])
	}

	host := parts[0]
	if host == "github.com" {
		host = "github"
	} else if host == "code.google.com" {
		host = "googlecode"
	} else if host == "cloud.google.com" {
		host = "googlecloud"
	} else if host == "gopkg.in" {
		host = "gopkg"
	} else if host == "golang.org" {
		host = "golang"
	} else if host == "google.golang.org" {
		host = "google"
	} else if host == "bitbucket.org" {
		host = "bitbucket"
	} else if host == "bazil.org" {
		host = "bazil"
	} else if host == "pault.ag" {
		host = "pault"
	} else if host == "howett.net" {
		host = "howett"
	} else if host == "go4.org" {
		host = "go4"
	} else {
		if allowUnknownHoster {
			suffix, _ := publicsuffix.PublicSuffix(host)
			host = host[:len(host)-len(suffix)-len(".")]
			log.Printf("WARNING: Using %q as canonical hostname for %q. If that is not okay, please file a bug against %s.\n", host, parts[0], os.Args[0])
		} else {
			log.Fatalf("Cannot derive Debian package name: unknown hoster %q. See -help output for -allow_unknown_hoster\n", host)
		}
	}
	parts[0] = host
	return strings.Trim("golang-"+strings.ToLower(strings.Replace(strings.Join(parts, "-"), "_", "-", -1)), "-")
}

func getDebianName() string {
	if name := strings.TrimSpace(os.Getenv("DEBFULLNAME")); name != "" {
		return name
	}
	if name := strings.TrimSpace(os.Getenv("DEBNAME")); name != "" {
		return name
	}
	if u, err := user.Current(); err == nil && u.Name != "" {
		return u.Name
	}
	return "TODO"
}

func getDebianEmail() string {
	if email := strings.TrimSpace(os.Getenv("DEBEMAIL")); email != "" {
		return email
	}
	mailname, err := ioutil.ReadFile("/etc/mailname")
	// By default, /etc/mailname contains "debian" which is not useful; check for ".".
	if err == nil && strings.Contains(string(mailname), ".") {
		if u, err := user.Current(); err == nil && u.Username != "" {
			return u.Username + "@" + strings.TrimSpace(string(mailname))
		}
	}
	return "TODO"
}

func writeTemplates(dir, gopkg, debsrc, debbin, debversion, pkgType string, dependencies []string, vendorDirs []string) error {
	if err := os.Mkdir(filepath.Join(dir, "debian"), 0755); err != nil {
		return err
	}

	if err := os.Mkdir(filepath.Join(dir, "debian", "source"), 0755); err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(dir, "debian", "changelog"))
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "%s (%s) UNRELEASED; urgency=medium\n", debsrc, debversion)
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "  * Initial release (Closes: TODO)\n")
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, " -- %s <%s>  %s\n",
		getDebianName(),
		getDebianEmail(),
		time.Now().Format("Mon, 02 Jan 2006 15:04:05 -0700"))

	f, err = os.Create(filepath.Join(dir, "debian", "compat"))
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "11\n")

	f, err = os.Create(filepath.Join(dir, "debian", "control"))
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "Source: %s\n", debsrc)
	// TODO: change this once we have a “golang” section.
	fmt.Fprintf(f, "Section: devel\n")
	fmt.Fprintf(f, "Priority: optional\n")
	fmt.Fprintf(f, "Maintainer: Debian Go Packaging Team <pkg-go-maintainers@lists.alioth.debian.org>\n")
	fmt.Fprintf(f, "Uploaders: %s <%s>\n", getDebianName(), getDebianEmail())
	sort.Strings(dependencies)
	builddeps := append([]string{"debhelper (>= 11)", "dh-golang", "golang-any"}, dependencies...)
	fmt.Fprintf(f, "Build-Depends: %s\n", strings.Join(builddeps, ",\n               "))
	fmt.Fprintf(f, "Standards-Version: 4.1.3\n")
	fmt.Fprintf(f, "Homepage: %s\n", getHomepageForGopkg(gopkg))
	fmt.Fprintf(f, "Vcs-Browser: https://anonscm.debian.org/cgit/pkg-go/packages/%s.git\n", debsrc)
	fmt.Fprintf(f, "Vcs-Git: https://anonscm.debian.org/git/pkg-go/packages/%s.git\n", debsrc)
	fmt.Fprintf(f, "XS-Go-Import-Path: %s\n", gopkg)
	fmt.Fprintf(f, "Testsuite: autopkgtest-pkg-go\n")
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Package: %s\n", debbin)
	deps := []string{"${misc:Depends}"}
	if pkgType == "program" {
		fmt.Fprintf(f, "Architecture: any\n")
		fmt.Fprintf(f, "Built-Using: ${misc:Built-Using}\n")
		deps = append(deps, "${shlibs:Depends}")
	} else {
		fmt.Fprintf(f, "Architecture: all\n")
		deps = append(deps, dependencies...)
	}
	fmt.Fprintf(f, "Depends: %s\n", strings.Join(deps, ",\n         "))
	description, err := getDescriptionForGopkg(gopkg)
	if err != nil {
		log.Printf("Could not determine description for %q: %v\n", gopkg, err)
		description = "TODO: short description"
	}
	fmt.Fprintf(f, "Description: %s\n", description)
	longdescription, err := getLongDescriptionForGopkg(gopkg)
	if err != nil {
		log.Printf("Could not determine long description for %q: %v\n", gopkg, err)
		longdescription = "TODO: long description"
	}
	fmt.Fprintf(f, " %s\n", longdescription)

	license, fulltext, err := getLicenseForGopkg(gopkg)
	if err != nil {
		log.Printf("Could not determine license for %q: %v\n", gopkg, err)
		license = "TODO"
		fulltext = "TODO"
	}
	f, err = os.Create(filepath.Join(dir, "debian", "copyright"))
	if err != nil {
		return err
	}
	defer f.Close()
	_, copyright, err := getAuthorAndCopyrightForGopkg(gopkg)
	if err != nil {
		log.Printf("Could not determine copyright for %q: %v\n", gopkg, err)
		copyright = "TODO"
	}
	fmt.Fprintf(f, "Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/\n")
	fmt.Fprintf(f, "Upstream-Name: %s\n", filepath.Base(gopkg))
	fmt.Fprintf(f, "Source: %s\n", getHomepageForGopkg(gopkg))
	fmt.Fprintf(f, "Files-Excluded:\n")
	for _, dir := range vendorDirs {
		fmt.Fprintf(f, "  %s\n", dir)
	}
	fmt.Fprintf(f, "  Godeps/_workspace\n")
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Files: *\n")
	fmt.Fprintf(f, "Copyright: %s\n", copyright)
	fmt.Fprintf(f, "License: %s\n", license)
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Files: debian/*\n")
	fmt.Fprintf(f, "Copyright: %s %s <%s>\n", time.Now().Format("2006"), getDebianName(), getDebianEmail())
	fmt.Fprintf(f, "License: %s\n", license)
	fmt.Fprintf(f, "Comment: Debian packaging is licensed under the same terms as upstream\n")
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "License: %s\n", license)
	fmt.Fprintf(f, fulltext)

	f, err = os.Create(filepath.Join(dir, "debian", "rules"))
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "#!/usr/bin/make -f\n")
	fmt.Fprintf(f, "\n")
	if pkgType == "program" {
		fmt.Fprintf(f, "override_dh_auto_install:\n")
		fmt.Fprintf(f, "\tdh_auto_install -- --no-source\n")
	}
	fmt.Fprintf(f, "%%:\n")
	fmt.Fprintf(f, "\tdh $@ --buildsystem=golang --with=golang\n")

	f, err = os.Create(filepath.Join(dir, "debian", "source", "format"))
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "3.0 (quilt)\n")

	f, err = os.Create(filepath.Join(dir, "debian", "gbp.conf"))
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "[DEFAULT]\n")
	fmt.Fprintf(f, "pristine-tar = True\n")

	if err := os.Chmod(filepath.Join(dir, "debian", "rules"), 0755); err != nil {
		return err
	}

	if strings.HasPrefix(gopkg, "github.com/") {
		f, err = os.Create(filepath.Join(dir, "debian", "watch"))
		if err != nil {
			return err
		}
		defer f.Close()
		fmt.Fprintf(f, "version=3\n")
		fmt.Fprintf(f, `opts=filenamemangle=s/.+\/v?(\d\S*)\.tar\.gz/%s-\$1\.tar\.gz/,\`+"\n", debsrc)
		fmt.Fprintf(f, `uversionmangle=s/(\d)[_\.\-\+]?(RC|rc|pre|dev|beta|alpha)[.]?(\d*)$/\$1~\$2\$3/ \`+"\n")
		fmt.Fprintf(f, `  https://%s/tags .*/v?(\d\S*)\.tar\.gz`+"\n", gopkg)
	}

	return nil
}

func writeITP(gopkg, debsrc, debversion string) (string, error) {
	itpname := fmt.Sprintf("itp-%s.txt", debsrc)
	f, err := os.Create(itpname)
	if err != nil {
		return itpname, err
	}
	defer f.Close()

	// TODO: memoize
	license, _, err := getLicenseForGopkg(gopkg)
	if err != nil {
		log.Printf("Could not determine license for %q: %v\n", gopkg, err)
		license = "TODO"
	}

	author, _, err := getAuthorAndCopyrightForGopkg(gopkg)
	if err != nil {
		log.Printf("Could not determine author for %q: %v\n", gopkg, err)
		author = "TODO"
	}

	description, err := getDescriptionForGopkg(gopkg)
	if err != nil {
		log.Printf("Could not determine description for %q: %v\n", gopkg, err)
		description = "TODO"
	}

	fmt.Fprintf(f, "From: %q <%s>\n", getDebianName(), getDebianEmail())
	fmt.Fprintf(f, "To: submit@bugs.debian.org\n")
	fmt.Fprintf(f, "Subject: ITP: %s -- %s\n", debsrc, description)
	fmt.Fprintf(f, "Content-Type: text/plain; charset=utf-8\n")
	fmt.Fprintf(f, "Content-Transfer-Encoding: 8bit\n")
	fmt.Fprintf(f, "X-Debbugs-CC: debian-devel@lists.debian.org, pkg-go-maintainers@lists.alioth.debian.org\n")
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Package: wnpp\n")
	fmt.Fprintf(f, "Severity: wishlist\n")
	fmt.Fprintf(f, "Owner: %s <%s>\n", getDebianName(), getDebianEmail())
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "* Package name    : %s\n", debsrc)
	fmt.Fprintf(f, "  Version         : %s\n", debversion)
	fmt.Fprintf(f, "  Upstream Author : %s\n", author)
	fmt.Fprintf(f, "* URL             : %s\n", getHomepageForGopkg(gopkg))
	fmt.Fprintf(f, "* License         : %s\n", license)
	fmt.Fprintf(f, "  Programming Lang: Go\n")
	fmt.Fprintf(f, "  Description     : %s\n", description)
	fmt.Fprintf(f, "\n")

	longdescription, err := getLongDescriptionForGopkg(gopkg)
	if err != nil {
		log.Printf("Could not determine long description for %q: %v\n", gopkg, err)
		longdescription = "TODO: long description"
	}
	fmt.Fprintf(f, " %s\n", longdescription)

	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "TODO: perhaps reasoning\n")
	return itpname, nil
}

func copyFile(src, dest string) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return output.Close()
}

func execMake(args []string, usage func()) {
	fs := flag.NewFlagSet("make", flag.ExitOnError)
	if usage != nil {
		fs.Usage = usage
	} else {
		fs.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: %s [make] <go-package-importpath>\n", os.Args[0])
			fmt.Fprintf(os.Stderr, "Example: %s make golang.org/x/oauth2\n", os.Args[0])
			fmt.Fprintf(os.Stderr, "\n")
			fmt.Fprintf(os.Stderr, "%s will create new files and directories in the current working directory.\n", os.Args[0])
			fmt.Fprintf(os.Stderr, "%s will connect to the internet to download the specified Go package.\n", os.Args[0])
			fmt.Fprintf(os.Stderr, "\n")
			fmt.Fprintf(os.Stderr, "Flags:\n")
			fs.PrintDefaults()
		}
	}

	var gitRevision string
	fs.StringVar(&gitRevision,
		"git_revision",
		"",
		"git revision (see gitrevisions(7)) of the specified Go package to check out, defaulting to the default behavior of git clone. Useful in case you do not want to package e.g. current HEAD.")

	var allowUnknownHoster bool
	fs.BoolVar(&allowUnknownHoster,
		"allow_unknown_hoster",
		false,
		"The pkg-go naming conventions (see https://pkg-go.alioth.debian.org/packaging.html) use a canonical identifier for the hostname, and the mapping is hardcoded into dh-make-golang. In case you want to package a Go package living on an unknown hoster, you may set this flag to true and double-check that the resulting package name is sane. Contact pkg-go if unsure.")

	var pkgType string
	fs.StringVar(&pkgType,
		"type",
		"",
		"One of \"library\" or \"program\"")

	err := fs.Parse(args)
	if err != nil {
		log.Fatal(err)
	}

	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	gitRevision = strings.TrimSpace(gitRevision)
	gopkg := fs.Arg(0)

	// Ensure the specified argument is a Go package import path.
	rr, err := vcs.RepoRootForImportPath(gopkg, false)
	if err != nil {
		log.Fatalf("Verifying arguments: %v — did you specify a Go package import path?", err)
	}
	if gopkg != rr.Root {
		log.Printf("Continuing with repository root %q instead of specified import path %q (repositories are the unit of packaging in Debian)", rr.Root, gopkg)
		gopkg = rr.Root
	}

	debsrc := debianNameFromGopkg(gopkg, "library", allowUnknownHoster)

	if strings.TrimSpace(pkgType) != "" {
		debsrc = debianNameFromGopkg(gopkg, pkgType, allowUnknownHoster)
		if _, err := os.Stat(debsrc); err == nil {
			log.Fatalf("Output directory %q already exists, aborting\n", debsrc)
		}
	}

	if strings.ToLower(gopkg) != gopkg {
		// Without -git_revision, specifying the package name in the wrong case
		// will lead to two checkouts, i.e. wasting bandwidth. With
		// -git_revision, packaging might fail.
		//
		// In case it turns out that Go package names should never contain any
		// uppercase letters, we can just auto-convert the argument.
		log.Printf("WARNING: Go package names are case-sensitive. Did you really mean %q instead of %q?\n",
			gopkg, strings.ToLower(gopkg))
	}

	var (
		eg             errgroup.Group
		golangBinaries map[string]string // map[goImportPath]debianBinaryPackage
	)

	// TODO: also check whether there already is a git repository on salsa.
	eg.Go(func() error {
		var err error
		golangBinaries, err = getGolangBinaries()
		return err
	})
	u, err := makeUpstreamSourceTarball(gopkg, gitRevision)
	if err != nil {
		log.Fatalf("Could not create a tarball of the upstream source: %v\n", err)
	}

	if strings.TrimSpace(pkgType) == "" {
		if u.firstMain != "" {
			log.Printf("Assuming you are packaging a program (because %q defines a main package), use -type to override\n", u.firstMain)
			pkgType = "program"
			debsrc = debianNameFromGopkg(gopkg, pkgType, allowUnknownHoster)
		} else {
			pkgType = "library"
		}
	}
	debbin := debsrc + "-dev"
	if pkgType == "program" {
		debbin = debsrc
	}

	if _, err := os.Stat(debsrc); err == nil {
		log.Fatalf("Output directory %q already exists, aborting\n", debsrc)
	}

	if err := eg.Wait(); err != nil {
		log.Printf("Could not check for existing Go packages in Debian: %v", err)
	}

	if debbin, ok := golangBinaries[gopkg]; ok {
		log.Printf("WARNING: A package called %q is already in Debian! See https://tracker.debian.org/pkg/%s\n",
			debbin, debbin)
	}

	orig := fmt.Sprintf("%s_%s.orig.tar.xz", debsrc, u.version)
	// We need to copy the file, merely renaming is not enough since the file
	// might be on a different filesystem (/tmp often is a tmpfs).
	if err := copyFile(u.tarPath, orig); err != nil {
		log.Fatalf("Could not rename orig tarball from %q to %q: %v\n", u.tarPath, orig, err)
	}
	if err := os.Remove(u.tarPath); err != nil {
		log.Printf("Could not remove tempfile %q: %v\n", u.tarPath, err)
	}

	debversion := u.version + "-1"

	dir, err := createGitRepository(debsrc, gopkg, orig)
	if err != nil {
		log.Fatalf("Could not create git repository: %v\n", err)
	}

	debdependencies := make([]string, 0, len(u.repoDeps))
	for _, dep := range u.repoDeps {
		if len(golangBinaries) == 0 {
			// fall back to heuristic
			debdependencies = append(debdependencies, debianNameFromGopkg(dep, "library", allowUnknownHoster)+"-dev")
			continue
		}
		bin, ok := golangBinaries[dep]
		if !ok {
			log.Printf("Build-Dependency %q is not yet available in Debian, or has not yet been converted to use XS-Go-Import-Path in debian/control", dep)
			continue
		}
		debdependencies = append(debdependencies, bin)
	}

	if err := writeTemplates(dir, gopkg, debsrc, debbin, debversion, pkgType, debdependencies, u.vendorDirs); err != nil {
		log.Fatalf("Could not create debian/ from templates: %v\n", err)
	}

	itpname, err := writeITP(gopkg, debsrc, debversion)
	if err != nil {
		log.Fatalf("Could not write ITP email: %v\n", err)
	}

	log.Printf("\n")
	log.Printf("Packaging successfully created in %s\n", dir)
	log.Printf("\n")
	log.Printf("Resolve all TODOs in %s, then email it out:\n", itpname)
	log.Printf("    sendmail -t < %s\n", itpname)
	log.Printf("\n")
	log.Printf("Resolve all the TODOs in debian/, find them using:\n")
	log.Printf("    grep -r TODO debian\n")
	log.Printf("\n")
	log.Printf("To build the package, commit the packaging and use gbp buildpackage:\n")
	log.Printf("    git add debian && git commit -a -m 'Initial packaging'\n")
	log.Printf("    gbp buildpackage --git-pbuilder\n")
	log.Printf("\n")
	log.Printf("To create the packaging git repository on salsa, use:\n")
	log.Printf("    dh-make-golang create-salsa-project %s", debsrc)
	log.Printf("\n")
	log.Printf("Once you are happy with your packaging, push it to salsa using:\n")
	log.Printf("    git remote set-url origin git@salsa.debian.org:go-team/packages/%s.git\n", debsrc)
	log.Printf("    gbp push\n")
}
