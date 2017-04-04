package main

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitCmdOrFatal(t *testing.T, tempdir string, arg ...string) {
	cmd := exec.Command("git", arg...)
	cmd.Dir = tempdir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Could not run %v: %v", cmd.Args, err)
	}
}

func modifyFile(t *testing.T, tempfile string, text string) {
	if err := ioutil.WriteFile(tempfile, []byte(text), 0644); err != nil {
		t.Fatalf("Could not write temp file %q: %v", tempfile, err)
	}
}

func gitCommit(t *testing.T, tempdir string, message string, timestamp string) {
	cmd := exec.Command("git", "commit", "-a", "-m", message)
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+timestamp)
	cmd.Dir = tempdir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Could not run %v: %v", cmd.Args, err)
	}
}

func TestSnapshotVersion(t *testing.T) {
	tempdir, err := ioutil.TempDir("", "dh-make-golang")
	if err != nil {
		t.Fatalf("Could not create temp dir: %v", err)
	}
	defer os.RemoveAll(tempdir)

	tempfile := filepath.Join(tempdir, "test")
	modifyFile(t, tempfile, "testcase")

	// set up the test repository
	gitCmdOrFatal(t, tempdir, "init")
	gitCmdOrFatal(t, tempdir, "config", "user.email", "unittest@example.com")
	gitCmdOrFatal(t, tempdir, "config", "user.name", "Unit Test")
	gitCmdOrFatal(t, tempdir, "add", "test")

	gitCommit(t, tempdir, "initial commit", "2015-04-20T11:22:33")

	if got, err := pkgVersionFromGit(tempdir, ""); nil != err {
		t.Fatalf("Determining package version from git failed: %v", err)
	} else if want := "0.0~git20150420.0."; !strings.HasPrefix(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}

	gitCmdOrFatal(t, tempdir, "tag", "-a", "v1", "-m", "release v1")

	if got, err := pkgVersionFromGit(tempdir, ""); nil != err {
		t.Fatalf("Determining package version from git failed: %v", err)
	} else if want := "1"; got != want {
		t.Logf("got %q, want %q", got, want)
	}

	modifyFile(t, tempfile, "testcase 2")
	gitCommit(t, tempdir, "first change", "2015-05-07T11:22:33")

	gitCmdOrFatal(t, tempdir, "tag", "-a", "v7.8", "-m", "release v7.8")

	// check exact version
	if got, err := pkgVersionFromGit(tempdir, ""); nil != err {
		t.Fatalf("Determining package version from git failed: %v", err)
	} else if want := "7.8"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	modifyFile(t, tempfile, "testcase 3")
	gitCommit(t, tempdir, "second change", "2015-05-08T11:22:33")

	if got, err := pkgVersionFromGit(tempdir, ""); nil != err {
		t.Fatalf("Determining package version from git failed: %v", err)
	} else if want := "7.8+git20150508.1."; !strings.HasPrefix(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}

	// add a spurious tag
	gitCmdOrFatal(t, tempdir, "tag", "-a", "foo55.55", "-m", "confusing tag")

	modifyFile(t, tempfile, "testcase 4")
	gitCommit(t, tempdir, "third change", "2015-05-09T11:22:33")

	// a second spurious tag
	gitCmdOrFatal(t, tempdir, "tag", "-a", "foo99.99", "-m", "confusing tag")

	// this gets the default tag (the second spurious one)
	if got, err := pkgVersionFromGit(tempdir, ""); nil != err {
		t.Fatalf("Determining package version from git failed: %v", err)
	} else if want := "foo99.99"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// this gets a specific tag
	if got, err := pkgVersionFromGit(tempdir, "v*"); nil != err {
		t.Fatalf("Determining package version from git failed: %v", err)
	} else if want := "7.8-2-g"; !strings.HasPrefix(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}

	// repeat to check that version suffix -2 -> -3
	modifyFile(t, tempfile, "testcase 5")
	gitCommit(t, tempdir, "fourth change", "2015-05-10T11:22:33")

	if got, err := pkgVersionFromGit(tempdir, "v*"); nil != err {
		t.Fatalf("Determining package version from git failed: %v", err)
	} else if want := "7.8-3-g"; !strings.HasPrefix(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}

	// repeat check wth exact version number
	if got, err := pkgVersionFromGit(tempdir, "v7.8"); nil != err {
		t.Fatalf("Determining package version from git failed: %v", err)
	} else if want := "7.8-3-g"; !strings.HasPrefix(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}

	// try a strange version tag
	modifyFile(t, tempfile, "testcase 6")
	gitCommit(t, tempdir, "fifth change", "2015-05-10T12:22:33")

	gitCmdOrFatal(t, tempdir, "tag", "-a", "VrSn67.12", "-m", "strange version tag")

	if got, err := pkgVersionFromGit(tempdir, "VrSn*"); nil != err {
		t.Fatalf("Determining package version from git failed: %v", err)
	} else if want := "67.12"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// check that strange version gets the -N-g* suffix after a change
	modifyFile(t, tempfile, "testcase 7")
	gitCommit(t, tempdir, "sixth change", "2015-05-11T12:22:33")

	if got, err := pkgVersionFromGit(tempdir, "VrSn*"); nil != err {
		t.Fatalf("Determining package version from git failed: %v", err)
	} else if want := "67.12-1-g"; !strings.HasPrefix(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
