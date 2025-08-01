package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	// describeRegexp parses the count and revision part of the “git describe --long” output.
	describeRegexp = regexp.MustCompile(`-\d+-g([0-9a-f]+)\s*$`)

	// semverRegexp checks if a string is a valid Go semver,
	// from https://semver.org/#is-there-a-suggested-regular-expression-regex-to-check-a-semver-string
	// with leading "v" added.
	semverRegexp = regexp.MustCompile(`^v(?P<major>0|[1-9]\d*)\.(?P<minor>0|[1-9]\d*)\.(?P<patch>0|[1-9]\d*)(?:-(?P<prerelease>(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+(?P<buildmetadata>[0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

	// uversionPrereleaseRegexp checks for upstream pre-release
	// so that '-' can be replaced with '~' in pkgVersionFromGit.
	// To be kept in sync with the regexp portion of uversionmanglePattern in template.go
	uversionPrereleaseRegexp = regexp.MustCompile(`(\d)[_\.\-\+]?(RC|rc|pre|dev|beta|alpha)[.]?(\d*)$`)
)

// pkgVersionFromGit determines the actual version to be packaged
// from the git repository status and user preference.
// Besides returning the Debian upstream version, the "upstream" struct
// struct fields u.version, u.commitIsh, u.hasRelease and u.isRelease
// are also set.
// `preferredRev` should be empty if there are no user preferences.
// TODO: also support other VCS
func pkgVersionFromGit(gitdir string, u *upstream, preferredRev string, forcePrerelease bool) (string, error) {
	var latestTag string
	var commitsAhead int

	var cmd *exec.Cmd // the temporary shell commands we execute

	// If the user specifies a valid tag as the preferred revision, that tag should be used without additional heuristics.
	if u.rr != nil {
		if out, err := u.rr.VCS.Tags(gitdir); err == nil && slices.Contains(out, preferredRev) {
			latestTag = preferredRev
		}
	}

	// Find @latest version tag (whether annotated or not) when the user
	// (1) does not specify a version tag, or
	// (2) specifies an invalid version tag.
	if len(latestTag) == 0 {
		cmd = exec.Command("git", "describe", "--abbrev=0", "--tags", "--exclude", "*/v*")
		cmd.Dir = gitdir
		if out, err := cmd.Output(); err == nil {
			latestTag = strings.TrimSpace(string(out))
		}
	}

	if len(latestTag) > 0 {
		u.hasRelease = true
		u.tag = latestTag
		log.Printf("Found latest tag %q", latestTag)

		if !semverRegexp.MatchString(latestTag) {
			log.Printf("WARNING: Latest tag %q is not a valid SemVer version\n", latestTag)
			// TODO: Enforce strict sementic versioning with leading "v"?
		}

		// Count number of commits since @latest version
		cmd = exec.Command("git", "rev-list", "--count", latestTag+"..HEAD")
		cmd.Dir = gitdir
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git rev-list: %w", err)
		}
		commitsAhead, err = strconv.Atoi(strings.TrimSpace(string(out)))
		if err != nil {
			return "", fmt.Errorf("parse commits ahead: %w", err)
		}

		if commitsAhead == 0 {
			// Equivalent to "git describe --exact-match --tags"
			log.Printf("Latest tag %q matches master", latestTag)
		} else {
			log.Printf("INFO: master is ahead of %q by %v commits", latestTag, commitsAhead)
		}

		u.commitIsh = latestTag

		// Mangle latestTag into Debian upstream_version
		// TODO: Move to function and write unit test?
		u.version = strings.TrimLeftFunc(
			uversionPrereleaseRegexp.ReplaceAllString(latestTag, "$1~$2$3"),
			func(r rune) bool {
				return !unicode.IsNumber(r)
			},
		)

		if forcePrerelease {
			log.Printf("INFO: Force packaging master (prerelease) as requested by user")
			// Fallthrough to package @master (prerelease)
		} else {
			u.isRelease = true
			return u.version, nil
		}
	}

	// Packaging @master (prerelease)

	// 1.0~rc1 < 1.0 < 1.0+b1, as per
	// https://www.debian.org/doc/manuals/maint-guide/first.en.html#namever
	mainVer := "0.0~"
	if u.hasRelease {
		mainVer = u.version + "+"
	}

	// Find committer date, UNIX timestamp
	cmd = exec.Command("git", "log", "--pretty=format:%ct", "-n1", "--no-show-signature")
	cmd.Dir = gitdir
	lastCommitUnixBytes, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git log: %w", err)
	}
	lastCommitUnix, err := strconv.ParseInt(strings.TrimSpace(string(lastCommitUnixBytes)), 0, 64)
	if err != nil {
		return "", fmt.Errorf("parse last commit date: %w", err)
	}

	// This results in an output like "v4.10.2-232-g9f107c8"
	cmd = exec.Command("git", "describe", "--long", "--tags")
	cmd.Dir = gitdir
	lastCommitHash := ""
	describeBytes, err := cmd.Output()
	if err != nil {
		// In case there are no tags at all, we just use the sha of the current commit
		cmd = exec.Command("git", "rev-parse", "--short", "HEAD")
		cmd.Dir = gitdir
		cmd.Stderr = os.Stderr
		revparseBytes, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git rev-parse: %w", err)
		}
		lastCommitHash = strings.TrimSpace(string(revparseBytes))
		u.commitIsh = lastCommitHash
	} else {
		submatches := describeRegexp.FindSubmatch(describeBytes)
		if submatches == nil {
			return "", fmt.Errorf("git describe output %q does not match expected format", string(describeBytes))
		}
		lastCommitHash = string(submatches[1])
		u.commitIsh = strings.TrimSpace(string(describeBytes))
	}
	u.version = fmt.Sprintf("%sgit%s.%s",
		mainVer,
		time.Unix(lastCommitUnix, 0).UTC().Format("20060102"),
		lastCommitHash)
	return u.version, nil
}
