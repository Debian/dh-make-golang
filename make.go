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
	"strings"

	"golang.org/x/net/publicsuffix"
	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/go/vcs"
)

type packageType int

const (
	typeGuess packageType = iota
	typeLibrary
	typeProgram
	typeLibraryProgram
	typeProgramLibrary
)

var (
	dep14       bool
	pristineTar bool
	wrapAndSort string
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

	if err := runGitCommandIn(dir, "checkout", "-b", "debian/sid"); err != nil {
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

	cmd := exec.Command("gbp", "import-orig", "--debian-branch=debian/sid", "--no-interactive", filepath.Join(wd, orig))
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
func debianNameFromGopkg(gopkg string, t packageType, allowUnknownHoster bool) string {
	parts := strings.Split(gopkg, "/")

	if t == typeProgram || t == typeProgramLibrary {
		return normalizeDebianProgramName(parts[len(parts)-1])
	}

	host := parts[0]
	switch host {
	case "github.com":
		host = "github"
	case "code.google.com":
		host = "googlecode"
	case "cloud.google.com":
		host = "googlecloud"
	case "gopkg.in":
		host = "gopkg"
	case "golang.org":
		host = "golang"
	case "google.golang.org":
		host = "google"
	case "bitbucket.org":
		host = "bitbucket"
	case "bazil.org":
		host = "bazil"
	case "pault.ag":
		host = "pault"
	case "howett.net":
		host = "howett"
	case "go4.org":
		host = "go4"
	case "salsa.debian.org":
		host = "debian"
	default:
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
	fmt.Fprintf(f, "X-Debbugs-CC: debian-devel@lists.debian.org, debian-go@lists.debian.org\n")
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
			fmt.Fprintf(os.Stderr, "Usage: %s [make] [FLAG]... <go-package-importpath>\n", os.Args[0])
			fmt.Fprintf(os.Stderr, "Example: %s make golang.org/x/oauth2\n", os.Args[0])
			fmt.Fprintf(os.Stderr, "\n")
			fmt.Fprintf(os.Stderr, "\"%s make\" downloads the specified Go package from the Internet,\nand creates new files and directories in the current working directory.\n", os.Args[0])
			fmt.Fprintf(os.Stderr, "\n")
			fmt.Fprintf(os.Stderr, "Flags:\n")
			fs.PrintDefaults()
		}
	}

	var gitRevision string
	fs.StringVar(&gitRevision,
		"git_revision",
		"",
		"git revision (see gitrevisions(7)) of the specified Go package\nto check out, defaulting to the default behavior of git clone.\nUseful in case you do not want to package e.g. current HEAD.")

	var allowUnknownHoster bool
	fs.BoolVar(&allowUnknownHoster,
		"allow_unknown_hoster",
		false,
		"The pkg-go naming conventions use a canonical identifier for\nthe hostname (see https://go-team.pages.debian.net/packaging.html),\nand the mapping is hardcoded into dh-make-golang.\nIn case you want to package a Go package living on an unknown hoster,\nyou may set this flag to true and double-check that the resulting\npackage name is sane. Contact pkg-go if unsure.")

	fs.BoolVar(&dep14,
		"dep14",
		true,
		"Follow DEP-14 branch naming and use debian/sid (instead of master)\nas the default debian-branch.")

	fs.BoolVar(&pristineTar,
		"pristine-tar",
		false,
		"Keep using a pristine-tar branch as in the old workflow.\nStrongly discouraged, see \"pristine-tar considered harmful\"\n https://michael.stapelberg.ch/posts/2018-01-28-pristine-tar/\nand the \"Drop pristine-tar branches\" section at\nhttps://go-team.pages.debian.net/workflow-changes.html")

	var pkgTypeString string
	fs.StringVar(&pkgTypeString,
		"type",
		"",
		"Set package type, one of:\n"+
			` * "library" (aliases: "lib", "l", "dev")`+"\n"+
			` * "program" (aliases: "prog", "p")`+"\n"+
			` * "library+program" (aliases: "lib+prog", "l+p", "both")`+"\n"+
			` * "program+library" (aliases: "prog+lib", "p+l", "combined")`)

	fs.StringVar(&wrapAndSort,
		"wrap-and-sort",
		"a",
		"Set how the various multi-line fields in debian/control are formatted.\nValid values are \"a\", \"at\" and \"ast\", see wrap-and-sort(1) man page\nfor more information.")

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

	// Set default source and binary package names.
	// Note that debsrc may change depending on the actual package type.
	debsrc := debianNameFromGopkg(gopkg, typeLibrary, allowUnknownHoster)
	debLib := debsrc + "-dev"
	debProg := debianNameFromGopkg(gopkg, typeProgram, allowUnknownHoster)

	var pkgType packageType

	switch strings.TrimSpace(pkgTypeString) {
	case "", "guess":
		pkgType = typeGuess
	case "library", "lib", "l", "dev":
		pkgType = typeLibrary
	case "program", "prog", "p":
		pkgType = typeProgram
	case "library+program", "lib+prog", "l+p", "both":
		// Example packages: golang-github-alecthomas-chroma,
		// golang-github-tdewolff-minify, golang-github-spf13-viper
		pkgType = typeLibraryProgram
	case "program+library", "prog+lib", "p+l", "combined":
		// Example package: hugo
		pkgType = typeProgramLibrary
	default:
		log.Fatalf("-type=%q not recognized, aborting\n", pkgTypeString)
	}

	if pkgType != typeGuess {
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

	if pkgType == typeGuess {
		if u.firstMain != "" {
			log.Printf("Assuming you are packaging a program (because %q defines a main package), use -type to override\n", u.firstMain)
			pkgType = typeProgram
			debsrc = debianNameFromGopkg(gopkg, pkgType, allowUnknownHoster)
		} else {
			pkgType = typeLibrary
		}
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
			debdependencies = append(debdependencies, debianNameFromGopkg(dep, typeLibrary, allowUnknownHoster)+"-dev")
			continue
		}
		bin, ok := golangBinaries[dep]
		if !ok {
			log.Printf("Build-Dependency %q is not yet available in Debian, or has not yet been converted to use XS-Go-Import-Path in debian/control", dep)
			continue
		}
		debdependencies = append(debdependencies, bin)
	}

	if err := writeTemplates(dir, gopkg, debsrc, debLib, debProg, debversion, pkgType, debdependencies, u.vendorDirs); err != nil {
		log.Fatalf("Could not create debian/ from templates: %v\n", err)
	}

	itpname, err := writeITP(gopkg, debsrc, debversion)
	if err != nil {
		log.Fatalf("Could not write ITP email: %v\n", err)
	}

	log.Printf("\n")
	log.Printf("Packaging successfully created in %s\n", dir)
	log.Printf("\n")
	log.Printf("    Source: %s\n", debsrc)
	switch pkgType {
	case typeLibrary:
		log.Printf("    Package: %s\n", debLib)
	case typeProgram:
		log.Printf("    Package: %s\n", debProg)
	case typeLibraryProgram:
		log.Printf("    Package: %s\n", debLib)
		log.Printf("    Package: %s\n", debProg)
	case typeProgramLibrary:
		log.Printf("    Package: %s\n", debProg)
		log.Printf("    Package: %s\n", debLib)
	}
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
