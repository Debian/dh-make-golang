package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/vcs"
)

// majorVersionRegexp checks if an import path contains a major version suffix.
var majorVersionRegexp = regexp.MustCompile(`([/.])v([0-9]+)$`)

func clone(srcdir, repo string) error {
	done := make(chan struct{})
	defer close(done)
	go progressSize("vcs clone", srcdir, done)

	// Get the sources of the module in a temporary dir to be able to run
	// go get in module mode, as the gopath mode has been removed in latest
	// version of Go.
	rr, err := vcs.RepoRootForImportPath(repo, false)
	if err != nil {
		return fmt.Errorf("get repo root: %w", err)
	}
	// Run "git clone {repo} {dir}" (or the equivalent command for hg, svn, bzr)
	return rr.VCS.Create(srcdir, rr.Repo)
}

func get(gopath, repodir, repo string) error {
	done := make(chan struct{})
	defer close(done)
	go progressSize("go get", repodir, done)

	// Run go mod tidy directly in the module directory to sync go.(mod|sum) and
	// download all its dependencies.
	cmd := exec.Command("go", "mod", "tidy")
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

// otherVersions guesses the import paths of potential other major version
// of the given module import path, based on [majorVersionRegex].
func otherVersions(mod string) (mods []string) {
	matches := majorVersionRegexp.FindStringSubmatch(mod)
	if matches == nil {
		return
	}
	matchFull, matchSep, matchVer := matches[0], matches[1], matches[2]
	matchIndex := len(mod) - len(matchFull)
	prefix := mod[:matchIndex]
	version, _ := strconv.Atoi(matchVer)
	for v := version - 1; v > 1; v-- {
		mods = append(mods, prefix+matchSep+"v"+strconv.Itoa(v))
	}
	mods = append(mods, prefix)
	return
}

func estimate(importpath string) error {
	removeTemp := func(path string) {
		if err := forceRemoveAll(path); err != nil {
			log.Printf("could not remove all %s: %v", path, err)
		}
	}

	// construct a separate GOPATH in a temporary directory
	gopath, err := os.MkdirTemp("", "dh-make-golang")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer removeTemp(gopath)
	// second temporary directosy for the repo sources
	repodir, err := os.MkdirTemp("", "dh-make-golang")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer removeTemp(repodir)

	// clone the repo inside the src directory of the GOPATH
	// and init a Go module if it is not yet one.
	if err := clone(repodir, importpath); err != nil {
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

	// Get dependency graph from go mod graph
	cmd := exec.Command("go", "mod", "graph")
	cmd.Dir = repodir
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		"GOPATH=" + gopath,
	}, passthroughEnv()...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("go mod graph: args: %v; error: %w", cmd.Args, err)
	}

	// Retrieve already-packaged ones
	golangBinaries, err := getGolangBinaries()
	if err != nil {
		return nil
	}

	// Build a graph in memory from the output of go mod graph
	type Node struct {
		name     string
		children []*Node
	}
	root := &Node{name: importpath}
	nodes := make(map[string]*Node)
	nodes[importpath] = root
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		// go mod graph outputs one line for each dependency. Each line
		// consists of the dependency preceded by the module that
		// imported it, separated by a single space. The module names
		// can have a version information delimited by the @ character
		src, dep, _ := strings.Cut(line, " ")
		depNode := &Node{name: dep}
		// Sometimes, the given import path is not the one outputed by
		// go mod graph, for instance when there are multiple major
		// versions.
		// The root module is the only one that does not have a version
		// indication with @ in the output of go mod graph, so if there
		// is no @ we always use the given importpath instead.
		if !strings.Contains(src, "@") {
			src = importpath
		}
		srcNode, ok := nodes[src]
		if !ok {
			log.Printf("source not found in graph: %s", src)
			continue
		}
		srcNode.children = append(srcNode.children, depNode)
		nodes[dep] = depNode
	}

	// Analyse the dependency graph
	var lines []string
	seen := make(map[string]bool)
	var visit func(n *Node, indent int)
	visit = func(n *Node, indent int) {
		// Get the module name without its version, as go mod graph
		// can return multiple times the same module with different
		// versions.
		mod, _, _ := strings.Cut(n.name, "@")
		if seen[mod] {
			return
		}
		seen[mod] = true
		// Go version dependency is indicated as a dependency to "go" and
		// "toolchain", we do not use this information for now.
		if mod == "go" || mod == "toolchain" {
			return
		}
		if _, ok := golangBinaries[mod]; ok {
			return // already packaged in Debian
		}
		var debianVersion string
		// Check for potential other major versions already in Debian.
		for _, otherVersion := range otherVersions(mod) {
			if _, ok := golangBinaries[otherVersion]; ok {
				debianVersion = otherVersion
				break
			}
		}
		if debianVersion == "" {
			// When multiple modules are developped in the same repo,
			// the repo root is often used as the import path metadata
			// in Debian, so we do a last try with that.
			rr, err := vcs.RepoRootForImportPath(mod, false)
			if err != nil {
				log.Printf("Could not determine repo path for import path %q: %v\n", mod, err)
			} else if _, ok := golangBinaries[rr.Root]; ok {
				// Log info to indicate that it is an approximate match
				// but consider that it is packaged and skip the children.
				log.Printf("%s is packaged as %s in Debian", mod, rr.Root)
				return
			}
		}
		if debianVersion != "" {
			lines = append(lines, fmt.Sprintf("%s%s\t(%s in Debian)", strings.Repeat("  ", indent), mod, debianVersion))
		} else {
			lines = append(lines, fmt.Sprintf("%s%s", strings.Repeat("  ", indent), mod))
		}
		for _, n := range n.children {
			visit(n, indent+1)
		}
	}

	visit(root, 0)

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
