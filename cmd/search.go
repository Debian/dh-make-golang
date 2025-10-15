package cmd

import (
	"github.com/spf13/cobra"
)

// searchCmd represents the search command
var searchCmd = &cobra.Command{
	Use:     "search [flags] <pattern>",
	Short:   "Search Debian for already-existing packages",
	Long:    `Search Debian for already-existing packages using Go's default regexp syntax.`,
	Example: "dh-make-golang search 'debi.*'",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		execSearch(args)
	},
}
