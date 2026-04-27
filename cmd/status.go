package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sudeeshjohn/shiftlaunch/config"
	"github.com/sudeeshjohn/shiftlaunch/logger"
	"github.com/sudeeshjohn/shiftlaunch/orchestrator"
	"github.com/sudeeshjohn/shiftlaunch/types"
	"gopkg.in/yaml.v3"
)

// Status shows deployment status
func Status(ctx context.Context, orch *orchestrator.Orchestrator) error {
	log := orch.GetLogger()
	status := orch.GetClusterStatus(ctx)

	// Access the cluster name through the Orchestrator helper
	clusterName := orch.GetClusterName()

	log.Info("Checking cluster status", "cluster", clusterName)
	
	fmt.Println("================================================================================")
	fmt.Printf(" Deployment Status for: %s\n", clusterName)
	fmt.Println("================================================================================")
	fmt.Println()
	fmt.Println(status)
	fmt.Println("================================================================================")
	
	return nil
}

// StatusFromClusterDir is maintained for logic that needs to load status from a path
func StatusFromClusterDir(clusterName string, debug bool) error {
	// Load daemon config
	daemonCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load daemon config: %w", err)
	}

	workspaceDir := filepath.Join(daemonCfg.Paths.WorkspaceDir, clusterName)

	if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
		return fmt.Errorf("cluster '%s' not found in workspace", clusterName)
	}

	configPath := filepath.Join(workspaceDir, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to load cluster config: %w", err)
	}

	var cfg types.AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Correctly initialize logger with string path
	logPath := filepath.Join(workspaceDir, "deployment.log")
	appLogger, _ := logger.New(debug, logPath)

	orch := orchestrator.NewOrchestrator(&cfg, daemonCfg, appLogger, workspaceDir, debug)

	return Status(context.Background(), orch)
}