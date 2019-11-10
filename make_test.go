package main

import (
	"testing"
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
		in := normalizeDebianProgramName(tt.in)
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

func TestNormalizeDebianProgramName(t *testing.T) {
	for _, tt := range miscName {
		s := normalizeDebianProgramName(tt.in)
		if s != tt.out {
			t.Errorf("normalizeDebianProgramName(%q) => %q, want %q", tt.in, s, tt.out)
		}
	}
}

var nameFromGoPkg = []struct {
	in  string
	t   packageType
	out string
}{
	{"github.com/Debian/dh-make-golang", typeProgram, "dh-make-golang"},
	{"github.com/Debian/DH-make-golang", typeGuess, "golang-github-debian-dh-make-golang"},
	{"github.com/Debian/dh_make_golang", typeGuess, "golang-github-debian-dh-make-golang"},
}

func TestDebianNameFromGopkg(t *testing.T) {
	for _, tt := range nameFromGoPkg {
		s := debianNameFromGopkg(tt.in, tt.t, false)
		if s != tt.out {
			t.Errorf("debianNameFromGopkg(%q) => %q, want %q", tt.in, s, tt.out)
		}
	}
}
