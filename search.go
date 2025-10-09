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

type debianPackage struct {
	binary string
	source string
}

type ftpMasterApiResult struct {
	Binary        string `json:"binary"`
	MetadataValue string `json:"metadata_value"`
	Source        string `json:"source"`
}

type getGolangBinariesConfig struct {
	url string
}

type getGolangBinariesOption func(cfg *getGolangBinariesConfig)

func withGolangBinariesUrl(url string) getGolangBinariesOption {
	return func(cfg *getGolangBinariesConfig) {
		cfg.url = url
	}
}

func getGolangBinaries(opts ...getGolangBinariesOption) (map[string]debianPackage, error) {
	cfg := &getGolangBinariesConfig{url: golangBinariesURL}
	for _, opt := range opts {
		opt(cfg)
	}
	golangBinaries := make(map[string]debianPackage)

	resp, err := http.Get(cfg.url)
	if err != nil {
		return nil, fmt.Errorf("getting %q: %w", cfg.url, err)
	}
	defer resp.Body.Close()
	if got, want := resp.StatusCode, http.StatusOK; got != want {
		return nil, fmt.Errorf("unexpected HTTP status code: got %d, want %d", got, want)
	}
	var pkgs []ftpMasterApiResult
	if err := json.NewDecoder(resp.Body).Decode(&pkgs); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	for _, pkg := range pkgs {
		if !strings.HasSuffix(pkg.Binary, "-dev") {
			continue // skip -dbgsym packages etc.
		}
		for importPath := range strings.SplitSeq(pkg.MetadataValue, ",") {
			// XS-Go-Import-Path can be comma-separated and contain spaces.
			importPath := strings.TrimSpace(importPath)
			// importPath might be the empty string if XS-Go-Import-Path has a leading comma, trailing
			// comma, or extraneous internal comma.  It might also be empty if api.ftp-master.d.o returns
			// packages where XS-Go-Import-Path is explicitly set to the empty string or to a
			// whitespace-only string.
			if importPath == "" {
				continue
			}
			golangBinaries[importPath] = debianPackage{
				binary: pkg.Binary,
				source: pkg.Source,
			}
		}
	}
	return golangBinaries, nil
}

func execSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: %s search <pattern>
Uses Go's default regexp syntax (https://golang.org/pkg/regexp/syntax/)
Example: %s search 'debi.*'
`, os.Args[0], os.Args[0])
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

	for importPath, pkg := range golangBinaries {
		if pattern.MatchString(importPath) {
			fmt.Printf("%s: %s\n", pkg.binary, importPath)
		}
	}
}
