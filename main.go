package main

import (
	"os"
)

func main() {
	// Retrieve args and Shift binary name off argument list.
	args := os.Args[1:]

	// Retrieve command name as first argument.
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}

	switch cmd {
	case "search":
		execSearch(args[1:])
	case "create-salsa-project":
		execCreateSalsaProject(args[1:])
	default:
		execMake(args)
	}

}
