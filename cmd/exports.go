package cmd

// This file contains exported functions from the main package
// that are used by the cobra commands.

// These functions should be imported from the main package
// and properly implemented here.

// Placeholder declarations for the exec* functions
// These should be replaced with the actual implementations
// from the main package.

var (
	execSearch             func(args []string)
	execCreateSalsaProject func(args []string)
	execEstimate           func(args []string)
	execMake               func(args []string, usage func())
	execClone              func(args []string)
	execCheckDepends       func(args []string)
)

// SetExecFunctions sets the exec functions from the main package
func SetExecFunctions(
	search func(args []string),
	createSalsaProject func(args []string),
	estimate func(args []string),
	make func(args []string, usage func()),
	clone func(args []string),
	checkDepends func(args []string),
) {
	execSearch = search
	execCreateSalsaProject = createSalsaProject
	execEstimate = estimate
	execMake = make
	execClone = clone
	execCheckDepends = checkDepends
}
