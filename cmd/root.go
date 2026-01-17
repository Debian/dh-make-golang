package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const program = "dh-make-golang"

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   program,
	Short: "A tool that converts Go packages into Debian package source",
	Long: `dh-make-golang is a tool that converts Go packages into Debian package source.
For backwards compatibility, when no command is specified, the make command is executed.`,
	// When no arguments are provided, show help instead of running make command
	Run:     nil,
	Version: buildVersionString(),
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// Add commands
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(createSalsaProjectCmd)
	rootCmd.AddCommand(estimateCmd)
	rootCmd.AddCommand(makeCmd)
	rootCmd.AddCommand(cloneCmd)
	rootCmd.AddCommand(checkDependsCmd)
	rootCmd.AddCommand(completionCmd)
}
