package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
)

func execCreateSalsaProject(args []string) {
	fs := flag.NewFlagSet("create-salsa-project", flag.ExitOnError)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s create-salsa-project <project-name>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s create-salsa-project golang-github-mattn-go-sqlite3\n", os.Args[0])
	}

	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	projectName := fs.Arg(0)

	// The source code of the corresponding server can be found at:
	// https://github.com/Debian/pkg-go-tools/tree/master/cmd/pgt-api-server
	u, _ := url.Parse("https://pgt-api-server.debian.net/v1/createrepo")
	q := u.Query()
	q.Set("repo", projectName)
	u.RawQuery = q.Encode()

	resp, err := http.Post(u.String(), "", nil)
	if err != nil {
		log.Fatal(err)
	}
	if got, want := resp.StatusCode, http.StatusOK; got != want {
		b, _ := ioutil.ReadAll(resp.Body)
		log.Fatalf("unexpected HTTP status code: got %d, want %d (response: %s)", got, want, string(b))
	}
}
