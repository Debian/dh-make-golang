package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
)

func execClone(args []string) {
	fs := flag.NewFlagSet("clone", flag.ExitOnError)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s clone <package-name>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Clone a Go package from Salsa\n"+
			"and download the appropriate tarball.\n")
		fmt.Fprintf(os.Stderr, "Example: %s clone golang-github-mmcdole-goxpp\n", os.Args[0])
	}

	err := fs.Parse(args)
	if err != nil {
		log.Fatalf("parse args: %s", err)
	}

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	cmd := exec.Command("gbp", "clone", fmt.Sprintf("vcsgit:%s", fs.Arg(0)), "--postclone=origtargz")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("Could not run %v: %v", cmd.Args, err)
	}

	fmt.Printf("Successfully cloned %s\n", fs.Arg(0))
}
