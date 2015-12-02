package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// describeRegexp parses the count and revision part of the “git describe --long” output.
	describeRegexp = regexp.MustCompile(`-(\d+)-g([0-9a-f]+)\s*$`)
)

// TODO: also support other VCS
func pkgVersionFromGit(gitdir string) (string, error) {
	cmd := exec.Command("git", "describe", "--exact-match")
	cmd.Dir = gitdir
	if tag, err := cmd.Output(); err == nil {
		version := strings.TrimSpace(string(tag))
		if strings.HasPrefix(version, "v") {
			version = version[1:]
		}
		return version, nil
	}

	cmd = exec.Command("git", "log", "--pretty=format:%ct", "-n1")
	cmd.Dir = gitdir
	lastCommitUnixBytes, err := cmd.Output()
	if err != nil {
		return "", err
	}
	lastCommitUnix, err := strconv.ParseInt(strings.TrimSpace(string(lastCommitUnixBytes)), 0, 64)
	if err != nil {
		return "", err
	}

	// Find the most recent tag (whether annotated or not)
	cmd = exec.Command("git", "describe", "--abbrev=0", "--tags")
	cmd.Dir = gitdir
	// 1.0~rc1 < 1.0 < 1.0+b1, as per
	// https://www.debian.org/doc/manuals/maint-guide/first.en.html#namever
	lastTag := "0.0~"
	if lastTagBytes, err := cmd.Output(); err == nil {
		lastTag = strings.TrimSpace(string(lastTagBytes))

		// Compare with the most recent annotated tag
		foundLightweightTag := false
		cmd = exec.Command("git", "describe", "--abbrev=0")
		cmd.Dir = gitdir
		if lastAnnotatedTagBytes, err := cmd.Output(); err == nil {
			lastAnnotatedTag := strings.TrimSpace(string(lastAnnotatedTagBytes))
			if lastTag != lastAnnotatedTag {
				log.Printf("WARNING: Lightweight tag %q found, but the most recent annotated tag is %q\n", lastTag, lastAnnotatedTag)
				foundLightweightTag = true
			}
		} else {
			log.Printf("WARNING: Annotated tag not found, using lightweight tag %q\n", lastTag)
			foundLightweightTag = true
		}

		if foundLightweightTag {
			log.Printf("    Lightweight tags (created by e.g. \"git tag %s\"", lastTag)
			log.Printf("    with no flag) are problematic because, among other\n")
			log.Printf("    things, they are ignored by \"git describe\" by default.\n")
			log.Printf("    Please suggest that the upstream replaces the\n")
			log.Printf("    lightweight tags with annotated ones.  See\n")
			log.Printf("    https://github.com/russross/blackfriday/issues/196\n")
			log.Printf("    for details.\n")
			log.Printf("\n")
		}

		lastTag += "+"
		if strings.HasPrefix(lastTag, "v") {
			lastTag = lastTag[1:]
		}
	}

	// This results in an output like 4.10.2-232-g9f107c8
	cmd = exec.Command("git", "describe", "--long", "--tags")
	cmd.Dir = gitdir
	describeBytes, err := cmd.Output()
	if err != nil {
		// In case there are no tags at all, we need to pass --all, but we
		// cannot use --all unconditionally because then git will describe
		// e.g. heads/master instead of tags.
		cmd = exec.Command("git", "describe", "--long", "--all")
		cmd.Dir = gitdir
		cmd.Stderr = os.Stderr
		describeBytes, err = cmd.Output()
		if err != nil {
			return "", err
		}
	}
	submatches := describeRegexp.FindSubmatch(describeBytes)
	if submatches == nil {
		return "", fmt.Errorf("git describe output %q does not match expected format", string(describeBytes))
	}
	version := fmt.Sprintf("%sgit%s.%s.%s",
		lastTag,
		time.Unix(lastCommitUnix, 0).UTC().Format("20060102"),
		string(submatches[1]),
		string(submatches[2]))
	return version, nil
}
