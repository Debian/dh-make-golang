package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"
	"golang.org/x/tools/go/vcs"
)

const (
	golangBinariesURL = "https://people.debian.org/~stapelberg/dh-make-golang/binary-amd64-grep-golang"
)

var (
	gitRevision = flag.String("git_revision",
		"",
		"git revision (see gitrevisions(7)) of the specified Go package to check out, defaulting to the default behavior of git clone. Useful in case you do not want to package e.g. current HEAD.")

	allowUnknownHoster = flag.Bool("allow_unknown_hoster",
		false,
		"The pkg-go naming conventions (see http://pkg-go.alioth.debian.org/packaging.html) use a canonical identifier for the hostname, and the mapping is hardcoded into dh-make-golang. In case you want to package a Go package living on an unknown hoster, you may set this flag to true and double-check that the resulting package name is sane. Contact pkg-go if unsure.")

	pkgType = flag.String("type",
		"",
		"One of \"library\" or \"program\"")
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

// TODO: refactor this function into multiple smaller ones. Currently all the
// code is in this function only due to the os.RemoveAll(tempdir).
func makeUpstreamSourceTarball(gopkg string) (string, string, map[string]bool, string, error) {
	// dependencies is a map in order to de-duplicate multiple imports
	// pointing into the same repository.
	dependencies := make(map[string]bool)
	autoPkgType := "library"

	tempdir, err := ioutil.TempDir("", "dh-make-golang")
	if err != nil {
		return "", "", dependencies, autoPkgType, err
	}
	defer os.RemoveAll(tempdir)

	log.Printf("Downloading %q\n", gopkg+"/...")
	done := make(chan bool)
	go progressSize("go get", filepath.Join(tempdir, "src"), done)

	// As per https://groups.google.com/forum/#!topic/golang-nuts/N5apfenE4m4,
	// the arguments to “go get” are packages, not repositories. Hence, we
	// specify “gopkg/...” in order to cover all packages.
	// As a concrete example, github.com/jacobsa/util is a repository we want
	// to package into a single Debian package, and using “go get -d
	// github.com/jacobsa/util” fails because there are no buildable go files
	// in the top level of that repository.
	cmd := exec.Command("go", "get", "-d", "-t", gopkg+"/...")
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		fmt.Sprintf("GOPATH=%s", tempdir),
	}, passthroughEnv()...)

	if err := cmd.Run(); err != nil {
		done <- true
		return "", "", dependencies, autoPkgType, err
	}
	done <- true
	fmt.Printf("\r")

	revision := strings.TrimSpace(*gitRevision)
	if revision != "" {
		log.Printf("Checking out git revision %q\n", revision)
		if err := runGitCommandIn(filepath.Join(tempdir, "src", gopkg), "reset", "--hard", *gitRevision); err != nil {
			log.Fatalf("Could not check out git revision %q: %v\n", revision, err)
		}

		log.Printf("Refreshing %q\n", gopkg+"/...")
		done := make(chan bool)
		go progressSize("go get", filepath.Join(tempdir, "src"), done)

		cmd := exec.Command("go", "get", "-d", "-t", gopkg+"/...")
		cmd.Stderr = os.Stderr
		cmd.Env = append([]string{
			fmt.Sprintf("GOPATH=%s", tempdir),
		}, passthroughEnv()...)

		if err := cmd.Run(); err != nil {
			done <- true
			return "", "", dependencies, autoPkgType, err
		}
		done <- true
		fmt.Printf("\r")
	}

	if _, err := os.Stat(filepath.Join(tempdir, "src", gopkg, "debian")); err == nil {
		log.Printf("WARNING: ignoring debian/ directory that came with the upstream sources\n")
	}

	vendorpath := filepath.Join(tempdir, "src", gopkg, "vendor")
	if fi, err := os.Stat(vendorpath); err == nil && fi.IsDir() {
		log.Printf("Deleting upstream vendor/ directory, installing remaining dependencies")
		if err := os.RemoveAll(vendorpath); err != nil {
			return "", "", dependencies, autoPkgType, err
		}
		done := make(chan bool)
		go progressSize("go get", filepath.Join(tempdir, "src"), done)
		cmd := exec.Command("go", "get", "-d", "-t", "./...")
		cmd.Stderr = os.Stderr
		cmd.Env = append([]string{
			fmt.Sprintf("GOPATH=%s", tempdir),
		}, passthroughEnv()...)
		cmd.Dir = filepath.Join(tempdir, "src", gopkg)
		if err := cmd.Run(); err != nil {
			done <- true
			return "", "", dependencies, autoPkgType, err
		}
		done <- true
		fmt.Printf("\r")
	}

	f, err := ioutil.TempFile("", "dh-make-golang")
	tempfile := f.Name()
	f.Close()
	base := filepath.Base(gopkg)
	dir := filepath.Dir(gopkg)
	cmd = exec.Command(
		"tar",
		"cJf",
		tempfile,
		"--exclude-vcs",
		"--exclude=Godeps",
		fmt.Sprintf("--exclude=%s/debian", base),
		base)
	cmd.Dir = filepath.Join(tempdir, "src", dir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", "", dependencies, autoPkgType, err
	}

	if _, err := os.Stat(filepath.Join(tempdir, "src", gopkg, ".git")); os.IsNotExist(err) {
		return "", "", dependencies, autoPkgType, fmt.Errorf("Not a git repository, dh-make-golang currently only supports git")
	}

	log.Printf("Determining upstream version number\n")

	version, err := pkgVersionFromGit(filepath.Join(tempdir, "src", gopkg))
	if err != nil {
		return "", "", dependencies, autoPkgType, err
	}

	log.Printf("Package version is %q\n", version)

	// If no import path defines a “main” package, we’re dealing with a
	// library, otherwise likely a program.
	log.Printf("Determining package type\n")
	cmd = exec.Command("go", "list", "-f", "{{.ImportPath}} {{.Name}}", gopkg+"/...")
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		fmt.Sprintf("GOPATH=%s", tempdir),
	}, passthroughEnv()...)

	out, err := cmd.Output()
	if err != nil {
		return "", "", dependencies, autoPkgType, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.Contains(line, "/vendor/") ||
			strings.Contains(line, "/Godeps/") ||
			strings.Contains(line, "/samples/") ||
			strings.Contains(line, "/examples/") ||
			strings.Contains(line, "/example/") {
			continue
		}
		if strings.HasSuffix(line, " main") {
			if strings.TrimSpace(*pkgType) == "" {
				log.Printf("Assuming you are packaging a program (because %q defines a main package), use -type to override\n", line[:len(line)-len(" main")])
			}
			autoPkgType = "program"
			break
		}
	}

	log.Printf("Determining dependencies\n")

	cmd = exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}\n{{join .TestImports \"\\n\"}}\n{{join .XTestImports \"\\n\"}}", gopkg+"/...")
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		fmt.Sprintf("GOPATH=%s", tempdir),
	}, passthroughEnv()...)

	out, err = cmd.Output()
	if err != nil {
		return "", "", dependencies, autoPkgType, err
	}

	var godependencies []string
	for _, p := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(p) == "" {
			continue
		}
		// Strip packages that are included in the repository we are packaging.
		if strings.HasPrefix(p, gopkg) {
			continue
		}
		if p == "C" {
			// TODO: maybe parse the comments to figure out C deps from pkg-config files?
		} else {
			godependencies = append(godependencies, p)
		}
	}

	if len(godependencies) == 0 {
		return tempfile, version, dependencies, autoPkgType, nil
	}

	// Remove all packages which are in the standard lib.
	args := []string{"list", "-f", "{{.ImportPath}}: {{.Standard}}"}
	args = append(args, godependencies...)

	cmd = exec.Command("go", args...)
	cmd.Dir = filepath.Join(tempdir, "src", gopkg)
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		fmt.Sprintf("GOPATH=%s", tempdir),
	}, passthroughEnv()...)

	out, err = cmd.Output()
	if err != nil {
		return "", "", dependencies, autoPkgType, err
	}

	for _, p := range strings.Split(string(out), "\n") {
		if strings.HasSuffix(p, ": false") {
			importpath := p[:len(p)-len(": false")]
			rr, err := vcs.RepoRootForImportPath(importpath, false)
			if err != nil {
				log.Printf("Could not determine repo path for import path %q: %v\n", importpath, err)
			}
			dependencies[debianNameFromGopkg(rr.Root, "library")+"-dev"] = true
		}
	}
	return tempfile, version, dependencies, autoPkgType, nil
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

	if err := ioutil.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".pc\n"), 0644); err != nil {
		return dir, err
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
func debianNameFromGopkg(gopkg, t string) string {
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
	} else {
		if *allowUnknownHoster {
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

func websiteForGopkg(gopkg string) string {
	if strings.HasPrefix(gopkg, "github.com/") {
		return "https://" + gopkg
	}
	return "TODO"
}

func writeTemplates(dir, gopkg, debsrc, debbin, debversion string, dependencies []string) error {
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
	fmt.Fprintf(f, "10\n")

	f, err = os.Create(filepath.Join(dir, "debian", "control"))
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "Source: %s\n", debsrc)
	// TODO: change this once we have a “golang” section.
	fmt.Fprintf(f, "Section: devel\n")
	fmt.Fprintf(f, "Priority: extra\n")
	fmt.Fprintf(f, "Maintainer: Debian Go Packaging Team <pkg-go-maintainers@lists.alioth.debian.org>\n")
	fmt.Fprintf(f, "Uploaders: %s <%s>\n", getDebianName(), getDebianEmail())
	sort.Strings(dependencies)
	builddeps := append([]string{"debhelper (>= 10)", "dh-golang", "golang-any"}, dependencies...)
	fmt.Fprintf(f, "Build-Depends: %s\n", strings.Join(builddeps, ",\n               "))
	fmt.Fprintf(f, "Standards-Version: 4.0.0\n")
	fmt.Fprintf(f, "Homepage: %s\n", websiteForGopkg(gopkg))
	fmt.Fprintf(f, "Vcs-Browser: https://anonscm.debian.org/cgit/pkg-go/packages/%s.git\n", debsrc)
	fmt.Fprintf(f, "Vcs-Git: https://anonscm.debian.org/git/pkg-go/packages/%s.git\n", debsrc)
	fmt.Fprintf(f, "XS-Go-Import-Path: %s\n", gopkg)
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Package: %s\n", debbin)
	deps := []string{"${shlibs:Depends}", "${misc:Depends}"}
	if *pkgType == "program" {
		fmt.Fprintf(f, "Architecture: any\n")
		fmt.Fprintf(f, "Built-Using: ${misc:Built-Using}\n")
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
	fmt.Fprintf(f, "Source: %s\n", websiteForGopkg(gopkg))
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
	fmt.Fprintf(f, "* URL             : %s\n", websiteForGopkg(gopkg))
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

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s <go-package-name>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s golang.org/x/oauth2\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "%s will create new files and directories in the current working directory.\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "%s will connect to the internet to download the specified Go package and its dependencies.\n", os.Args[0])
		os.Exit(1)
	}

	gopkg := flag.Arg(0)
	debsrc := debianNameFromGopkg(gopkg, "library")

	if strings.TrimSpace(*pkgType) != "" {
		debsrc = debianNameFromGopkg(gopkg, *pkgType)
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

	golangBinaries := make(map[string]bool)
	var golangBinariesMu sync.RWMutex

	// TODO: also check whether there already is a git repository on alioth.
	go func() {
		resp, err := http.Get(golangBinariesURL)
		if err != nil {
			log.Printf("Cannot do duplicate check: could not download %q: %v\n", golangBinariesURL, err)
			return
		}
		if resp.StatusCode != http.StatusOK {
			log.Printf("Cannot do duplicate check: could not download %q: status %v\n", golangBinariesURL, resp.Status)
			return
		}
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Cannot do duplicate check: could not download %q: reading response: %v\n", golangBinariesURL, err)
			return
		}
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		golangBinariesMu.Lock()
		for _, line := range lines {
			if strings.HasPrefix(line, "Package: ") {
				line = line[len("Package: "):]
			}
			golangBinaries[line] = true
		}
		golangBinariesMu.Unlock()
	}()

	tempfile, version, debdependencies, autoPkgType, err := makeUpstreamSourceTarball(gopkg)
	if err != nil {
		log.Fatalf("Could not create a tarball of the upstream source: %v\n", err)
	}

	if strings.TrimSpace(*pkgType) == "" {
		*pkgType = autoPkgType
		debsrc = debianNameFromGopkg(gopkg, *pkgType)
	}
	debbin := debsrc + "-dev"
	if *pkgType == "program" {
		debbin = debsrc
	}

	if _, err := os.Stat(debsrc); err == nil {
		log.Fatalf("Output directory %q already exists, aborting\n", debsrc)
	}

	dependencies := make([]string, len(debdependencies))
	i := 0
	for root := range debdependencies {
		if root == debsrc+"-dev" {
			continue
		}
		dependencies[i] = root
		i++
	}

	golangBinariesMu.RLock()
	if golangBinaries[debbin] {
		log.Printf("WARNING: A package called %q is already in Debian! See https://tracker.debian.org/pkg/%s\n",
			debbin, debbin)
	}
	golangBinariesMu.RUnlock()

	orig := fmt.Sprintf("%s_%s.orig.tar.xz", debsrc, version)
	// We need to copy the file, merely renaming is not enough since the file
	// might be on a different filesystem (/tmp often is a tmpfs).
	if err := copyFile(tempfile, orig); err != nil {
		log.Fatalf("Could not rename orig tarball from %q to %q: %v\n", tempfile, orig, err)
	}
	if err := os.Remove(tempfile); err != nil {
		log.Printf("Could not remove tempfile %q: %v\n", tempfile, err)
	}

	debversion := version + "-1"

	dir, err := createGitRepository(debsrc, gopkg, orig)
	if err != nil {
		log.Fatalf("Could not create git repository: %v\n", err)
	}

	golangBinariesMu.RLock()
	if len(golangBinaries) > 0 {
		for _, dep := range dependencies {
			if golangBinaries[dep] {
				continue
			}
			log.Printf("Build-Dependency %q is not yet available in Debian\n", dep)
		}
	}
	golangBinariesMu.RUnlock()

	if err := writeTemplates(dir, gopkg, debsrc, debbin, debversion, dependencies); err != nil {
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
	log.Printf("To create the packaging git repository on alioth, use:\n")
	log.Printf("    ssh git.debian.org \"/git/pkg-go/setup-repository %s 'Packaging for %s'\"\n", debsrc, debsrc)
	log.Printf("\n")
	log.Printf("Once you are happy with your packaging, push it to alioth using:\n")
	log.Printf("    git push git+ssh://git.debian.org/git/pkg-go/packages/%s.git --tags master pristine-tar upstream\n", debsrc)
}
