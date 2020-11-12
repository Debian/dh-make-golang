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

func TestSnapshotVersion(t *testing.T) {
	os.Setenv("TZ", "UTC")
	defer os.Unsetenv("TZ")

	tempdir, err := ioutil.TempDir("", "dh-make-golang")
	if err != nil {
		t.Fatalf("Could not create temp dir: %v", err)
	}
	defer os.RemoveAll(tempdir)

	tempfile := filepath.Join(tempdir, "test")
	if err := ioutil.WriteFile(tempfile, []byte("testcase"), 0644); err != nil {
		t.Fatalf("Could not write temp file %q: %v", tempfile, err)
	}

	gitCmdOrFatal(t, tempdir, "init")
	gitCmdOrFatal(t, tempdir, "config", "user.email", "unittest@example.com")
	gitCmdOrFatal(t, tempdir, "config", "user.name", "Unit Test")
	gitCmdOrFatal(t, tempdir, "add", "test")
	cmd := exec.Command("git", "commit", "-a", "-m", "initial commit")
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE=2015-04-20T11:22:33")
	cmd.Dir = tempdir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Could not run %v: %v", cmd.Args, err)
	}

	var u upstream
	got, err := pkgVersionFromGit(tempdir, &u, false)
	if err != nil {
		t.Fatalf("Determining package version from git failed: %v", err)
	}
	if want := "0.0~git20150420."; !strings.HasPrefix(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}

	gitCmdOrFatal(t, tempdir, "tag", "-a", "v1", "-m", "release v1")

	got, err = pkgVersionFromGit(tempdir, &u, false)
	if err != nil {
		t.Fatalf("Determining package version from git failed: %v", err)
	}
	if want := "1"; got != want {
		t.Logf("got %q, want %q", got, want)
	}

	if err := ioutil.WriteFile(tempfile, []byte("testcase 2"), 0644); err != nil {
		t.Fatalf("Could not write temp file %q: %v", tempfile, err)
	}

	cmd = exec.Command("git", "commit", "-a", "-m", "first change")
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE=2015-05-07T11:22:33")
	cmd.Dir = tempdir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Could not run %v: %v", cmd.Args, err)
	}

	got, err = pkgVersionFromGit(tempdir, &u, false)
	if err != nil {
		t.Fatalf("Determining package version from git failed: %v", err)
	}
	if want := "1+git20150507.1."; !strings.HasPrefix(got, want) {
		t.Logf("got %q, want %q", got, want)
	}

	if err := ioutil.WriteFile(tempfile, []byte("testcase 3"), 0644); err != nil {
		t.Fatalf("Could not write temp file %q: %v", tempfile, err)
	}

	cmd = exec.Command("git", "commit", "-a", "-m", "second change")
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE=2015-05-08T11:22:33")
	cmd.Dir = tempdir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Could not run %v: %v", cmd.Args, err)
	}

	got, err = pkgVersionFromGit(tempdir, &u, false)
	if err != nil {
		t.Fatalf("Determining package version from git failed: %v", err)
	}
	if want := "1+git20150508.2."; !strings.HasPrefix(got, want) {
		t.Logf("got %q, want %q", got, want)
	}
}
