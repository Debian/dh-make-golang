package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/vcs"
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

	// Use digraph(1) to obtain the forward transitive closure of the repo in
	// question.
	cmd := exec.Command("/bin/sh", "-c", "go list -f '{{.ImportPath}}{{.Imports}}{{.TestImports}}{{.XTestImports}}' ... | tr '[]' ' ' | digraph forward $(go list "+importpath+"/...)")
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		fmt.Sprintf("GOPATH=%s", gopath),
	}, passthroughEnv()...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("%v: %v", cmd.Args, err)
	}

	closure := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		closure[line] = true
	}

	// Remove standard lib packages
	cmd = exec.Command("go", "list", "std")
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		fmt.Sprintf("GOPATH=%s", gopath),
	}, passthroughEnv()...)

	out, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("%v: %v", cmd.Args, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		delete(closure, line)
	}

	delete(closure, "C") // would fail resolving anyway

	// Resolve all packages to the root of their repository.
	roots := make(map[string]bool)
	for dep := range closure {
		rr, err := vcs.RepoRootForImportPath(dep, false)
		if err != nil {
			log.Printf("Could not determine repo path for import path %q: %v\n", dep, err)
			continue
		}

		roots[rr.Root] = true
	}

	// Filter out all already-packaged ones:
	golangBinaries, err := getGolangBinaries()
	if err != nil {
		return nil
	}

	for importpath, binary := range golangBinaries {
		if roots[importpath] {
			log.Printf("found %s in Debian package %s", importpath, binary)
			delete(roots, importpath)
		}
	}

	if len(roots) == 0 {
		log.Printf("%s is already fully packaged in Debian", importpath)
		return nil
	}
	log.Printf("Bringing %s to Debian requires packaging the following Go packages:", importpath)
	for importpath := range roots {
		fmt.Println(importpath)
	}

	return nil
}

func execEstimate(args []string) {
	fs := flag.NewFlagSet("estimate", flag.ExitOnError)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s estimate <go-package-importpath>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Estimates the work necessary to bring <go-package-importpath> into Debian by printing all currently unpacked repositories\n")
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
