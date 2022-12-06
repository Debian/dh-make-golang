package main

import (
	"testing"

	"golang.org/x/tools/go/vcs"
)

var shortName = []struct {
	in  string
	out string
}{
	{"", "TODO"},
	{"d", "TODO"},
	{"d--", "TODO"},
}

func TestAcceptInput(t *testing.T) {
	for _, tt := range shortName {
		in := normalizeDebianPackageName(tt.in)
		if in != tt.out {
			t.Errorf("userInput(%q) => %q, want %q", tt.in, in, tt.out)
		}
	}
}

var miscName = []struct {
	in  string
	out string
}{
	{"dh-make-golang", "dh-make-golang"},
	{"DH-make-golang", "dh-make-golang"},
	{"dh_make_golang", "dh-make-golang"},
	{"dh_make*go&3*@@", "dh-makego3"},
	{"7h_make*go&3*@@", "7h-makego3"},
	{"7h_make*go&3*.@", "7h-makego3."},
	{"7h_make*go+3*.@", "7h-makego+3."},
}

func TestNormalizeDebianPackageName(t *testing.T) {
	for _, tt := range miscName {
		s := normalizeDebianPackageName(tt.in)
		if s != tt.out {
			t.Errorf("normalizeDebianPackageName(%q) => %q, want %q", tt.in, s, tt.out)
		}
	}
}

var nameFromGoPkg = []struct {
	in     string
	t      packageType
	custom string
	out    string
}{
	{"github.com/Debian/dh-make-golang", typeProgram, "", "dh-make-golang"},
	{"github.com/Debian/DH-make-golang", typeGuess, "", "golang-github-debian-dh-make-golang"},
	{"github.com/Debian/dh_make_golang", typeGuess, "", "golang-github-debian-dh-make-golang"},
	{"github.com/sean-/seed", typeGuess, "", "golang-github-sean--seed"},
	{"git.sr.ht/~sircmpwn/getopt", typeGuess, "", "golang-sourcehut-sircmpwn-getopt"},
	{"golang.org/x/term", typeLibrary, "", "golang-golang-x-term"},
	{"github.com/cli/cli", typeProgram, "gh", "gh"},
}

func TestDebianNameFromGopkg(t *testing.T) {
	for _, tt := range nameFromGoPkg {
		s := debianNameFromGopkg(tt.in, tt.t, tt.custom, false)
		if s != tt.out {
			t.Errorf("debianNameFromGopkg(%q) => %q, want %q", tt.in, s, tt.out)
		}
	}
}

var tarballUrl = []struct {
	repoRoot    string
	tag         string
	compression string
	url         string
}{
	{"https://github.com/Debian/dh-make-golang", "0.6.0", "gz", "https://github.com/Debian/dh-make-golang/archive/0.6.0.tar.gz"},
	{"https://github.com/Debian/dh-make-golang.git", "0.6.0", "gz", "https://github.com/Debian/dh-make-golang/archive/0.6.0.tar.gz"},
	{"https://gitlab.com/gitlab-org/labkit", "1.3.0", "gz", "https://gitlab.com/gitlab-org/labkit/-/archive/1.3.0/labkit-1.3.0.tar.gz"},
	{"https://git.sr.ht/~sircmpwn/getopt", "v1.0.0", "gz", "https://git.sr.ht/~sircmpwn/getopt/archive/v1.0.0.tar.gz"},
}

func TestUpstreamTarmballUrl(t *testing.T) {
	for _, tt := range tarballUrl {
		u := upstream{
			rr:          &vcs.RepoRoot{Repo: tt.repoRoot},
			compression: tt.compression,
			tag:         tt.tag,
		}

		url, _ := u.tarballUrl()
		if url != tt.url {
			t.Errorf("TestUpstreamTarmballUrl(%q) => %q, want %q", tt.repoRoot, url, tt.url)
		}
	}
}
