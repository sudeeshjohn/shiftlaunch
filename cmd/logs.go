package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/IBM/shiftlaunch/config"
)

var followLogs bool

var logsCmd = &cobra.Command{
	Use:     "logs [cluster-name]",
	Aliases: []string{"log"},
	Short:   "Fetch the logs of a cluster deployment",
	GroupID: "core",
	Long: `Display deployment logs for a cluster. By default, shows the entire log file.
Use -f/--follow to stream logs in real-time (like tail -f).

Examples:
  shiftlaunch logs my-cluster          # Display all logs
  shiftlaunch logs my-cluster -f       # Follow logs in real-time
  shiftlaunch logs -f --cluster prod   # Follow using --cluster flag`,
	RunE: runLogs,
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "Follow log output in real-time")
}

func runLogs(cmd *cobra.Command, args []string) error {
	daemonCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load daemon config: %w", err)
	}

	// Determine cluster name (from arg or --cluster flag)
	targetCluster := clusterName
	if len(args) > 0 {
		targetCluster = args[0]
	}
	if targetCluster == "" {
		return fmt.Errorf("cluster name required: provide as argument or use --cluster flag")
	}

	logPath := filepath.Join(daemonCfg.Paths.WorkspaceDir, targetCluster, "deployment.log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("no logs found for cluster '%s'\nExpected log file at: %s", targetCluster, logPath)
	}

	// Use system 'tail' for the -f follow behavior, or 'cat' for a static dump
	var execCmd *exec.Cmd
	if followLogs {
		execCmd = exec.Command("tail", "-f", logPath)
	} else {
		execCmd = exec.Command("cat", logPath)
	}

	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Stdin = os.Stdin

	return execCmd.Run()
}

// Made with Bob
