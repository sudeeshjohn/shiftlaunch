package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/sudeeshjohn/shiftlaunch/config"
	"github.com/sudeeshjohn/shiftlaunch/logger"
	"github.com/sudeeshjohn/shiftlaunch/types"
	"gopkg.in/yaml.v3"
)

var listQuiet bool
var listJson bool

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all managed clusters in the workspace",
	GroupID: "core",
	Long: `Lists all active clusters currently managed in the local workspace.

The list command displays:
- Cluster name and status
- Cluster IP (VIP)
- Deployment type (SNO/Multi, LPAR/BYOI)
- Current phase
- Duration
- Pre-provisioned services
- Last updated timestamp`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().BoolVarP(&listQuiet, "quiet", "q", false, "Only display cluster names")
	listCmd.Flags().BoolVar(&listJson, "json", false, "Output cluster list in pure JSON format")
}

func runList(cmd *cobra.Command, args []string) error {
	// Load daemon config to get workspace directory
	daemonCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load daemon config: %w", err)
	}

	return printClusterList(daemonCfg.Paths.WorkspaceDir)
}

// formatDuration formats a duration into a human-readable string matching the old architecture
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// printClusterList prints all managed clusters and their current states from the local workspace
func printClusterList(workspaceBase string) error {
	// Initialize a console-only logger to match the rest of the CLI
	log, _ := logger.New(debug, "")

	entries, err := os.ReadDir(workspaceBase)
	if err != nil {
		if !listQuiet && !listJson {
			log.Info("No clusters found or workspace directory does not exist.")
		} else if listJson {
			fmt.Println("[]") // Output empty array for valid JSON parsing
		}
		return nil
	}

	// Prepare table data for human output
	tableData := pterm.TableData{
		{"CLUSTER NAME", "STATUS", "CLUSTER IP", "TYPE", "PHASE", "DURATION", "PRE-PROVISIONED", "LAST UPDATED"},
	}

	visibleCount := 0
	clusterNames := []string{} // For quiet mode
	
	// Create a slice of maps for JSON output
	var jsonOutput []map[string]string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		clusterName := entry.Name()

		// Skip clusters marked as deleted to match the old shouldExposeCluster behavior [cite: 3]
		deletedMarker := filepath.Join(workspaceBase, clusterName, ".deleted")
		if _, err := os.Stat(deletedMarker); err == nil {
			continue
		}

		stateFile := filepath.Join(workspaceBase, clusterName, "state.json")
		configFile := filepath.Join(workspaceBase, clusterName, "config.yaml")

		// Try to load state
		state, err := types.LoadState(clusterName)
		if err != nil {
			if listQuiet {
				clusterNames = append(clusterNames, clusterName)
			} else {
				tableData = append(tableData, []string{
					clusterName, "unknown", "N/A", "N/A", "N/A", "N/A", "N/A", "N/A",
				})
				
				// Append corrupted state to JSON as well
				jsonOutput = append(jsonOutput, map[string]string{
					"name":   clusterName,
					"status": "unknown",
				})
			}
			visibleCount++
			continue
		}

		// Extract and format the pre_provisioned items and cluster type
		clusterType := "Multi/LPAR" // Default
		preProvStr := "Unknown"
		clusterIP := "Unknown" // --- FIX: Default value for Cluster IP ---

		data, err := os.ReadFile(configFile)
		if err == nil {
			var cfg types.AgentConfig
			if err := yaml.Unmarshal(data, &cfg); err == nil {

				// --- FIX: Extract the LoadBalancerIP (VIP) ---
				if cfg.Network.LoadBalancerIP != "" {
					clusterIP = cfg.Network.LoadBalancerIP
				}

				// Determine deployment type (SNO vs Multi-node)
				deploymentType := "Multi"
				if cfg.IsSNO() {
					deploymentType = "SNO"
				}

				// Evaluate new ManagedServices flags (inverted logic from BYOI pre-provisioned)
				// Boot method aware: DHCP/PXE only matter for netboot, NFS only matters for ISO
				var prepItems []string
				if !cfg.ManagedServices.DNS {
					prepItems = append(prepItems, "DNS")
				}
				
				// DHCP and PXE are only BYOI dependencies if doing a netboot
				if !cfg.ManagedServices.DHCP && cfg.Nodes.BootMethod != "iso" {
					prepItems = append(prepItems, "DHCP")
				}
				if !cfg.ManagedServices.PXE && cfg.Nodes.BootMethod != "iso" {
					prepItems = append(prepItems, "PXE")
				}
				
				if !cfg.ManagedServices.LoadBalancer {
					prepItems = append(prepItems, "LB")
				}
				
				// NFS is a BYOI dependency ONLY if doing an ISO boot
				if !cfg.ManagedServices.NFS && cfg.Nodes.BootMethod == "iso" {
					prepItems = append(prepItems, "NFS")
				}

				// Format the display string
				provisioningType := "LPAR"
				if len(prepItems) > 0 {
					preProvStr = strings.Join(prepItems, ",")
					provisioningType = "BYOI"
				} else {
					preProvStr = "None"
				}

				clusterType = fmt.Sprintf("%s/%s", deploymentType, provisioningType)
			}
		}

		// Format timestamp (Last Updated)
		timestamp := "N/A"
		// If EndTime exists, use it. Otherwise fallback to the file modification time. [cite: 6]
		if state.EndTime != "" {
			if t, err := time.Parse(time.RFC3339, state.EndTime); err == nil {
				timestamp = t.Format("2006-01-02 15:04:05")
			}
		} else {
			if info, err := os.Stat(stateFile); err == nil {
				timestamp = info.ModTime().Format("2006-01-02 15:04:05")
			}
		}

		// Calculate deployment duration
		duration := "N/A"
		if state.StartTime != "" {
			startTime, err := time.Parse(time.RFC3339, state.StartTime)
			if err == nil {
				var d time.Duration
				if state.EndTime != "" {
					// Deployment completed or failed
					endTime, err := time.Parse(time.RFC3339, state.EndTime)
					if err == nil {
						d = endTime.Sub(startTime)
					}
				} else if state.Status == "in_progress" {
					// Deployment still in progress
					d = time.Since(startTime)
					duration = formatDuration(d) + "*"
				}

				if d > 0 && duration == "N/A" {
					duration = formatDuration(d)
				}
			}
		}

		// Add row to lists
		if listQuiet {
			clusterNames = append(clusterNames, clusterName)
		} else {
			tableData = append(tableData, []string{
				clusterName,
				state.Status,
				clusterIP,
				clusterType,
				state.CurrentPhase,
				duration,
				preProvStr,
				timestamp,
			})
			
			// Append valid row to JSON array
			jsonOutput = append(jsonOutput, map[string]string{
				"name":            clusterName,
				"status":          state.Status,
				"cluster_ip":      clusterIP,
				"type":            clusterType,
				"phase":           state.CurrentPhase,
				"duration":        duration,
				"pre_provisioned": preProvStr,
				"last_updated":    timestamp,
			})
		}

		visibleCount++
	}

	// Output logic based on flags
	if listJson {
		// Output pure JSON array
		if jsonOutput == nil {
			fmt.Println("[]")
			return nil
		}
		jsonData, _ := json.MarshalIndent(jsonOutput, "", "  ")
		fmt.Println(string(jsonData))
		return nil
	}

	if visibleCount == 0 {
		if !listQuiet && !listJson {
			log.Info("No active clusters found.")
			log.Info("Deleted workspaces are hidden. Use 'shiftlaunch prune' to reclaim disk space.")
		}
		return nil
	}

	// Render output based on mode
	if listQuiet {
		// Quiet mode: just print cluster names
		for _, name := range clusterNames {
			fmt.Println(name)
		}
	} else {
		// Normal mode: render beautiful pterm table
		pterm.DefaultTable.
			WithHasHeader().
			WithHeaderStyle(pterm.NewStyle(pterm.FgCyan, pterm.Bold)).
			WithData(tableData).
			Render()
		
		fmt.Printf("Total clusters: %d\n", visibleCount)
	}

	return nil
}
