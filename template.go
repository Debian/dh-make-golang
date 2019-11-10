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

func writeTemplates(dir, gopkg, debsrc, debLib, debProg, debversion string, pkgType packageType, dependencies []string, vendorDirs []string) error {
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
	if err := writeDebianCopyright(dir, gopkg, vendorDirs); err != nil {
		return err
	}
	if err := writeDebianRules(dir, pkgType); err != nil {
		return err
	}
	if err := writeDebianSourceFormat(dir); err != nil {
		return err
	}
	if err := writeDebianGbpConf(dir); err != nil {
		return err
	}
	if err := writeDebianWatch(dir, gopkg, debsrc); err != nil {
		return err
	}
	if err := writeDebianPackageInstall(dir, debLib, debProg, pkgType); err != nil {
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
	case "at", "ta":
		// -t, --trailing-comma, preferred by Martina Ferrari
		// and currently used in quite a few packages
		fmt.Fprintf(f, "%s: %s,\n", field, strings.Join(valueArray, ",\n"+strings.Repeat(" ", len(field)+2)))
	case "ast", "ats", "sat", "sta", "tas", "tsa":
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
	deps := []string{"${misc:Depends}"}
	fmt.Fprintf(f, "Architecture: all\n")
	deps = append(deps, dependencies...)
	sort.Strings(deps)
	fprintfControlField(f, "Depends", deps)
	addDescription(f, gopkg, "(library)")
}

func addProgramPackage(f *os.File, gopkg, debProg string, dependencies []string) {
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Package: %s\n", debProg)
	deps := []string{"${misc:Depends}"}
	fmt.Fprintf(f, "Architecture: any\n")
	deps = append(deps, "${shlibs:Depends}")
	sort.Strings(deps)
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
	// TODO: change this once we have a “golang” section.
	fmt.Fprintf(f, "Section: devel\n")
	fmt.Fprintf(f, "Testsuite: autopkgtest-pkg-go\n")
	fmt.Fprintf(f, "Priority: optional\n")

	builddeps := []string{"debhelper-compat (= 12)", "dh-golang"}
	builddepsByType := append([]string{"golang-any"}, dependencies...)
	sort.Strings(builddepsByType)
	switch pkgType {
	case typeLibrary, typeProgram:
		fprintfControlField(f, "Build-Depends", builddeps)
		builddepsDepType := "Indep"
		if pkgType == typeProgram {
			builddepsDepType = "Arch"
		}
		fprintfControlField(f, "Build-Depends-"+builddepsDepType, builddepsByType)
	case typeLibraryProgram, typeProgramLibrary:
		builddeps = append(builddeps, builddepsByType...)
		fprintfControlField(f, "Build-Depends", builddeps)
	default:
		log.Fatalf("Invalid pkgType %d in writeDebianControl(), aborting", pkgType)
	}

	fmt.Fprintf(f, "Standards-Version: 4.4.1\n")
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
		addProgramPackage(f, gopkg, debProg, dependencies)
	case typeLibraryProgram:
		addLibraryPackage(f, gopkg, debLib, dependencies)
		addProgramPackage(f, gopkg, debProg, dependencies)
	case typeProgramLibrary:
		addProgramPackage(f, gopkg, debProg, dependencies)
		addLibraryPackage(f, gopkg, debLib, dependencies)
	default:
		log.Fatalf("Invalid pkgType %d in writeDebianControl(), aborting", pkgType)
	}

	return nil
}

func writeDebianCopyright(dir, gopkg string, vendorDirs []string) error {
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

	fmt.Fprintf(f, "Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/\n")
	fmt.Fprintf(f, "Source: %s\n", getHomepageForGopkg(gopkg))
	fmt.Fprintf(f, "Upstream-Name: %s\n", filepath.Base(gopkg))
	fmt.Fprintf(f, "Files-Excluded:\n")
	for _, dir := range vendorDirs {
		fmt.Fprintf(f, " %s\n", dir)
	}
	fmt.Fprintf(f, " Godeps/_workspace\n")
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Files:\n *\n")
	fmt.Fprintf(f, "Copyright:\n %s\n", copyright)
	fmt.Fprintf(f, "License: %s\n", license)
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Files:\n debian/*\n")
	fmt.Fprintf(f, "Copyright:\n %s %s <%s>\n", time.Now().Format("2006"), getDebianName(), getDebianEmail())
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

func writeDebianGbpConf(dir string) error {
	f, err := os.Create(filepath.Join(dir, "debian", "gbp.conf"))
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "[DEFAULT]\n")
	if dep14 {
		fmt.Fprintf(f, "debian-branch = debian/sid\n")
	}
	if pristineTar {
		fmt.Fprintf(f, "pristine-tar = True\n")
	}
	return nil
}

func writeDebianWatch(dir, gopkg, debsrc string) error {
	if strings.HasPrefix(gopkg, "github.com/") {
		f, err := os.Create(filepath.Join(dir, "debian", "watch"))
		if err != nil {
			return err
		}
		defer f.Close()

		fmt.Fprintf(f, "version=4\n")
		fmt.Fprintf(f, `opts=filenamemangle=s/.+\/v?(\d\S*)\.tar\.gz/%s-\$1\.tar\.gz/,\`+"\n", debsrc)
		fmt.Fprintf(f, `uversionmangle=s/(\d)[_\.\-\+]?(RC|rc|pre|dev|beta|alpha)[.]?(\d*)$/\$1~\$2\$3/ \`+"\n")
		fmt.Fprintf(f, `  https://%s/tags .*/v?(\d\S*)\.tar\.gz`+"\n", gopkg)
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
