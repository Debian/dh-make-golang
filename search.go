package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
)

const (
	golangBinariesURL = "https://api.ftp-master.debian.org/binary/by_metadata/Go-Import-Path"
)

func getGolangBinaries() (map[string]string, error) {
	golangBinaries := make(map[string]string)

	resp, err := http.Get(golangBinariesURL)
	if err != nil {
		return nil, fmt.Errorf("getting %q: %v", golangBinariesURL, err)
	}
	if got, want := resp.StatusCode, http.StatusOK; got != want {
		return nil, fmt.Errorf("unexpected HTTP status code: got %d, want %d", got, want)
	}
	var pkgs []struct {
		Binary         string `json:"binary"`
		XSGoImportPath string `json:"metadata_value"`
		Source         string `json:"source"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pkgs); err != nil {
		return nil, err
	}
	for _, pkg := range pkgs {
		if !strings.HasSuffix(pkg.Binary, "-dev") {
			continue // skip -dbgsym packages etc.
		}
		for _, importPath := range strings.Split(pkg.XSGoImportPath, ",") {
			// XS-Go-Import-Path can be comma-separated and contain spaces.
			golangBinaries[strings.TrimSpace(importPath)] = pkg.Binary
		}
	}
	return golangBinaries, nil
}

func execSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s search <pattern>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Uses Go's default regexp syntax (https://golang.org/pkg/regexp/syntax/)\n")
		fmt.Fprintf(os.Stderr, "Example: %s search 'debi.*'\n", os.Args[0])
	}

	err := fs.Parse(args)
	if err != nil {
		log.Fatal(err)
	}

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	pattern, err := regexp.Compile(fs.Arg(0))
	if err != nil {
		log.Fatal(err)
	}

	golangBinaries, err := getGolangBinaries()
	if err != nil {
		log.Fatal(err)
	}

	for importPath, binary := range golangBinaries {
		if pattern.MatchString(importPath) {
			fmt.Printf("%s: %s\n", binary, importPath)
		}
	}
}
