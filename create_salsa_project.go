package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
)

func execCreateSalsaProject(args []string) {
	fs := flag.NewFlagSet("create-salsa-project", flag.ExitOnError)

	useHTTPS := fs.Bool("https", false, "Use HTTPS remote URL instead of SSH")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s create-salsa-project <project-name>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s create-salsa-project golang-github-mattn-go-sqlite3\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
        fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		log.Fatalf("parse: %s", err)
	}

	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	projectName := fs.Arg(0)

	

	// The source code of the corresponding server can be found at:
	// https://salsa.debian.org/go-team/infra/pkg-go-tools/-/tree/master/cmd/pgt-api-server
	u, _ := url.Parse("https://pgt-api-server.debian.net/v1/createrepo")
	q := u.Query()
	q.Set("repo", projectName)
	u.RawQuery = q.Encode()

	resp, err := http.Post(u.String(), "", nil)
	if err != nil {
		log.Fatalf("http post: %s", err)
	}

	defer resp.Body.Close()

	if got, want := resp.StatusCode, http.StatusOK; got != want {
		b, _ := io.ReadAll(resp.Body)
		log.Fatalf("unexpected HTTP status code: got %d, want %d (response: %s)", got, want, string(b))
	}

	// Print the remote URL that will be used (informational)
    var remoteURL string
    if *useHTTPS {
        remoteURL = fmt.Sprintf("https://salsa.debian.org/go-team/packages/%s.git", projectName)
    } else {
        remoteURL = fmt.Sprintf("git@salsa.debian.org:go-team/packages/%s.git", projectName)
    }

    fmt.Printf("Project created. Remote URL: %s\n", remoteURL)
}
