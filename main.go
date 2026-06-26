package main

import (
	"fmt"
	"os"

	"github.com/IBM/shiftlaunch/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		// Because we silenced default Cobra errors to prevent duplicates during deployment,
		// we must manually catch and print any initialization/config errors that bubble up here!
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
}

// Made with Bob
