package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/vcs"
)

const (
	sourcesInNewURL = "https://api.ftp-master.debian.org/sources_in_suite/new"
)

// majorVersionRegexp checks if an import path contains a major version suffix.
var majorVersionRegexp = regexp.MustCompile(`([/.])v([0-9]+)$`)

// moduleBlocklist is a map of modules that we want to exclude from the estimate
// output, associated with the reason why.
var moduleBlocklist = map[string]string{
	"github.com/arduino/go-win32-utils": "Windows only",
	"github.com/Microsoft/go-winio":     "Windows only",
}

func getSourcesInNew() (map[string]string, error) {
	sourcesInNew := make(map[string]string)

	resp, err := http.Get(sourcesInNewURL)
	if err != nil {
		return nil, fmt.Errorf("getting %q: %w", golangBinariesURL, err)
	}
	if got, want := resp.StatusCode, http.StatusOK; got != want {
		return nil, fmt.Errorf("unexpected HTTP status code: got %d, want %d", got, want)
	}
	var pkgs []struct {
		Source  string `json:"source"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pkgs); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	for _, pkg := range pkgs {
		sourcesInNew[pkg.Source] = pkg.Version
	}
	return sourcesInNew, nil
}

func get(gopath, repodir, repo, rev string) error {
	done := make(chan struct{})
	defer close(done)
	go progressSize("go get", gopath, done)

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

	out := bytes.Buffer{}
	cmd.Dir = repodir
	cmd.Stderr = &out
	cmd.Env = append([]string{
		"GOPATH=" + gopath,
	}, passthroughEnv()...)
	err := cmd.Run()
	if err != nil {
		fmt.Fprint(os.Stderr, "\n", out.String())
	}
	return err
}

// getModuleDir returns the path of the directory containing a module for the
// given GOPATH and repository dir values.
func getModuleDir(gopath, repodir, module string) (string, error) {
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", module)
	cmd.Dir = repodir
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		"GOPATH=" + gopath,
	}, passthroughEnv()...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go list: args: %v; error: %w", cmd.Args, err)
	}
	return string(bytes.TrimSpace(out)), nil
}

// getDirectDependencies returns a set of all the direct dependencies of a
// module for the given GOPATH and repository dir values. It first finds the
// directory that contains this module, then uses go list in this directory
// to get its direct dependencies.
func getDirectDependencies(gopath, repodir, module string) (map[string]bool, error) {
	dir, err := getModuleDir(gopath, repodir, module)
	if err != nil {
		return nil, fmt.Errorf("get module dir: %w", err)
	}
	cmd := exec.Command("go", "list", "-m", "-f", "{{if not .Indirect}}{{.Path}}{{end}}", "all")
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{
		"GOPATH=" + gopath,
	}, passthroughEnv()...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: args: %v; error: %w", cmd.Args, err)
	}
	out = bytes.TrimRight(out, "\n")
	lines := strings.Split(string(out), "\n")
	deps := make(map[string]bool, len(lines))
	for _, line := range lines {
		deps[line] = true
	}
	return deps, nil
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

// findOtherVersion search in m for potential other versions of the given
// module and returns the number of the major version found, 0 if not,
// along with the corresponding package name.
func findOtherVersion(m map[string]debianPackage, mod string) (int, debianPackage) {
	versions := otherVersions(mod)
	for i, version := range versions {
		if pkg, ok := m[version]; ok {
			return len(versions) - i, pkg
		}
	}
	return 0, debianPackage{}
}

// trackerLink generates an OSC 8 hyperlink to the tracker for the given Debian
// package name.
func trackerLink(pkg string) string {
	return fmt.Sprintf("\033]8;;https://tracker.debian.org/pkg/%[1]s\033\\%[1]s\033]8;;\033\\", pkg)
}

// newPackageLine generates a line for packages in NEW, including an OSC 8
// hyperlink to the FTP masters website for the given Debian package.
func newPackageLine(indent int, mod, debpkg, version string) string {
	const format = "%s\033[36m%s (\033]8;;https://ftp-master.debian.org/new/%s_%s.html\033\\in NEW\033]8;;\033\\)\033[0m"
	return fmt.Sprintf(format, strings.Repeat("  ", indent), mod, debpkg, version)
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

	// Get direct dependencies, to filter out indirect ones from go mod graph output
	directDeps, err := getDirectDependencies(gopath, repodir, importpath)
	if err != nil {
		return fmt.Errorf("get direct dependencies: %w", err)
	}

	// Retrieve already-packaged ones
	golangBinaries, err := getGolangBinaries()
	if err != nil {
		return fmt.Errorf("get golang debian packages: %w", err)
	}
	sourcesInNew, err := getSourcesInNew()
	if err != nil {
		return fmt.Errorf("get packages in new: %w", err)
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
		// Get the module names without their version, as we do not use
		// this information.
		// The root module is the only one that does not have a version
		// indication with @ in the output of go mod graph. We use this
		// to filter out the depencencies of the "dummymod" module.
		dep, _, _ = strings.Cut(dep, "@")
		src, _, found := strings.Cut(src, "@")
		if !found {
			continue
		}
		// Due to importing all packages of the estimated module in a
		// dummy one, some modules can depend on submodules of the
		// estimated one. We do as if they are dependencies of the
		// root one.
		if strings.HasPrefix(src, importpath+"/") {
			src = importpath
		}
		// go mod graph also lists indirect dependencies as dependencies
		// of the current module, so we filter them out. They will still
		// appear later.
		if src == importpath && !directDeps[dep] {
			continue
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
		mod := n.name
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
			if pkg, ok := golangBinaries[mod]; ok {
				if version, ok := sourcesInNew[pkg.source]; ok {
					line := newPackageLine(indent, mod, pkg.source, version)
					lines = append(lines, line)
				}
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
			// Check for potential other major versions already in Debian.
			v, pkg := findOtherVersion(golangBinaries, mod)
			if v != 0 {
				// Log info to indicate that it is an approximate match
				// but consider that it is packaged and skip the children.
				if v == 1 {
					log.Printf("%s has no version string in Debian (%s)", mod, trackerLink(pkg.source))
				} else {
					log.Printf("%s is v%d in Debian (%s)", mod, v, trackerLink(pkg.source))
				}
				if version, ok := sourcesInNew[pkg.source]; ok {
					line := newPackageLine(indent, mod, pkg.source, version)
					lines = append(lines, line)
				}
				return
			}
			// When multiple modules are developped in the same repo,
			// the repo root is often used as the import path metadata
			// in Debian, so we do a last try with that.
			if pkg, ok := golangBinaries[repoRoot]; ok {
				// Log info to indicate that it is an approximate match
				// but consider that it is packaged and skip the children.
				log.Printf("%s is packaged as %s in Debian (%s)", mod, repoRoot, trackerLink(pkg.source))
				if version, ok := sourcesInNew[pkg.source]; ok {
					line := newPackageLine(indent, mod, pkg.source, version)
					lines = append(lines, line)
				}
				return
			}
			// Ignore modules from the blocklist.
			if reason, found := moduleBlocklist[mod]; found {
				log.Printf("Ignoring module %s: %s", mod, reason)
				return
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
