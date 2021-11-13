package main

import (
	"fmt"
	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/vcs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"pault.ag/go/debian/control"
	"strings"
)

type dependency struct {
	importPath  string
	packageName string
	// todo version?
}

func execCheckDepends(args []string) {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("error while getting current directory: %s", err)
	}

	// Load the already packaged Go modules
	golangBinaries, err := getGolangBinaries()
	if err != nil {
		log.Fatalf("error while getting packaged Go modules: %s", err)
	}

	// Load the dependencies defined in the Go module (go.mod)
	goModDepds, err := parseGoModDependencies(cwd, golangBinaries)
	if err != nil {
		log.Fatalf("error while parsing go.mod: %s", err)
	}

	// Load the dependencies defined in the Debian packaging (d/control)
	packageDeps, err := parseDebianControlDependencies(cwd)
	if err != nil {
		log.Fatalf("error while parsing d/control: %s", err)
	}

	hasChanged := false

	// Check for newly introduced dependencies (defined in go.mod but not in d/control)
	for _, goModDep := range goModDepds {
		found := false

		if goModDep.packageName == "" {
			fmt.Printf("NEW dependency %s is NOT yet packaged in Debian\n", goModDep.importPath)
			continue
		}

		for _, packageDep := range packageDeps {
			if packageDep.packageName == goModDep.packageName {
				found = true
				break
			}
		}

		if !found {
			hasChanged = true
			fmt.Printf("NEW dependency %s (%s)\n", goModDep.importPath, goModDep.packageName)
		}
	}

	// Check for now unused dependencies (defined in d/control but not in go.mod)
	for _, packageDep := range packageDeps {
		found := false

		for _, goModDep := range goModDepds {
			if goModDep.packageName == packageDep.packageName {
				found = true
				break
			}
		}

		if !found {
			hasChanged = true
			fmt.Printf("RM dependency %s (%s)\n", packageDep.importPath, packageDep.packageName)
		}
	}

	if !hasChanged {
		fmt.Printf("go.mod and d/control are in sync\n")
	}
}

// parseGoModDependencies parse ALL dependencies listed in go.mod
// i.e. it returns the one defined in go.mod as well as the transitively ones
// TODO: this may not be the best way of doing thing since it requires the package to be converted to go module
func parseGoModDependencies(directory string, goBinaries map[string]string) ([]dependency, error) {
	b, err := ioutil.ReadFile(filepath.Join(directory, "go.mod"))
	if err != nil {
		return nil, err
	}

	modFile, err := modfile.Parse("go.mod", b, nil)
	if err != nil {
		return nil, err
	}

	var dependencies []dependency
	for _, require := range modFile.Require {
		if !require.Indirect {
			packageName := ""

			// Translate all packages to the root of their repository
			rr, err := vcs.RepoRootForImportPath(require.Mod.Path, false)
			if err != nil {
				log.Printf("Could not determine repo path for import path %q: %v\n", require.Mod.Path, err)
				continue
			}

			if val, exists := goBinaries[rr.Root]; exists {
				packageName = val
			}

			dependencies = append(dependencies, dependency{
				importPath:  rr.Root,
				packageName: packageName,
			})
		}
	}

	return dependencies, nil
}

// parseDebianControlDependencies parse the Build-Depends defined in d/control
func parseDebianControlDependencies(directory string) ([]dependency, error) {
	ctrl, err := control.ParseControlFile(filepath.Join(directory, "debian", "control"))
	if err != nil {
		return nil, err
	}

	var dependencies []dependency

	for _, bp := range ctrl.Source.BuildDepends.GetAllPossibilities() {
		packageName := strings.Trim(bp.Name, "\n")

		// Ignore non -dev dependencies (i.e, debhelper-compat, git, cmake, etc...)
		if !strings.HasSuffix(packageName, "-dev") {
			continue
		}

		dependencies = append(dependencies, dependency{
			importPath:  "", // TODO XS-Go-Import-Path?
			packageName: packageName,
		})
	}

	return dependencies, nil
}
