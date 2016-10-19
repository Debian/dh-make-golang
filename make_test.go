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
			t.Errorf("userInput(%q) => %q, want %q", tt.in, tt.out)
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
			t.Errorf("normalizeDebianProgramName(%q) => %q, want %q", tt.in, tt.out)
		}
	}
}

var nameFromGoPkg = []struct {
	in  string
	t   string
	out string
}{
	{"github.com/dh-make-golang", "program", "dh-make-golang"},
	{"github.com/DH-make-golang", "", "golang-github-dh-make-golang"},
	{"github.com/dh_make_golang", "", "golang-github-dh-make-golang"},
}

func TestDebianNameFromGopkg(t *testing.T) {
	for _, tt := range nameFromGoPkg {
		s := debianNameFromGopkg(tt.in, tt.t)
		if s != tt.out {
			t.Errorf("debianNameFromGopkg(%q) => %q, want %q", tt.in, s, tt.out)
		}
	}
}
