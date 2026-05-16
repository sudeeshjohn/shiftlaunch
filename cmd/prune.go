package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/sudeeshjohn/shiftlaunch/config"
	"github.com/sudeeshjohn/shiftlaunch/logger"
)

var pruneCmd = &cobra.Command{
	Use:     "prune",
	Short:   "Remove all deleted cluster workspaces",
	GroupID: "core",
	Long: `Reclaim disk space by permanently deleting cluster workspaces that have been marked as deleted by the 'remove' command.

This command scans the workspace directory for clusters with a .deleted marker and permanently removes them from disk.

Examples:
  shiftlaunch prune                    # Remove all deleted cluster workspaces
  shiftlaunch prune --cluster prod     # Not applicable (prunes all deleted clusters)`,
	RunE: runPrune,
}

func init() {
	rootCmd.AddCommand(pruneCmd)
}

func runPrune(cmd *cobra.Command, args []string) error {
	daemonCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load daemon config: %w", err)
	}

	// Initialize a console-only logger for the CLI command
	log, _ := logger.New(debug, "")

	workspaceBase := daemonCfg.Paths.WorkspaceDir
	entries, err := os.ReadDir(workspaceBase)
	if err != nil {
		log.Warn("Workspace directory does not exist or cannot be read.")
		return nil
	}

	reclaimedDirs := 0
	log.Info("Scanning for deleted clusters to prune...")

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		clusterDir := filepath.Join(workspaceBase, entry.Name())
		deletedMarker := filepath.Join(clusterDir, ".deleted")

		// If the .deleted marker exists, nuke the directory
		if _, err := os.Stat(deletedMarker); err == nil {
			if err := os.RemoveAll(clusterDir); err != nil {
				log.Warn("Failed to prune workspace", "cluster", entry.Name(), "error", err)
			} else {
				log.Info("Pruned workspace", "cluster", entry.Name())
				reclaimedDirs++
			}
		}
	}

	if reclaimedDirs == 0 {
		log.Info("No deleted clusters found. Nothing to prune.")
	} else {
		// Handle grammar for 1 vs many
		workspaceStr := "workspaces"
		if reclaimedDirs == 1 {
			workspaceStr = "workspace"
		}
		log.Info(fmt.Sprintf("=== Successfully pruned %d cluster %s ===", reclaimedDirs, workspaceStr))
	}

	return nil
}

// Made with Bob
