package cmd

import (
	"github.com/spf13/cobra"
)

// checkDependsCmd represents the check-depends command
var checkDependsCmd = &cobra.Command{
	Use:   "check-depends",
	Short: "Compare go.mod and d/control to check for changes",
	Long:  `Compare go.mod and d/control to check for changes in dependencies.`,
	Run: func(cmd *cobra.Command, args []string) {
		execCheckDepends(args)
	},
}
