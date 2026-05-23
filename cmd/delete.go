package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.ibm.com/sudeeshjohn/shiftlaunch/types"
)

var removeForce bool

var removeCmd = &cobra.Command{
	Use:     "remove",
	Aliases: []string{"rm", "delete"},
	Short:   "Power off LPARs and remove local services",
	GroupID: "core",
	Long: `Safely tears down a cluster, unmaps storage, and cleans up local services.

The remove command will:
- Power off all cluster LPARs
- Remove network services (DNS, DHCP, PXE, Load Balancer)
- Clean up local workspace files
- Mark the cluster as deleted`,
	RunE: runRemove,
}

func init() {
	rootCmd.AddCommand(removeCmd)
	removeCmd.Flags().BoolVarP(&removeForce, "force", "f", false, "Forcefully delete without prompting for confirmation")
}

func runRemove(cmd *cobra.Command, args []string) error {
	// Require explicit --cluster or --config flag for safety
	configFlagSet := cmd.Flags().Changed("config")
	clusterFlagSet := cmd.Flags().Changed("cluster")
	
	if !configFlagSet && !clusterFlagSet {
		return fmt.Errorf("delete command requires explicit --cluster or --config flag\nUsage:\n  shiftlaunch delete --cluster <cluster-name>\n  shiftlaunch delete --config <config-file>")
	}
	
	cfg, _, orch, err := loadConfig(true)
	if err != nil {
		return err
	}
	
	// Ensure logger file descriptor is closed when command completes
	defer orch.GetLogger().Close()

	ctx := GetContext()
	log := orch.GetLogger()

	if orch.IsDeleted() {
		log.Info("Cluster is already deleted. Nothing to do.", "cluster", cfg.OpenShift.ClusterName)
		return nil
	}

	// INTERACTIVE PROMPT: Only prompt if --force is NOT passed
	if !removeForce {
		// Use pterm to make a beautiful, styled interactive confirmation prompt
		prompt := pterm.DefaultInteractiveConfirm.
			WithDefaultValue(false).
			WithTextStyle(pterm.NewStyle(pterm.FgLightYellow, pterm.Bold)).
			WithConfirmStyle(pterm.NewStyle(pterm.FgLightRed, pterm.Bold))
		
		result, err := prompt.Show(fmt.Sprintf("Are you sure you want to completely remove cluster '%s'?", cfg.OpenShift.ClusterName))
		if err != nil {
			return err
		}
		
		// If user selects "No", exit cleanly immediately
		if !result {
			log.Info("Teardown aborted by user.")
			return nil
		}
	}

	// Record command execution
	hostname, _ := os.Hostname()
	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME")
	}

	stateManager := types.NewStateManager(cfg.OpenShift.ClusterName)
	var phasesBefore []string
	if state, err := stateManager.LoadState(); err == nil && state != nil {
		phasesBefore = append([]string{}, state.CompletedPhases...)
	}

	cmdExec := types.CommandExecution{
		Command:      "remove",
		StartTime:    time.Now().Format(time.RFC3339),
		Status:       "in_progress",
		User:         username,
		Hostname:     hostname,
		PID:          os.Getpid(),
		ConfigFile:   configFile,
		PhasesBefore: phasesBefore,
		Flags: map[string]string{
			"debug":   fmt.Sprintf("%v", debug),
			"cluster": cfg.OpenShift.ClusterName,
			"force":   fmt.Sprintf("%v", removeForce),
		},
	}

	log.Info(fmt.Sprintf("=== Tearing Down Cluster: %s ===", cfg.OpenShift.ClusterName))

	cmdStartTime := time.Now()
	err = orch.Teardown(ctx)
	cmdDuration := time.Since(cmdStartTime)

	// Record command execution end
	cmdExec.EndTime = time.Now().Format(time.RFC3339)
	cmdExec.Duration = cmdDuration.String()
	if err != nil {
		cmdExec.Status = "failed"
		cmdExec.Error = err.Error()
	} else {
		cmdExec.Status = "success"
		log.Info("=== Cluster Teardown Completed Successfully ===")
	}

	// Update state with command execution
	if state, loadErr := stateManager.LoadState(); loadErr == nil && state != nil {
		cmdExec.PhasesAfter = append([]string{}, state.CompletedPhases...)
		state.Version = version
		stateManager.AddCommandExecution(state, cmdExec)
		stateManager.SaveState(state)
	}

	return err
}
