package main

import (
	"log"
)

type dependency struct {
	importPath  string
	packageName string
	// todo version?
}

func execCheckDepends(args []string) {
	// Load the dependencies defined in the Go module (go.mod)
	goModDepds, err := parseGoModDependencies()
	if err != nil {
		log.Fatalf("error while parsing go.mod: %s", err)
	}

	// Load the dependencies defined in the Debian packaging (d/control)
	packageDeps, err := parsePackageDependencies()
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
func parseGoModDependencies() ([]dependency, error) {
	return nil, nil
}

// parsePackageDependencies parse the Build-Depends defined in d/control
func parsePackageDependencies() ([]dependency, error) {
	return nil, nil
}
