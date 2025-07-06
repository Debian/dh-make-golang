package main

import (
	"github.com/Debian/dh-make-golang/cmd"
	"github.com/google/go-github/v60/github"
	"github.com/gregjones/httpcache"
	"os"
)

var (
	gitHub *github.Client
)

func init() {
	// Initialize GitHub client
	transport := github.BasicAuthTransport{
		Username:  os.Getenv("GITHUB_USERNAME"),
		Password:  os.Getenv("GITHUB_PASSWORD"),
		OTP:       os.Getenv("GITHUB_OTP"),
		Transport: httpcache.NewMemoryCacheTransport(),
	}
	gitHub = github.NewClient(transport.Client())

	// Set the exec functions for the cmd package
	cmd.SetExecFunctions(
		execSearch,
		execCreateSalsaProject,
		execEstimate,
		execMake,
		execClone,
		execCheckDepends,
	)
}

func main() {
	// For backward compatibility, add aliases for flags with underscores
	os.Args = updateFlagNames(os.Args)
	cmd.Execute()
}

// updateFlagNames replaces underscore flags with hyphen flags for backward compatibility
func updateFlagNames(args []string) []string {
	flagsToUpdate := map[string]string{
		"git_revision":         "git-revision",
		"allow_unknown_hoster": "allow-unknown-hoster",
		"force_prerelease":     "force-prerelease",
		"program_package_name": "program-package-name",
		"upstream_git_history": "upstream-git-history",
	}

	for i, arg := range args {
		for oldFlag, newFlag := range flagsToUpdate {
			if arg == "--"+oldFlag || arg == "-"+oldFlag {
				args[i] = "--" + newFlag
				break
			}
		}
	}

	return args
}
