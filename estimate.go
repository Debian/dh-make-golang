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

func get(gopath, repodir, repo, rev string) error {
	done := make(chan struct{})
	defer close(done)
	go progressSize("go get", repodir, done)

	// As per https://groups.google.com/forum/#!topic/golang-nuts/N5apfenE4m4,
	// the arguments to “go get” are packages, not repositories. Hence, we
	// specify “gopkg/...” in order to cover all packages.
	// As a concrete example, github.com/jacobsa/util is a repository we want
	// to package into a single Debian package, and using “go get -t
	// github.com/jacobsa/util” fails because there are no buildable go files
	// in the top level of that repository.
	packages := repo + "/..."
	if rev != "" {
		packages += "@" + rev
	}
	cmd := exec.Command("go", "get", "-t", packages)
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

func estimate(importpath, revision string) error {
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

	// Create a dummy go module in repodir to be able to use go get.
	err = os.WriteFile(filepath.Join(repodir, "go.mod"), []byte("module dummymod\n"), 0644)
	if err != nil {
		return fmt.Errorf("create dummymod: %w", err)
	}

	if err := get(gopath, repodir, importpath, revision); err != nil {
		return fmt.Errorf("go get: %w", err)
	}

	found, err := removeVendor(repodir)
	if err != nil {
		return fmt.Errorf("remove vendor: %w", err)
	}

	if found {
		// Fetch un-vendored dependencies
		if err := get(gopath, repodir, importpath, revision); err != nil {
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
		// The root module is the only one that does not have a version
		// indication with @ in the output of go mod graph. We use this
		// to filter out the depencencies of the "dummymod" module.
		if mod, _, found := strings.Cut(src, "@"); !found {
			continue
		} else if mod == importpath || strings.HasPrefix(mod, importpath+"/") {
			src = importpath
		}
		depNode, ok := nodes[dep]
		if !ok {
			depNode = &Node{name: dep}
			nodes[dep] = depNode
		}
		srcNode, ok := nodes[src]
		if !ok {
			srcNode = &Node{name: src}
			nodes[src] = srcNode
		}
		srcNode.children = append(srcNode.children, depNode)
	}

	// Analyse the dependency graph
	var lines []string
	seen := make(map[string]bool)
	rrseen := make(map[string]bool)
	needed := make(map[string]int)
	var visit func(n *Node, indent int)
	visit = func(n *Node, indent int) {
		// Get the module name without its version, as go mod graph
		// can return multiple times the same module with different
		// versions.
		mod, _, _ := strings.Cut(n.name, "@")
		count, isNeeded := needed[mod]
		if isNeeded {
			count++
			needed[mod] = count
			lines = append(lines, fmt.Sprintf("%s\033[90m%s (%d)\033[0m", strings.Repeat("  ", indent), mod, count))
		} else if seen[mod] {
			return
		} else {
			seen[mod] = true
			// Go version dependency is indicated as a dependency to "go" and
			// "toolchain", we do not use this information for now.
			if mod == "go" || mod == "toolchain" {
				return
			}
			if _, ok := golangBinaries[mod]; ok {
				return // already packaged in Debian
			}
			var repoRoot string
			rr, err := vcs.RepoRootForImportPath(mod, false)
			if err != nil {
				log.Printf("Could not determine repo path for import path %q: %v\n", mod, err)
				repoRoot = mod
			} else {
				repoRoot = rr.Root
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
				if _, ok := golangBinaries[repoRoot]; ok {
					// Log info to indicate that it is an approximate match
					// but consider that it is packaged and skip the children.
					log.Printf("%s is packaged as %s in Debian", mod, repoRoot)
					return
				}
			}
			line := strings.Repeat("  ", indent)
			if rrseen[repoRoot] {
				line += fmt.Sprintf("\033[90m%s\033[0m", mod)
			} else if strings.HasPrefix(mod, repoRoot) && len(mod) > len(repoRoot) {
				suffix := mod[len(repoRoot):]
				line += fmt.Sprintf("%s\033[90m%s\033[0m", repoRoot, suffix)
			} else {
				line += mod
			}
			if debianVersion != "" {
				line += fmt.Sprintf("\t(%s in Debian)", debianVersion)
			}
			lines = append(lines, line)
			rrseen[repoRoot] = true
			needed[mod] = 1
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
	log.Printf("Bringing %s to Debian requires packaging the following Go modules:", importpath)
	for _, line := range lines {
		fmt.Println(line)
	}

	return nil
}

func execEstimate(args []string) {
	fs := flag.NewFlagSet("estimate", flag.ExitOnError)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s estimate <go-module-importpath>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Estimates the work necessary to bring <go-module-importpath> into Debian\n"+
			"by printing all currently unpacked repositories.\n")
		fmt.Fprintf(os.Stderr, "Example: %s estimate github.com/Debian/dh-make-golang\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fs.PrintDefaults()
	}

	var gitRevision string
	fs.StringVar(&gitRevision,
		"git_revision",
		"",
		"git revision (see gitrevisions(7)) of the specified Go package\n"+
			"to estimate, defaulting to the default behavior of go get.\n"+
			"Useful in case you do not want to estimate the latest version.")

	err := fs.Parse(args)
	if err != nil {
		log.Fatalf("parse args: %s", err)
	}

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	gitRevision = strings.TrimSpace(gitRevision)

	if err := estimate(fs.Arg(0), gitRevision); err != nil {
		log.Fatalf("estimate: %s", err)
	}
}
