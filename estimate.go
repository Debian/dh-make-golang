package main

import (
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/vcs"
	"golang.org/x/tools/refactor/importgraph"
)

func get(gopath, repo string) error {
	done := make(chan struct{})
	defer close(done)
	go progressSize("go get", filepath.Join(gopath, "src"), done)

	// As per https://groups.google.com/forum/#!topic/golang-nuts/N5apfenE4m4,
	// the arguments to “go get” are packages, not repositories. Hence, we
	// specify “gopkg/...” in order to cover all packages.
	// As a concrete example, github.com/jacobsa/util is a repository we want
	// to package into a single Debian package, and using “go get -d
	// github.com/jacobsa/util” fails because there are no buildable go files
	// in the top level of that repository.
	cmd := exec.Command("go", "get", "-d", "-t", repo+"/...")
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		fmt.Sprintf("GOPATH=%s", gopath),
	}, passthroughEnv()...)
	return cmd.Run()
}

func removeVendor(gopath string) (found bool, _ error) {
	err := filepath.Walk(filepath.Join(gopath, "src"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil // skip non-directories
		}
		if info.Name() != "vendor" {
			return nil
		}
		found = true
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		return filepath.SkipDir
	})
	return found, err
}

func estimate(importpath string) error {
	// construct a separate GOPATH in a temporary directory
	gopath, err := ioutil.TempDir("", "dh-make-golang")
	if err != nil {
		return err
	}
	defer os.RemoveAll(gopath)

	if err := get(gopath, importpath); err != nil {
		return err
	}

	found, err := removeVendor(gopath)
	if err != nil {
		return err
	}

	if found {
		// Fetch un-vendored dependencies
		if err := get(gopath, importpath); err != nil {
			return err
		}
	}

	// Remove standard lib packages
	cmd := exec.Command("go", "list", "std")
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		fmt.Sprintf("GOPATH=%s", gopath),
	}, passthroughEnv()...)

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("%v: %v", cmd.Args, err)
	}
	stdlib := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		stdlib[line] = true
	}

	stdlib["C"] = true // would fail resolving anyway

	// Filter out all already-packaged ones:
	golangBinaries, err := getGolangBinaries()
	if err != nil {
		return nil
	}

	build.Default.GOPATH = gopath
	forward, _, errors := importgraph.Build(&build.Default)
	if len(errors) > 0 {
		lines := make([]string, 0, len(errors))
		for importPath, err := range errors {
			lines = append(lines, fmt.Sprintf("%s: %v", importPath, err))
		}
		return fmt.Errorf("could not load packages: %v", strings.Join(lines, "\n"))
	}

	var lines []string
	seen := make(map[string]bool)
	rrseen := make(map[string]bool)
	node := func(importPath string, indent int) {
		rr, err := vcs.RepoRootForImportPath(importPath, false)
		if err != nil {
			log.Printf("Could not determine repo path for import path %q: %v\n", importPath, err)
			return
		}
		if rrseen[rr.Root] {
			return
		}
		rrseen[rr.Root] = true
		if _, ok := golangBinaries[rr.Root]; ok {
			return // already packaged in Debian
		}
		lines = append(lines, fmt.Sprintf("%s%s", strings.Repeat("  ", indent), rr.Root))
	}
	var visit func(x string, indent int)
	visit = func(x string, indent int) {
		if seen[x] {
			return
		}
		seen[x] = true
		if !stdlib[x] {
			node(x, indent)
		}
		for y := range forward[x] {
			visit(y, indent+1)
		}
	}

	keys := make([]string, 0, len(forward))
	for key := range forward {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !strings.HasPrefix(key, importpath) {
			continue
		}
		if seen[key] {
			continue // already covered in a previous visit call
		}
		visit(key, 0)
	}

	if len(lines) == 0 {
		log.Printf("%s is already fully packaged in Debian", importpath)
		return nil
	}
	log.Printf("Bringing %s to Debian requires packaging the following Go packages:", importpath)
	for _, line := range lines {
		fmt.Println(line)
	}

	return nil
}

func execEstimate(args []string) {
	fs := flag.NewFlagSet("estimate", flag.ExitOnError)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s estimate <go-package-importpath>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Estimates the work necessary to bring <go-package-importpath> into Debian\n"+
			"by printing all currently unpacked repositories.\n")
		fmt.Fprintf(os.Stderr, "Example: %s estimate github.com/Debian/dh-make-golang\n", os.Args[0])
	}

	err := fs.Parse(args)
	if err != nil {
		log.Fatal(err)
	}

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	// TODO: support the -git_revision flag

	if err := estimate(fs.Arg(0)); err != nil {
		log.Fatal(err)
	}
}
