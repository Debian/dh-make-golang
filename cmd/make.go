package cmd

import (
	"github.com/spf13/cobra"
)

var (
	// Make command flags
	gitRevision            string
	allowUnknownHoster     bool
	dep14                  bool
	pristineTar            bool
	forcePrerelease        bool
	pkgTypeString          string
	customProgPkgName      string
	includeUpstreamHistory bool
	wrapAndSortFlag        string
)

// makeCmd represents the make command
var makeCmd = &cobra.Command{
	Use:   "make [flags] <go-package-importpath>",
	Short: "Create a Debian package",
	Long: `"dh-make-golang make" downloads the specified Go package from the Internet,
and creates new files and directories in the current working directory.`,
	Example: "dh-make-golang make golang.org/x/oauth2",
	Run: func(cmd *cobra.Command, args []string) {
		execMake(args, nil)
	},
}

func init() {
	// Add flags to the make command
	makeCmd.Flags().StringVar(&gitRevision, "git-revision", "",
		"git revision (see gitrevisions(7)) of the specified Go package\n"+
			"to check out, defaulting to the default behavior of git clone.\n"+
			"Useful in case you do not want to package e.g. current HEAD.")

	makeCmd.Flags().BoolVar(&allowUnknownHoster, "allow-unknown-hoster", false,
		"The pkg-go naming conventions use a canonical identifier for\n"+
			"the hostname (see https://go-team.pages.debian.net/packaging.html),\n"+
			"and the mapping is hardcoded into dh-make-golang.\n"+
			"In case you want to package a Go package living on an unknown hoster,\n"+
			"you may set this flag to true and double-check that the resulting\n"+
			"package name is sane. Contact pkg-go if unsure.")

	makeCmd.Flags().BoolVar(&dep14, "dep14", true,
		"Follow DEP-14 branch naming and use debian/sid (instead of master)\n"+
			"as the default debian-branch.")

	makeCmd.Flags().BoolVar(&pristineTar, "pristine-tar", false,
		"Keep using a pristine-tar branch as in the old workflow.\n"+
			"Discouraged, see \"pristine-tar considered harmful\"\n"+
			"https://michael.stapelberg.ch/posts/2018-01-28-pristine-tar/\n"+
			"and the \"Drop pristine-tar branches\" section at\n"+
			"https://go-team.pages.debian.net/workflow-changes.html")

	makeCmd.Flags().BoolVar(&forcePrerelease, "force-prerelease", false,
		"Package @master or @tip instead of the latest tagged version")

	makeCmd.Flags().StringVar(&pkgTypeString, "type", "",
		"Set package type, one of:\n"+
			" * \"library\" (aliases: \"lib\", \"l\", \"dev\")\n"+
			" * \"program\" (aliases: \"prog\", \"p\")\n"+
			" * \"library+program\" (aliases: \"lib+prog\", \"l+p\", \"both\")\n"+
			" * \"program+library\" (aliases: \"prog+lib\", \"p+l\", \"combined\")")

	makeCmd.Flags().StringVar(&customProgPkgName, "program-package-name", "",
		"Override the program package name, and the source package name too\n"+
			"when appropriate, e.g. to name github.com/cli/cli as \"gh\"")

	makeCmd.Flags().BoolVar(&includeUpstreamHistory, "upstream-git-history", true,
		"Include upstream git history (Debian pkg-go team new workflow).\n"+
			"New in dh-make-golang 0.3.0, currently experimental.")

	makeCmd.Flags().StringVar(&wrapAndSortFlag, "wrap-and-sort", "at",
		"Set how the various multi-line fields in debian/control are formatted.\n"+
			"Valid values are \"a\", \"at\" and \"ast\", see wrap-and-sort(1) man page\n"+
			"for more information.")
}
