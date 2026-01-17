package cmd

import (
	"github.com/spf13/cobra"
)

// cloneCmd represents the clone command
var cloneCmd = &cobra.Command{
	Use:     "clone <package-name>",
	Short:   "Clone a Go package from Salsa",
	Long:    `Clone a Go package from Salsa and download the appropriate tarball.`,
	Example: "dh-make-golang clone golang-github-mmcdole-goxpp",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		execClone(args)
	},
}
