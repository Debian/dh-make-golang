package main

func execCheckDepends(args []string) {
	// Load the dependencies defined in the Go module (go.mod)

	// Load the dependencies defined in the Debian packaging (d/control)

	// Check for newly introduced dependencies (defined in go.mod but not in d/control)

	// Check for now unused dependencies (defined in d/control but not in go.mod)
}
