package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParsePackageDependencies(t *testing.T) {
	f := `
Source: terminews
Maintainer: Debian Go Packaging Team <team+pkg-go@tracker.debian.org>
Uploaders:
 Aloïs Micard <alois@micard.lu>,
Section: news
Testsuite: autopkgtest-pkg-go
Priority: optional
Build-Depends:
 debhelper-compat (= 13),
 dh-golang,
 golang-any,
 golang-github-advancedlogic-goose-dev,
 golang-github-fatih-color-dev,
 golang-github-jroimartin-gocui-dev,
 golang-github-mattn-go-sqlite3-dev,
 golang-github-mmcdole-gofeed-dev,
Standards-Version: 4.5.1
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
Built-Using:
 ${misc:Built-Using},
Description: read your RSS feeds from your terminal
 Terminews is a terminal based application (TUI)
 that allows you to manage RSS resources and display their news feeds.
`
	tmpDir, err := ioutil.TempDir("", "dh-make-golang")
	if err != nil {
		t.Fatalf("Could not create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.MkdirAll(filepath.Join(tmpDir, "dummy-package", "debian"), 0750); err != nil {
		t.Fatalf("Could not create dummy Debian package: %v", err)
	}
	if err := ioutil.WriteFile(filepath.Join(tmpDir, "dummy-package", "debian", "control"), []byte(f), 0640); err != nil {
		t.Fatalf("Could not create dummy Debian package: %v", err)
	}

	deps, err := parsePackageDependencies(filepath.Join(tmpDir, "dummy-package"))
	if err != nil {
		t.Fatalf("Could not parse Debian packag dependencies: %v", err)

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
