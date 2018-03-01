package main

import (
	"fmt"
	"os"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [globalflags] <command> [flags] <args>\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "%s commands:\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\tmake\t\t\tcreate a Debian package\n")
	fmt.Fprintf(os.Stderr, "\tsearch\t\t\tsearch Debian for already-existing packages\n")
	fmt.Fprintf(os.Stderr, "\testimate\t\testimate the amount of work for a package\n")
	fmt.Fprintf(os.Stderr, "\tcreate-salsa-project\tcreate a project for hosting Debian packaging\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "For backwards compatibility, when no command is specified, the make command is executed.\n")
	fmt.Fprintf(os.Stderr, "To learn more about a command, run %s <command> -help, e.g. %s make -help\n", os.Args[0], os.Args[0])
}

func main() {
	// Retrieve args and Shift binary name off argument list.
	args := os.Args[1:]

	// Retrieve command name as first argument.
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}

	switch cmd {
	case "help":
		usage()
	case "search":
		execSearch(args[1:])
	case "create-salsa-project":
		execCreateSalsaProject(args[1:])
	case "estimate":
		execEstimate(args[1:])
	case "make":
		execMake(args[1:], nil)
	default:
		// redirect -help to the global usage
		execMake(args, usage)
	}
}
