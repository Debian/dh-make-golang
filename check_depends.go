package main

import (
	"golang.org/x/mod/modfile"
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

	// Load the dependencies defined in the Go module (go.mod)
	goModDepds, err := parseGoModDependencies(cwd)
	if err != nil {
		log.Fatalf("error while parsing go.mod: %s", err)
	}

	// Load the dependencies defined in the Debian packaging (d/control)
	packageDeps, err := parsePackageDependencies(cwd)
	if err != nil {
		log.Fatalf("error while parsing d/control: %s", err)
	}

	hasChanged := false

	// Check for newly introduced dependencies (defined in go.mod but not in d/control)
	for _, goModDep := range goModDepds {
		found := false

		for _, packageDep := range packageDeps {
			if packageDep.packageName == goModDep.packageName {
				found = true
				break
			}
		}

		if !found {
			hasChanged = true
			log.Printf("NEW dependency %s (%s)", goModDep.importPath, goModDep.packageName)
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
			log.Printf("RM dependency %s (%s)", packageDep.importPath, packageDep.packageName)
		}
	}

	if !hasChanged {
		log.Printf("go.mod and d/control are in sync")
	}
}

// parseGoModDependencies parse ALL dependencies listed in go.mod
// i.e. it returns the one defined in go.mod as well as the transitively ones
// TODO: this may not be the best way of doing thing since it requires the package to be converted to go module
func parseGoModDependencies(directory string) ([]dependency, error) {
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
			dependencies = append(dependencies, dependency{
				importPath:  require.Mod.Path,
				packageName: debianNameFromGopkg(require.Mod.Path, typeLibrary, "", true) + "-dev",
			})
		}
	}

	return dependencies, nil
}

// parsePackageDependencies parse the Build-Depends defined in d/control
func parsePackageDependencies(directory string) ([]dependency, error) {
	ctrl, err := control.ParseControlFile(filepath.Join(directory, "debian", "control"))
	if err != nil {
		return nil, err
	}

	var dependencies []dependency
	baseBuildDepends := getBaseBuildDepends()

	for _, bp := range ctrl.Source.BuildDepends.GetAllPossibilities() {
		packageName := strings.Trim(bp.Name, "\n")

		isBase := false
		for _, baseBuildDepend := range baseBuildDepends {
			if baseBuildDepend == packageName {
				isBase = true
				break
			}
		}

		// Skip base build depends
		if isBase {
			continue
		}

		dependencies = append(dependencies, dependency{
			importPath:  "", // TODO XS-Go-Import-Path?
			packageName: packageName,
		})
	}

	return dependencies, nil
}

// getBaseBuildDepends returns the list of dependencies that are non Go package
// i.e. debhelper-compat, dh-golang, golang-any and friends.
// TODO: is there a better way?
func getBaseBuildDepends() []string {
	return []string{"debhelper-compat", "dh-golang", "golang-any"}
}
