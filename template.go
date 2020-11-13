package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func writeTemplates(dir, gopkg, debsrc, debLib, debProg, debversion string,
	pkgType packageType, dependencies []string, u *upstream,
	dep14, pristineTar bool,
) error {

	if err := os.Mkdir(filepath.Join(dir, "debian"), 0755); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(dir, "debian", "source"), 0755); err != nil {
		return err
	}

	if err := writeDebianChangelog(dir, debsrc, debversion); err != nil {
		return err
	}
	if err := writeDebianControl(dir, gopkg, debsrc, debLib, debProg, pkgType, dependencies); err != nil {
		return err
	}
	if err := writeDebianCopyright(dir, gopkg, u.vendorDirs, u.hasGodeps); err != nil {
		return err
	}
	if err := writeDebianRules(dir, pkgType); err != nil {
		return err
	}

	var repack bool = len(u.vendorDirs) > 0 || u.hasGodeps
	if err := writeDebianWatch(dir, gopkg, debsrc, u.hasRelease, repack); err != nil {
		return err
	}

	if err := writeDebianSourceFormat(dir); err != nil {
		return err
	}
	if err := writeDebianPackageInstall(dir, debLib, debProg, pkgType); err != nil {
		return err
	}
	if err := writeDebianUpstreamMetadata(dir, gopkg); err != nil {
		return err
	}

	if err := writeDebianGbpConf(dir, dep14, pristineTar); err != nil {
		return err
	}

	if err := writeDebianGitLabCI(dir); err != nil {
		return err
	}

	return nil
}

func writeDebianChangelog(dir, debsrc, debversion string) error {
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

	return nil
}

func fprintfControlField(f *os.File, field string, valueArray []string) {
	switch wrapAndSort {
	case "a":
		// Current default, also what "cme fix dpkg" generates
		fmt.Fprintf(f, "%s: %s\n", field, strings.Join(valueArray, ",\n"+strings.Repeat(" ", len(field)+2)))
	case "at":
		// -t, --trailing-comma, preferred by Martina Ferrari
		// and currently used in quite a few packages
		fmt.Fprintf(f, "%s: %s,\n", field, strings.Join(valueArray, ",\n"+strings.Repeat(" ", len(field)+2)))
	case "ast":
		// -s, --short-indent too, proposed by Guillem Jover
		fmt.Fprintf(f, "%s:\n %s,\n", field, strings.Join(valueArray, ",\n "))
	default:
		log.Fatalf("%q is not a valid value for -wrap-and-sort, aborting.", wrapAndSort)
	}
}

func addDescription(f *os.File, gopkg, comment string) {
	description, err := getDescriptionForGopkg(gopkg)
	if err != nil {
		log.Printf("Could not determine description for %q: %v\n", gopkg, err)
		description = "TODO: short description"
	}
	fmt.Fprintf(f, "Description: %s %s\n", description, comment)

	longdescription, err := getLongDescriptionForGopkg(gopkg)
	if err != nil {
		log.Printf("Could not determine long description for %q: %v\n", gopkg, err)
		longdescription = "TODO: long description"
	}
	fmt.Fprintf(f, " %s\n", longdescription)
}

func addLibraryPackage(f *os.File, gopkg, debLib string, dependencies []string) {
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Package: %s\n", debLib)
	fmt.Fprintf(f, "Architecture: all\n")
	deps := dependencies
	sort.Strings(deps)
	deps = append(deps, "${misc:Depends}")
	fprintfControlField(f, "Depends", deps)
	addDescription(f, gopkg, "(library)")
}

func addProgramPackage(f *os.File, gopkg, debProg string) {
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Package: %s\n", debProg)
	fmt.Fprintf(f, "Architecture: any\n")
	deps := []string{"${misc:Depends}", "${shlibs:Depends}"}
	fprintfControlField(f, "Depends", deps)
	fmt.Fprintf(f, "Built-Using: ${misc:Built-Using}\n")
	addDescription(f, gopkg, "(program)")
}

func writeDebianControl(dir, gopkg, debsrc, debLib, debProg string, pkgType packageType, dependencies []string) error {
	f, err := os.Create(filepath.Join(dir, "debian", "control"))
	if err != nil {
		return err
	}
	defer f.Close()

	// Source package:

	fmt.Fprintf(f, "Source: %s\n", debsrc)
	fmt.Fprintf(f, "Maintainer: Debian Go Packaging Team <team+pkg-go@tracker.debian.org>\n")
	fprintfControlField(f, "Uploaders", []string{getDebianName() + " <" + getDebianEmail() + ">"})
	fmt.Fprintf(f, "Section: golang\n")
	fmt.Fprintf(f, "Testsuite: autopkgtest-pkg-go\n")
	fmt.Fprintf(f, "Priority: optional\n")

	builddeps := append([]string{
		"debhelper-compat (= 13)",
		"dh-golang",
		"golang-any"},
		dependencies...)
	sort.Strings(builddeps)
	fprintfControlField(f, "Build-Depends", builddeps)

	fmt.Fprintf(f, "Standards-Version: 4.5.0\n")
	fmt.Fprintf(f, "Vcs-Browser: https://salsa.debian.org/go-team/packages/%s\n", debsrc)
	fmt.Fprintf(f, "Vcs-Git: https://salsa.debian.org/go-team/packages/%s.git\n", debsrc)
	fmt.Fprintf(f, "Homepage: %s\n", getHomepageForGopkg(gopkg))
	fmt.Fprintf(f, "Rules-Requires-Root: no\n")
	fmt.Fprintf(f, "XS-Go-Import-Path: %s\n", gopkg)

	// Binary package(s):

	switch pkgType {
	case typeLibrary:
		addLibraryPackage(f, gopkg, debLib, dependencies)
	case typeProgram:
		addProgramPackage(f, gopkg, debProg)
	case typeLibraryProgram:
		addLibraryPackage(f, gopkg, debLib, dependencies)
		addProgramPackage(f, gopkg, debProg)
	case typeProgramLibrary:
		addProgramPackage(f, gopkg, debProg)
		addLibraryPackage(f, gopkg, debLib, dependencies)
	default:
		log.Fatalf("Invalid pkgType %d in writeDebianControl(), aborting", pkgType)
	}

	return nil
}

func writeDebianCopyright(dir, gopkg string, vendorDirs []string, hasGodeps bool) error {
	license, fulltext, err := getLicenseForGopkg(gopkg)
	if err != nil {
		log.Printf("Could not determine license for %q: %v\n", gopkg, err)
		license = "TODO"
		fulltext = "TODO"
	}

	f, err := os.Create(filepath.Join(dir, "debian", "copyright"))
	if err != nil {
		return err
	}
	defer f.Close()

	_, copyright, err := getAuthorAndCopyrightForGopkg(gopkg)
	if err != nil {
		log.Printf("Could not determine copyright for %q: %v\n", gopkg, err)
		copyright = "TODO"
	}

	var indent = "  "
	var linebreak = ""
	if wrapAndSort == "ast" {
		indent = " "
		linebreak = "\n"
	}

	fmt.Fprintf(f, "Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/\n")
	fmt.Fprintf(f, "Upstream-Name: %s\n", filepath.Base(gopkg))
	fmt.Fprintf(f, "Upstream-Contact: TODO\n")
	fmt.Fprintf(f, "Source: %s\n", getHomepageForGopkg(gopkg))
	if len(vendorDirs) > 0 || hasGodeps {
		fmt.Fprintf(f, "Files-Excluded:\n")
		for _, dir := range vendorDirs {
			fmt.Fprintf(f, indent+"%s\n", dir)
		}
		if hasGodeps {
			fmt.Fprintf(f, indent+"Godeps/_workspace\n")
		}
	}
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Files:"+linebreak+" *\n")
	fmt.Fprintf(f, "Copyright:"+linebreak+" %s\n", copyright)
	fmt.Fprintf(f, "License: %s\n", license)
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Files:"+linebreak+" debian/*\n")
	fmt.Fprintf(f, "Copyright:"+linebreak+" %s %s <%s>\n", time.Now().Format("2006"), getDebianName(), getDebianEmail())
	fmt.Fprintf(f, "License: %s\n", license)
	fmt.Fprintf(f, "Comment: Debian packaging is licensed under the same terms as upstream\n")
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "License: %s\n", license)
	fmt.Fprintf(f, fulltext)

	return nil
}

func writeDebianRules(dir string, pkgType packageType) error {
	f, err := os.Create(filepath.Join(dir, "debian", "rules"))
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "#!/usr/bin/make -f\n")
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "%%:\n")
	fmt.Fprintf(f, "\tdh $@ --builddirectory=_build --buildsystem=golang --with=golang\n")
	if pkgType == typeProgram {
		fmt.Fprintf(f, "\n")
		fmt.Fprintf(f, "override_dh_auto_install:\n")
		fmt.Fprintf(f, "\tdh_auto_install -- --no-source\n")
	}

	if err := os.Chmod(filepath.Join(dir, "debian", "rules"), 0755); err != nil {
		return err
	}

	return nil
}

func writeDebianSourceFormat(dir string) error {
	f, err := os.Create(filepath.Join(dir, "debian", "source", "format"))
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "3.0 (quilt)\n")
	return nil
}

func writeDebianGbpConf(dir string, dep14, pristineTar bool) error {
	if !(dep14 || pristineTar) {
		return nil
	}

	f, err := os.Create(filepath.Join(dir, "debian", "gbp.conf"))
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "[DEFAULT]\n")
	if dep14 {
		fmt.Fprintf(f, "debian-branch = debian/sid\n")
		fmt.Fprintf(f, "dist = DEP14\n")
	}
	if pristineTar {
		fmt.Fprintf(f, "pristine-tar = True\n")
	}
	return nil
}

func writeDebianWatch(dir, gopkg, debsrc string, hasRelease bool, repack bool) error {
	// TODO: Support other hosters too
	host := "github.com"

	owner, repo, err := findGitHubRepo(gopkg)
	if err != nil {
		log.Printf("debian/watch: Unable to resolve %s to github.com, skipping\n", gopkg)
		return nil
	}
	if !strings.HasPrefix(gopkg, "github.com/") {
		log.Printf("debian/watch: %s resolves to %s/%s/%s\n", gopkg, host, owner, repo)
	}

	f, err := os.Create(filepath.Join(dir, "debian", "watch"))
	if err != nil {
		return err
	}
	defer f.Close()

	filenamemanglePattern := `s%%(?:.*?)?v?(\d[\d.]*)\.tar\.gz%%%s-$1.tar.gz%%`
	uversionmanglePattern := `s/(\d)[_\.\-\+]?(RC|rc|pre|dev|beta|alpha)[.]?(\d*)$/\$1~\$2\$3/`

	if hasRelease {
		log.Printf("Setting debian/watch to track release tarball")
		fmt.Fprintf(f, "version=4\n")
		fmt.Fprintf(f, `opts="filenamemangle=`+filenamemanglePattern+`,\`+"\n", debsrc)
		fmt.Fprintf(f, `      uversionmangle=`+uversionmanglePattern)
		if repack {
			fmt.Fprintf(f, `,\`+"\n")
			fmt.Fprintf(f, `      dversionmangle=s/\+ds\d*$//,repacksuffix=+ds1`)
		}
		fmt.Fprintf(f, `" \`+"\n")
		fmt.Fprintf(f, `  https://%s/%s/%s/tags .*/v?(\d\S*)\.tar\.gz debian`+"\n", host, owner, repo)
	} else {
		log.Printf("Setting debian/watch to track git HEAD")
		fmt.Fprintf(f, "version=4\n")
		fmt.Fprintf(f, `opts="mode=git, pgpmode=none`)
		if repack {
			fmt.Fprintf(f, `,\`+"\n")
			fmt.Fprintf(f, `      dversionmangle=s/\+ds\d*$//,repacksuffix=+ds1`)
		}
		fmt.Fprintf(f, `" \`+"\n")
		fmt.Fprintf(f, `  https://%s/%s/%s.git \`+"\n", host, owner, repo)
		fmt.Fprintf(f, "  HEAD debian\n")

		// Anticipate that upstream would eventually switch to tagged releases
		fmt.Fprintf(f, "\n")
		fmt.Fprintf(f, "# Use the following when upstream starts to tag releases:\n")
		fmt.Fprintf(f, "#\n")
		fmt.Fprintf(f, "#version=4\n")
		fmt.Fprintf(f, `#opts="filenamemangle=`+filenamemanglePattern+`,\`+"\n", debsrc)
		fmt.Fprintf(f, `#      uversionmangle=`+uversionmanglePattern)
		if repack {
			fmt.Fprintf(f, `,\`+"\n")
			fmt.Fprintf(f, `#      dversionmangle=s/\+ds\d*$//,repacksuffix=+ds1`)
		}
		fmt.Fprintf(f, `" \`+"\n")
		fmt.Fprintf(f, `#  https://%s/%s/%s/tags .*/v?(\d\S*)\.tar\.gz debian`+"\n", host, owner, repo)
	}

	return nil
}

func writeDebianPackageInstall(dir, debLib, debProg string, pkgType packageType) error {
	if pkgType == typeLibraryProgram || pkgType == typeProgramLibrary {
		f, err := os.Create(filepath.Join(dir, "debian", debProg+".install"))
		if err != nil {
			return err
		}
		defer f.Close()
		fmt.Fprintf(f, "usr/bin\n")

		f, err = os.Create(filepath.Join(dir, "debian", debLib+".install"))
		if err != nil {
			return err
		}
		defer f.Close()
		fmt.Fprintf(f, "usr/share\n")
	}
	return nil
}

func writeDebianUpstreamMetadata(dir, gopkg string) error {
	// TODO: Support other hosters too
	host := "github.com"

	owner, repo, err := findGitHubRepo(gopkg)
	if err != nil {
		log.Printf("debian/upstream/metadata: Unable to resolve %s to github.com, skipping\n", gopkg)
		return nil
	}
	if !strings.HasPrefix(gopkg, "github.com/") {
		log.Printf("debian/upstream/metadata: %s resolves to %s/%s/%s\n", gopkg, host, owner, repo)
	}

	if err := os.Mkdir(filepath.Join(dir, "debian", "upstream"), 0755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "debian", "upstream", "metadata"))
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "---\n")
	fmt.Fprintf(f, "Bug-Database: https://%s/%s/%s/issues\n", host, owner, repo)
	fmt.Fprintf(f, "Bug-Submit: https://%s/%s/%s/issues/new\n", host, owner, repo)
	fmt.Fprintf(f, "Repository: https://%s/%s/%s.git\n", host, owner, repo)
	fmt.Fprintf(f, "Repository-Browse: https://%s/%s/%s\n", host, owner, repo)

	return nil
}

func writeDebianGitLabCI(dir string) error {
	const gitlabciymlTmpl = `# auto-generated, DO NOT MODIFY.
# The authoritative copy of this file lives at:
# https://salsa.debian.org/go-team/ci/blob/master/config/gitlabciyml.go

image: stapelberg/ci2

test_the_archive:
  artifacts:
    paths:
    - before-applying-commit.json
    - after-applying-commit.json
  script:
    # Create an overlay to discard writes to /srv/gopath/src after the build:
    - "rm -rf /cache/overlay/{upper,work}"
    - "mkdir -p /cache/overlay/{upper,work}"
    - "mount -t overlay overlay -o lowerdir=/srv/gopath/src,upperdir=/cache/overlay/upper,workdir=/cache/overlay/work /srv/gopath/src"
    - "export GOPATH=/srv/gopath"
    - "export GOCACHE=/cache/go"
    # Build the world as-is:
    - "ci-build -exemptions=/var/lib/ci-build/exemptions.json > before-applying-commit.json"
    # Copy this package into the overlay:
    - "GBP_CONF_FILES=:debian/gbp.conf gbp buildpackage --git-no-pristine-tar --git-ignore-branch --git-ignore-new --git-export-dir=/tmp/export --git-no-overlay --git-tarball-dir=/nonexistant --git-cleaner=/bin/true --git-builder='dpkg-buildpackage -S -d --no-sign'"
    - "pgt-gopath -dsc /tmp/export/*.dsc"
    # Rebuild the world:
    - "ci-build -exemptions=/var/lib/ci-build/exemptions.json > after-applying-commit.json"
    - "ci-diff before-applying-commit.json after-applying-commit.json"
`

	f, err := os.Create(filepath.Join(dir, "debian", "gitlab-ci.yml"))
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, gitlabciymlTmpl)

	return nil
}
