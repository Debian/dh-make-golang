package main

import (
	"flag"
	"os"
)

func main() {
	// Retrieve args and Shift binary name off argument list.
	args := os.Args[1:]

	fs := flag.NewFlagSet("main", flag.ExitOnError)
	fs.Parse(args)

	// Retrieve command name as first argument.
	cmd := fs.Arg(0)

	switch cmd {
	case "search":
		execSearch(args[1:])
	default:
		execMake(args)
	}

}
