package cmd

import (
	"github.com/spf13/cobra"
)

// createSalsaProjectCmd represents the create-salsa-project command
var createSalsaProjectCmd = &cobra.Command{
	Use:     "create-salsa-project <project-name>",
	Short:   "Create a project for hosting Debian packaging",
	Long:    `Create a project for hosting Debian packaging on Salsa.`,
	Example: "dh-make-golang create-salsa-project golang-github-mattn-go-sqlite3",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		execCreateSalsaProject(args)
	},
}
