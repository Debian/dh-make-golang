package cmd

import (
	"github.com/spf13/cobra"
)

var (
	// Estimate command flags
	estimateGitRevision string
)

// estimateCmd represents the estimate command
var estimateCmd = &cobra.Command{
	Use:   "estimate <go-module-importpath>",
	Short: "Estimate the amount of work for a package",
	Long: `Estimates the work necessary to bring <go-module-importpath> into Debian
by printing all currently unpacked repositories.`,
	Example: "dh-make-golang estimate github.com/Debian/dh-make-golang",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		execEstimate(args)
	},
}

func init() {
	// Add flags to the estimate command
	estimateCmd.Flags().StringVar(&estimateGitRevision, "git-revision", "",
		"git revision (see gitrevisions(7)) of the specified Go package\n"+
			"to estimate, defaulting to the default behavior of go get.\n"+
			"Useful in case you do not want to estimate the latest version.")
}
