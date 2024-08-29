package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseDebianControlDependencies(t *testing.T) {
	f := `Source: terminews
Maintainer: Debian Go Packaging Team <team+pkg-go@tracker.debian.org>
Uploaders:
 Aloïs Micard <alois@micard.lu>,
Section: news
Testsuite: autopkgtest-pkg-go
Priority: optional
Build-Depends:
 debhelper-compat (= 13),
 dh-sequence-golang,
 golang-any,
 golang-github-advancedlogic-goose-dev,
 golang-github-fatih-color-dev,
 golang-github-jroimartin-gocui-dev,
 golang-github-mattn-go-sqlite3-dev,
 golang-github-mmcdole-gofeed-dev,
Standards-Version: 4.7.0
Vcs-Browser: https://salsa.debian.org/go-team/packages/terminews
Vcs-Git: https://salsa.debian.org/go-team/packages/terminews.git
Homepage: https://github.com/antavelos/terminews
Rules-Requires-Root: no
XS-Go-Import-Path: github.com/antavelos/terminews

Package: terminews
Architecture: any
Depends:
 ${misc:Depends},
 ${shlibs:Depends},
Static-Built-Using:
 ${misc:Static-Built-Using},
Description: read your RSS feeds from your terminal
 Terminews is a terminal based application (TUI)
 that allows you to manage RSS resources and display their news feeds.
`
	tmpDir, err := os.MkdirTemp("", "dh-make-golang")
	if err != nil {
		t.Fatalf("Could not create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.MkdirAll(filepath.Join(tmpDir, "dummy-package", "debian"), 0750); err != nil {
		t.Fatalf("Could not create dummy Debian package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "dummy-package", "debian", "control"), []byte(f), 0640); err != nil {
		t.Fatalf("Could not create dummy Debian package: %v", err)
	}

	deps, err := parseDebianControlDependencies(filepath.Join(tmpDir, "dummy-package"))
	if err != nil {
		t.Fatalf("Could not parse Debian package dependencies: %v", err)

	}

	want := []dependency{
		{
			importPath:  "",
			packageName: "golang-github-advancedlogic-goose-dev",
		},
		{
			importPath:  "",
			packageName: "golang-github-fatih-color-dev",
		}, {
			importPath:  "",
			packageName: "golang-github-jroimartin-gocui-dev",
		},
		{
			importPath:  "",
			packageName: "golang-github-mattn-go-sqlite3-dev",
		},
		{
			importPath:  "",
			packageName: "golang-github-mmcdole-gofeed-dev",
		},
	}

	if !reflect.DeepEqual(deps, want) {
		t.Fatalf("Wrong dependencies returned (got %v want %v)", deps, want)
	}
}

func TestParseGoModDependencies(t *testing.T) {
	f := `module github.com/Debian/dh-make-golang

go 1.16

require (
	github.com/charmbracelet/glamour v0.3.0
	github.com/google/go-github/v60 v60.0.0
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79
)`
	tmpDir, err := os.MkdirTemp("", "dh-make-golang")
	if err != nil {
		t.Fatalf("Could not create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.MkdirAll(filepath.Join(tmpDir, "dummy-package"), 0750); err != nil {
		t.Fatalf("Could not create dummy Debian package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "dummy-package", "go.mod"), []byte(f), 0640); err != nil {
		t.Fatalf("Could not create dummy Debian package: %v", err)
	}

	deps, err := parseGoModDependencies(filepath.Join(tmpDir, "dummy-package"), map[string]string{
		"github.com/charmbracelet/glamour": "golang-github-charmbracelet-glamour-dev",
		"github.com/google/go-github":      "golang-github-google-go-github-dev",
		"github.com/gregjones/httpcache":   "golang-github-gregjones-httpcache-dev",
	})
	if err != nil {
		t.Fatalf("Could not parse go.mod dependencies: %v", err)

	}

	want := []dependency{
		{
			importPath:  "github.com/charmbracelet/glamour",
			packageName: "golang-github-charmbracelet-glamour-dev",
		},
		{
			importPath:  "github.com/google/go-github",
			packageName: "golang-github-google-go-github-dev",
		}, {
			importPath:  "github.com/gregjones/httpcache",
			packageName: "golang-github-gregjones-httpcache-dev",
		},
	}

	if !reflect.DeepEqual(deps, want) {
		t.Fatalf("Wrong dependencies returned (got %v want %v)", deps, want)
	}
}
