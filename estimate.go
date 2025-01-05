package main

import (
	"flag"
	"fmt"
	"go/build"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/vcs"
	"golang.org/x/tools/refactor/importgraph"
)

func clone(srcdir, repo string) (string, error) {
	done := make(chan struct{})
	defer close(done)
	go progressSize("vcs clone", srcdir, done)

	// Get the sources of the module in a temporary dir to be able to run
	// go get in module mode, as the gopath mode has been removed in latest
	// version of Go.
	rr, err := vcs.RepoRootForImportPath(repo, false)
	if err != nil {
		return "", fmt.Errorf("get repo root: %w", err)
	}
	dir := filepath.Join(srcdir, rr.Root)
	// Run "git clone {repo} {dir}" (or the equivalent command for hg, svn, bzr)
	return dir, rr.VCS.Create(dir, rr.Repo)
}

func get(gopath, repodir, repo string) error {
	done := make(chan struct{})
	defer close(done)
	go progressSize("go get", repodir, done)

	// Run go get without arguments directly in the module directory to
	// download all its dependencies (with -t to include the test dependencies).
	cmd := exec.Command("go", "get", "-t")
	cmd.Dir = repodir
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		"GOPATH=" + gopath,
	}, passthroughEnv()...)
	return cmd.Run()
}

func removeVendor(gopath string) (found bool, _ error) {
	err := filepath.Walk(gopath, func(path string, info os.FileInfo, err error) error {
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
			return fmt.Errorf("remove all: %w", err)
		}
		return filepath.SkipDir
	})
	return found, err
}

func estimate(importpath string) error {
	// construct a separate GOPATH in a temporary directory
	gopath, err := os.MkdirTemp("", "dh-make-golang")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() {
		if err := forceRemoveAll(gopath); err != nil {
			log.Printf("could not remove all %s: %v", gopath, err)
		}
	}()

	// clone the repo inside the src directory of the GOPATH
	// and init a Go module if it is not yet one.
	srcdir := filepath.Join(gopath, "src")
	repodir, err := clone(srcdir, importpath)
	if err != nil {
		return fmt.Errorf("vcs clone: %w", err)
	}
	if !isFile(filepath.Join(repodir, "go.mod")) {
		cmd := exec.Command("go", "mod", "init", importpath)
		cmd.Dir = repodir
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go mod init: %w", err)
		}
	}

	if err := get(gopath, repodir, importpath); err != nil {
		return fmt.Errorf("go get: %w", err)
	}

	found, err := removeVendor(repodir)
	if err != nil {
		return fmt.Errorf("remove vendor: %w", err)
	}

	if found {
		// Fetch un-vendored dependencies
		if err := get(gopath, repodir, importpath); err != nil {
			return fmt.Errorf("fetch un-vendored: go get: %w", err)
		}
	}

	// Remove standard lib packages
	cmd := exec.Command("go", "list", "std")
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		"GOPATH=" + gopath,
	}, passthroughEnv()...)

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("go list std: args: %v; error: %w", cmd.Args, err)
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
	build.Default.Dir = repodir
	forward, _, errors := importgraph.Build(&build.Default)
	errLines := make([]string, 0, len(errors))
	for importPath, err := range errors {
		// For an unknown reason, parent directories and subpackages
		// of the current module report an error about not being able
		// to import them. We can safely ignore them.
		isSubpackage := strings.HasPrefix(importPath, importpath+"/")
		isParentDir := strings.HasPrefix(importpath, importPath+"/")
		if !isSubpackage && !isParentDir && importPath != importPath {
			errLines = append(errLines, fmt.Sprintf("%s: %v", importPath, err))
		}
	}
	if len(errLines) > 0 {
		return fmt.Errorf("could not load packages: %v", strings.Join(errLines, "\n"))
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
		log.Fatalf("parse args: %s", err)
	}

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	// TODO: support the -git_revision flag

	if err := estimate(fs.Arg(0)); err != nil {
		log.Fatalf("estimate: %s", err)
	}
}
