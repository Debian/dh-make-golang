package main

import (
	"fmt"
	"os"

	"github.com/google/go-github/v60/github"
	"github.com/gregjones/httpcache"
)

const program = "dh-make-golang"

var (
	gitHub *github.Client
)

func usage() {
	fmt.Fprintf(os.Stderr, `%s

%s is a tool that converts Go packages into Debian package source.

Usage:
	%s [globalflags] <command> [flags] <args>

%s commands:
	make			create a Debian package
	search			search Debian for already-existing packages
	estimate		estimate the amount of work for a package
	create-salsa-project	create a project for hosting Debian packaging
	clone			clone a Go package from Salsa
	check-depends		compare go.mod and d/control to check for changes

For backwards compatibility, when no command is specified,
the make command is executed.

To learn more about a command, run "%s <command> -help",
e.g. "%s make -help"

`, buildVersionString(), program, program, program, program, program)
}

func main() {
	transport := github.BasicAuthTransport{
		Username:  os.Getenv("GITHUB_USERNAME"),
		Password:  os.Getenv("GITHUB_PASSWORD"),
		OTP:       os.Getenv("GITHUB_OTP"),
		Transport: httpcache.NewMemoryCacheTransport(),
	}
	gitHub = github.NewClient(transport.Client())

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
	case "clone":
		execClone(args[1:])
	case "check-depends":
		execCheckDepends(args[1:])
	default:
		// redirect -help to the global usage
		execMake(args, usage)
	}
}
