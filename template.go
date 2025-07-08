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
		// If upstream debian dir exists, try to move it aside, and then below.
		if err := os.Rename(filepath.Join(dir, "debian"), filepath.Join(dir, "upstream_debian")); err != nil {
			return fmt.Errorf("rename debian/ to upstream_debian/: %w", err)
		} else { // Second attempt to create template debian dir, after moving upstream dir aside.
			if err := os.Mkdir(filepath.Join(dir, "debian"), 0755); err != nil {
				return fmt.Errorf("mkdir debian/: %w", err)
			}
			if err := os.Rename(filepath.Join(dir, "upstream_debian"), filepath.Join(dir, "debian/upstream_debian")); err != nil {
				return fmt.Errorf("move upstream_debian into debian/: %w", err)
			}
			log.Printf("WARNING: Upstream debian/ dir found, and relocated to debian/upstream_debian/\n")
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "debian", "source"), 0755); err != nil {
		return fmt.Errorf("mkdir debian/source/: %w", err)
	}

	if err := writeDebianGitIgnore(dir, debLib, debProg, pkgType); err != nil {
		return fmt.Errorf("write debian/.gitignore: %w", err)
	}
	if err := writeDebianChangelog(dir, debsrc, debversion); err != nil {
		return fmt.Errorf("write changelog: %w", err)
	}
	if err := writeDebianControl(dir, gopkg, debsrc, debLib, debProg, pkgType, dependencies); err != nil {
		return fmt.Errorf("write control: %w", err)
	}
	if err := writeDebianCopyright(dir, gopkg, u.vendorDirs, u.hasGodeps); err != nil {
		return fmt.Errorf("write copyright: %w", err)
	}
	if err := writeDebianRules(dir, pkgType); err != nil {
		return fmt.Errorf("write rules: %w", err)
	}

	var repack bool = len(u.vendorDirs) > 0 || u.hasGodeps
	if err := writeDebianWatch(dir, gopkg, debsrc, u.hasRelease, repack); err != nil {
		return fmt.Errorf("write watch: %w", err)
	}

	if err := writeDebianSourceFormat(dir); err != nil {
		return fmt.Errorf("write source/format: %w", err)
	}
	if err := writeDebianPackageInstall(dir, debLib, debProg, pkgType); err != nil {
		return fmt.Errorf("write install: %w", err)
	}
	if err := writeDebianUpstreamMetadata(dir, gopkg); err != nil {
		return fmt.Errorf("write upstream metadata: %w", err)
	}

	if err := writeDebianGbpConf(dir, dep14, pristineTar); err != nil {
		return fmt.Errorf("write gbp conf: %w", err)
	}

	if err := writeDebianGitLabCI(dir); err != nil {
		return fmt.Errorf("write GitLab CI: %w", err)
	}

	return nil
}

func writeDebianGitIgnore(dir, debLib, debProg string, pkgType packageType) error {
	f, err := os.Create(filepath.Join(dir, "debian", ".gitignore"))
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "*.debhelper\n")
	fmt.Fprintf(f, "*.log\n")
	fmt.Fprintf(f, "*.substvars\n")
	fmt.Fprintf(f, "/.debhelper/\n")
	fmt.Fprintf(f, "/debhelper-build-stamp\n")
	fmt.Fprintf(f, "/files\n")

	switch pkgType {
	case typeLibrary:
		fmt.Fprintf(f, "/%s/\n", debLib)
	case typeProgram:
		fmt.Fprintf(f, "/%s/\n", debProg)
	case typeLibraryProgram:
		fallthrough
	case typeProgramLibrary:
		fmt.Fprintf(f, "/%s/\n", debLib)
		fmt.Fprintf(f, "/%s/\n", debProg)
	default:
		log.Fatalf("Invalid pkgType %d in writeDebianGitIgnore(), aborting", pkgType)
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
	fmt.Fprintln(f, longdescription)
}

func addLibraryPackage(f *os.File, gopkg, debLib string, dependencies []string) {
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Package: %s\n", debLib)
	fmt.Fprintf(f, "Architecture: all\n")
	fmt.Fprintf(f, "Multi-Arch: foreign\n")
	deps := dependencies
	sort.Strings(deps)
	deps = append(deps, "${misc:Depends}")
	fprintfControlField(f, "Depends", deps)
	addDescription(f, gopkg, "(library)")
}

func addProgramPackage(f *os.File, gopkg, debProg string) {
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Package: %s\n", debProg)
	fmt.Fprintf(f, "Section: TODO\n")
	fmt.Fprintf(f, "Architecture: any\n")
	deps := []string{"${misc:Depends}", "${shlibs:Depends}"}
	fprintfControlField(f, "Depends", deps)
	fmt.Fprintf(f, "Static-Built-Using: ${misc:Static-Built-Using}\n")
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
	fmt.Fprintf(f, "Section: golang\n")
	fmt.Fprintf(f, "Priority: optional\n")
	fmt.Fprintf(f, "Maintainer: Debian Go Packaging Team <team+pkg-go@tracker.debian.org>\n")
	fprintfControlField(f, "Uploaders", []string{getDebianName() + " <" + getDebianEmail() + ">"})

	builddeps := append([]string{
		"debhelper-compat (= 13)",
		"dh-sequence-golang",
		"golang-any"},
		dependencies...)
	sort.Strings(builddeps)
	fprintfControlField(f, "Build-Depends", builddeps)

	fmt.Fprintf(f, "Testsuite: autopkgtest-pkg-go\n")
	fmt.Fprintf(f, "Standards-Version: 4.7.0\n")
	fmt.Fprintf(f, "Vcs-Browser: https://salsa.debian.org/go-team/packages/%s\n", debsrc)
	fmt.Fprintf(f, "Vcs-Git: https://salsa.debian.org/go-team/packages/%s.git\n", debsrc)
	fmt.Fprintf(f, "Homepage: %s\n", getHomepageForGopkg(gopkg))
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
	fmt.Fprintf(f, "Source: %s\n", getHomepageForGopkg(gopkg))
	fmt.Fprintf(f, "Upstream-Name: %s\n", filepath.Base(gopkg))
	fmt.Fprintf(f, "Upstream-Contact: TODO\n")
	if len(vendorDirs) > 0 || hasGodeps {
		fmt.Fprintf(f, "Files-Excluded:\n")
		for _, dir := range vendorDirs {
			fmt.Fprintf(f, "%s%s\n", indent, dir)
		}
		if hasGodeps {
			fmt.Fprintf(f, "%sGodeps/_workspace\n", indent)
		}
	}
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Files:%s *\n", linebreak)
	fmt.Fprintf(f, "Copyright:%s %s\n", linebreak, copyright)
	fmt.Fprintf(f, "License: %s\n", license)
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "Files:%s debian/*\n", linebreak)
	fmt.Fprintf(f, "Copyright:%s %s %s <%s>\n", linebreak, time.Now().Format("2006"), getDebianName(), getDebianEmail())
	fmt.Fprintf(f, "License: %s\n", license)
	fmt.Fprintf(f, "Comment: Debian packaging is licensed under the same terms as upstream\n")
	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "License: %s\n", license)
	fmt.Fprint(f, fulltext)
	fmt.Fprint(f, "\n")

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
	fmt.Fprintf(f, "\tdh $@ --builddirectory=_build --buildsystem=golang\n")
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
		fmt.Fprintf(f, "debian-branch = debian/latest\n")
		fmt.Fprintf(f, "upstream-branch = upstream/latest\n")
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

	filenamemanglePattern := `s%(?:.*?)?v?(\d[\d.]*)\.tar\.gz%@PACKAGE@-$1.tar.gz%`
	uversionmanglePattern := `s/(\d)[_\.\-\+]?(RC|rc|pre|dev|beta|alpha)[.]?(\d*)$/$1~$2$3/`

	if hasRelease {
		log.Printf("Setting debian/watch to track release tarball")
		fmt.Fprint(f, "version=4\n")
		fmt.Fprint(f, `opts="filenamemangle=`+filenamemanglePattern+`,\`+"\n")
		fmt.Fprint(f, `      uversionmangle=`+uversionmanglePattern)
		if repack {
			fmt.Fprint(f, `,\`+"\n")
			fmt.Fprint(f, `      dversionmangle=s/\+ds\d*$//,repacksuffix=+ds1`)
		}
		fmt.Fprint(f, `" \`+"\n")
		fmt.Fprintf(f, `  https://%s/%s/%s/tags .*/v?(\d\S*)\.tar\.gz debian`+"\n", host, owner, repo)
	} else {
		log.Printf("Setting debian/watch to track git HEAD")
		fmt.Fprint(f, "version=4\n")
		fmt.Fprint(f, `opts="mode=git, pgpmode=none`)
		if repack {
			fmt.Fprint(f, `,\`+"\n")
			fmt.Fprint(f, `      dversionmangle=s/\+ds\d*$//,repacksuffix=+ds1`)
		}
		fmt.Fprint(f, `" \`+"\n")
		fmt.Fprintf(f, `  https://%s/%s/%s.git \`+"\n", host, owner, repo)
		fmt.Fprint(f, "  HEAD debian\n")

		// Anticipate that upstream would eventually switch to tagged releases
		fmt.Fprint(f, "\n")
		fmt.Fprint(f, "# Use the following when upstream starts to tag releases:\n")
		fmt.Fprint(f, "#\n")
		fmt.Fprint(f, "#version=4\n")
		fmt.Fprint(f, `#opts="filenamemangle=`+filenamemanglePattern+`,\`+"\n")
		fmt.Fprint(f, `#      uversionmangle=`+uversionmanglePattern)
		if repack {
			fmt.Fprint(f, `,\`+"\n")
			fmt.Fprint(f, `#      dversionmangle=s/\+ds\d*$//,repacksuffix=+ds1`)
		}
		fmt.Fprint(f, `" \`+"\n")
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
	const gitlabciymlTmpl = `# DO NOT MODIFY
# This file was automatically generated from the authoritative copy at:
# https://salsa.debian.org/go-team/infra/pkg-go-tools/blob/master/config/gitlabciyml.go
---
stages:
  - test
  - package

include:
  - project: go-team/infra/pkg-go-tools
    ref: master
    file: pipeline/test-archive.yml
    # Run the Go team CI only in the go-team project that has access to GitLab
    # CI runners tagged 'go-ci'
    rules:
      - if: $CI_PROJECT_ROOT_NAMESPACE  == "go-team"

Salsa CI:
  stage: package
  trigger:
    include:
      - project: salsa-ci-team/pipeline
        ref: master
        file: recipes/debian.yml
    strategy: depend
  rules:
    # Do not create a pipeline for tags unless SALSA_CI_ENABLE_PIPELINE_ON_TAGS is set
    - if: $CI_COMMIT_TAG != null && $SALSA_CI_ENABLE_PIPELINE_ON_TAGS !~ /^(1|yes|true)$/
      when: never
    # Avoid duplicated pipelines, do not run detached pipelines
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
      when: never
    # Run Salsa CI only if the Play button is pressed on the pipeline
    - if: $CI_PIPELINE_SOURCE == "push"
      when: manual
  variables:
    SALSA_CI_DISABLE_REPROTEST: 1 # Disable to save CI runner resources

# If Salsa CI is not running at
# https://salsa.debian.org/%{project_path}/-/pipelines, ensure that
# https://salsa.debian.org/%{project_path}/-/settings/ci_cd has in field "CI/CD
# configuration file" the same filename as this file.
#
# If Salsa CI is running, but first job is stuck because the project doesn't
# have any runners online assigned to it, ensure that
# https://salsa.debian.org/%{project_path}/-/settings/ci_cd has under "Runners"
# the setting for "Enable instance runners for this project" enabled.
`

	f, err := os.Create(filepath.Join(dir, "debian", "gitlab-ci.yml"))
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprint(f, gitlabciymlTmpl)

	return nil
}
