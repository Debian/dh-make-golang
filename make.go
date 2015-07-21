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
)

func makeUpstreamSourceTarball(gopkg, debsrc string) (string, string, []string, error) {
	var dependencies []string

	tempdir, err := ioutil.TempDir("", "dh-make-golang")
	if err != nil {
		return "", "", dependencies, err
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
	cmd.Env = []string{
		fmt.Sprintf("GOPATH=%s", tempdir),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
	}
	if err := cmd.Run(); err != nil {
		done <- true
		return "", "", dependencies, err
	}
	done <- true
	fmt.Printf("\r")

	revision := strings.TrimSpace(*gitRevision)
	if revision != "" {
		log.Printf("Checking out git revision %q\n", revision)
		if err := runGitCommandIn(filepath.Join(tempdir, "src", gopkg), "reset", "--hard", *gitRevision); err != nil {
			log.Fatalf("Could not check out git revision %q: %v\n", revision, err)
		}
	}

	f, err := ioutil.TempFile("", "dh-make-golang")
	tempfile := f.Name()
	f.Close()
	base := filepath.Base(gopkg)
	dir := filepath.Dir(gopkg)
	cmd = exec.Command("tar", "cjf", tempfile, "--exclude-vcs", base)
	cmd.Dir = filepath.Join(tempdir, "src", dir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", "", dependencies, err
	}

	if _, err := os.Stat(filepath.Join(tempdir, "src", gopkg, ".git")); os.IsNotExist(err) {
		return "", "", dependencies, fmt.Errorf("Not a git repository, dh-make-golang currently only supports git")
	}

	log.Printf("Determining upstream version number\n")

	version, err := pkgVersionFromGit(filepath.Join(tempdir, "src", gopkg))
	if err != nil {
		return "", "", dependencies, err
	}

	log.Printf("Package version is %q\n", version)

	log.Printf("Determining dependencies\n")

	cmd = exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}\n{{join .TestImports \"\\n\"}}\n{{join .XTestImports \"\\n\"}}", gopkg+"/...")
	cmd.Stderr = os.Stderr
	cmd.Env = []string{
		fmt.Sprintf("GOPATH=%s", tempdir),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
	}
	out, err := cmd.Output()
	if err != nil {
		return "", "", dependencies, err
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

	// Remove all packages which are in the standard lib.
	args := []string{"list", "-f", "{{.ImportPath}}: {{.Standard}}"}
	args = append(args, godependencies...)

	cmd = exec.Command("go", args...)
	cmd.Stderr = os.Stderr
	cmd.Env = []string{
		fmt.Sprintf("GOPATH=%s", tempdir),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
	}
	out, err = cmd.Output()
	if err != nil {
		return "", "", godependencies, err
	}

	// debdependencies is a map in order to de-duplicate multiple imports
	// pointing into the same repository.
	debdependencies := make(map[string]bool)
	for _, p := range strings.Split(string(out), "\n") {
		if strings.HasSuffix(p, ": false") {
			importpath := p[:len(p)-len(": false")]
			rr, err := vcs.RepoRootForImportPath(importpath, false)
			if err != nil {
				log.Printf("Could not determine repo path for import path %q: %v\n", importpath, err)
			}
			importdebsrc := debianNameFromGopkg(rr.Root)
			if importdebsrc == debsrc {
				continue
			}
			debdependencies[importdebsrc+"-dev"] = true
		}
	}
	debdependenciesSlice := make([]string, len(debdependencies))
	i := 0
	for root := range debdependencies {
		debdependenciesSlice[i] = root
		i++
	}

	return tempfile, version, debdependenciesSlice, nil
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

	if err := runGitCommandIn(dir, "config", "user.name", getDebianName()); err != nil {
		return dir, err
	}

	if err := runGitCommandIn(dir, "config", "user.email", getDebianEmail()); err != nil {
		return dir, err
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
	return dir, cmd.Run()
}

// This follows https://fedoraproject.org/wiki/PackagingDrafts/Go#Package_Names
func debianNameFromGopkg(gopkg string) string {
	parts := strings.Split(gopkg, "/")
	host := parts[0]
	if host == "github.com" {
		host = "github"
	} else if host == "code.google.com" {
		host = "googlecode"
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
	return "golang-" + strings.ToLower(strings.Join(parts, "-"))
}

func getDebianName() string {
	if name := strings.TrimSpace(os.Getenv("DEBFULLNAME")); name != "" {
		return name
	}
	if name := strings.TrimSpace(os.Getenv("DEBNAME")); name != "" {
		return name
	}
	if u, err := user.Current(); err == nil {
		return u.Name
	}
	return "TODO"
}

func getDebianEmail() string {
	if email := strings.TrimSpace(os.Getenv("DEBEMAIL")); email != "" {
		return email
	}
	if mailname, err := ioutil.ReadFile("/etc/mailname"); err == nil {
		if u, err := user.Current(); err == nil {
			return u.Name + "@" + strings.TrimSpace(string(mailname))
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

func writeTemplates(dir, gopkg, debsrc, debversion string, dependencies []string) error {
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
	fmt.Fprintf(f, "%s (%s) unstable; urgency=medium\n", debsrc, debversion)
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "  * Initial release (Closes: nnnn)\n")
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
	fmt.Fprintf(f, "9\n")

	f, err = os.Create(filepath.Join(dir, "debian", "control"))
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "Source: %s\n", debsrc)
	// TODO: change this once we have a “golang” section.
	fmt.Fprintf(f, "Section: devel\n")
	fmt.Fprintf(f, "Priority: extra\n")
	fmt.Fprintf(f, "Maintainer: pkg-go <pkg-go-maintainers@lists.alioth.debian.org>\n")
	fmt.Fprintf(f, "Uploaders: %s <%s>\n", getDebianName(), getDebianEmail())
	sort.Strings(dependencies)
	builddeps := append([]string{"debhelper (>= 9)", "dh-golang", "golang-go"}, dependencies...)
	fmt.Fprintf(f, "Build-Depends: %s\n", strings.Join(builddeps, ",\n               "))
	fmt.Fprintf(f, "Standards-Version: 3.9.6\n")
	fmt.Fprintf(f, "Homepage: %s\n", websiteForGopkg(gopkg))
	fmt.Fprintf(f, "Vcs-Browser: http://anonscm.debian.org/gitweb/?p=pkg-go/packages/%s.git;a=summary\n", debsrc)
	fmt.Fprintf(f, "Vcs-Git: git://anonscm.debian.org/pkg-go/packages/%s.git\n", debsrc)
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Package: %s-dev\n", debsrc)
	fmt.Fprintf(f, "Architecture: all\n")
	deps := append([]string{"${shlibs:Depends}", "${misc:Depends}", "golang-go"}, dependencies...)
	fmt.Fprintf(f, "Depends: %s\n", strings.Join(deps, ",\n         "))
	fmt.Fprintf(f, "Built-Using: ${misc:Built-Using}\n")
	fmt.Fprintf(f, "Description: TODO: short description\n")
	fmt.Fprintf(f, " TODO: long description\n")

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
	fmt.Fprintf(f, "Format: http://www.debian.org/doc/packaging-manuals/copyright-format/1.0/\n")
	fmt.Fprintf(f, "Upstream-Name: %s\n", filepath.Base(gopkg))
	fmt.Fprintf(f, "Source: %s\n", websiteForGopkg(gopkg))
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Files: *\n")
	fmt.Fprintf(f, "Copyright: TODO\n")
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
	fmt.Fprintf(f, "export DH_GOPKG := %s\n", gopkg)
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

	fmt.Fprintf(f, "From: %q <%s>\n", getDebianName(), getDebianEmail())
	fmt.Fprintf(f, "To: submit@bugs.debian.org\n")
	fmt.Fprintf(f, "Subject: ITP: %s -- TODO\n", debsrc)
	fmt.Fprintf(f, "Content-Type: text/plain; charset=utf-8\n")
	fmt.Fprintf(f, "Content-Transfer-Encoding: 8bit\n")
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Package: wnpp\n")
	fmt.Fprintf(f, "Severity: wishlist\n")
	fmt.Fprintf(f, "Owner: %s <%s>\n", getDebianName(), getDebianEmail())
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "* Package name    : %s\n", debsrc)
	fmt.Fprintf(f, "  Version         : %s\n", debversion)
	fmt.Fprintf(f, "  Upstream Author : TODO\n")
	fmt.Fprintf(f, "* URL             : %s\n", websiteForGopkg(gopkg))
	fmt.Fprintf(f, "* License         : %s\n", license)
	fmt.Fprintf(f, "  Programming Lang: Go\n")
	fmt.Fprintf(f, "  Description     : TODO\n")
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, " TODO: long description\n")
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
	debsrc := debianNameFromGopkg(gopkg)

	if _, err := os.Stat(debsrc); err == nil {
		log.Fatalf("Output directory %q already exists, aborting\n", debsrc)
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

	tempfile, version, dependencies, err := makeUpstreamSourceTarball(gopkg, debsrc)
	if err != nil {
		log.Fatalf("Could not create a tarball of the upstream source: %v\n", err)
	}

	golangBinariesMu.RLock()
	if golangBinaries[debsrc] {
		log.Printf("WARNING: A package called %q is already in Debian! See https://tracker.debian.org/pkg/%s\n", debsrc, debsrc)
	}
	golangBinariesMu.RUnlock()

	orig := fmt.Sprintf("%s_%s.orig.tar.bz2", debsrc, version)
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

	if err := writeTemplates(dir, gopkg, debsrc, debversion, dependencies); err != nil {
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
	log.Printf("    sendmail -t -f < %s\n", itpname)
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
