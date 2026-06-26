package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var shortVersion bool

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Print the version number of ShiftLaunch",
	GroupID: "utils",
	Run: func(cmd *cobra.Command, args []string) {
		if shortVersion {
			fmt.Println(version)
		} else {
			fmt.Printf("ShiftLaunch v%s\n", version)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.Flags().BoolVarP(&shortVersion, "short", "s", false, "Print just the version number")
}
