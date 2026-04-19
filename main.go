package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/sudeeshjohn/shiftlaunch/cmd"
	"github.com/sudeeshjohn/shiftlaunch/config"
	"github.com/sudeeshjohn/shiftlaunch/infra/compute"
	"github.com/sudeeshjohn/shiftlaunch/infra/controller"
	"github.com/sudeeshjohn/shiftlaunch/localexec"
	"github.com/sudeeshjohn/shiftlaunch/logger"
	"github.com/sudeeshjohn/shiftlaunch/orchestrator"
	"github.com/sudeeshjohn/shiftlaunch/types"
	"github.com/sudeeshjohn/shiftlaunch/validation"
)

const version = "0.3.0-byoi-agent"

func main() {
	// Define command-line flags
	command := flag.String("command", "", "Command to execute: create, delete, validate, status, list, dump-config, version")
	configFile := flag.String("config", "config.yaml", "Path to agent configuration file")
	clusterName := flag.String("cluster", "", "Cluster name override")
	debug := flag.Bool("debug", false, "Enable debug output to terminal (all logs always saved to workspace)")
	flag.Parse()

	// --- NEW: Allow positional arguments to act as the command ---
	if *command == "" && flag.NArg() > 0 {
		*command = flag.Arg(0)
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if *command == "version" {
		fmt.Printf("ShiftLaunch Local Agent v%s\n", version)
		fmt.Println("A tool for bootstrapping OpenShift clusters on IBM Power Systems")
		os.Exit(0)
	}

	if *command == "help" || *command == "-h" || *command == "--help" || *command == "" {
		printUsage()
		os.Exit(0)
	}

	// --- NEW: Handle list command early so it doesn't try to load config.yaml ---
	if *command == "list" {
		if err := cmd.List(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// ---------------------------------------------------------
	// Graceful Shutdown & Signal Handling
	// ---------------------------------------------------------
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\n\n[WARNING] Interrupt signal received! Attempting graceful shutdown...")
		cancel()
		fmt.Println("Shutdown complete. Exiting...")
		os.Exit(130)
	}()

	// --- NEW: Load the Agent Daemon configuration ---
	daemonCfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to initialize daemon configuration: %v", err)
	}

	// ---------------------------------------------------------
	// Configuration Loading & Workspace Setup
	// ---------------------------------------------------------
	var cfg types.AgentConfig
	configPath := *configFile

	// If deleting or checking status, look for config in the workspace
	if (*command == "delete" || *command == "status") && *clusterName != "" {
		workspaceConfig := filepath.Join(daemonCfg.Paths.WorkspaceDir, *clusterName, "config.yaml")
		if _, err := os.Stat(workspaceConfig); err == nil {
			configPath = workspaceConfig
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Failed to read configuration file: %v\n(Hint: Provide a valid config.yaml or specify the cluster name if it was already created)", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("Failed to parse YAML configuration: %v", err)
	}
	
	// Debug: Log config source and node counts for delete/status commands
	if *command == "delete" || *command == "status" {
		log.Printf("Loaded config from: %s", configPath)
		log.Printf("Config validation: SNO=%v, Bootstrap=%d, Masters=%d, Workers=%d",
			cfg.IsSNO(), len(cfg.Nodes.Bootstrap), len(cfg.Nodes.Masters), len(cfg.Nodes.Workers))
	}
	// --- NEW: Auto-Discover Controller IP ---
	if cfg.Controller.NetworkInterface != "" {
		ip, err := controller.GetInterfaceIPv4(cfg.Controller.NetworkInterface)
		if err != nil {
			log.Fatalf("Failed to auto-discover Controller IP: %v", err)
		}
		cfg.Controller.IP = ip
	} else {
		log.Fatalf("controller.network_interface must be specified in the configuration file")
	}

	// Override cluster name if provided via flag
	if *clusterName != "" {
		cfg.OpenShift.ClusterName = *clusterName
	} else {
		*clusterName = cfg.OpenShift.ClusterName
	}

	if *clusterName == "" {
		log.Fatalf("Cluster name must be specified in config or via -cluster flag")
	}

	workspaceDir := filepath.Join(daemonCfg.Paths.WorkspaceDir, *clusterName)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		log.Fatalf("Failed to create workspace directory: %v", err)
	}

	// Save a master copy of the config to the workspace for future resume/delete/status commands
	if *command == "create" {
		os.MkdirAll(workspaceDir, 0755)
		
		// Check if cluster is already managed by ShiftLaunch
		managedMarker := filepath.Join(workspaceDir, ".managed")
		deletedMarker := filepath.Join(workspaceDir, ".deleted")
		existingConfigPath := filepath.Join(workspaceDir, "config.yaml")
		
		// Check if cluster was deleted - if so, allow re-creation
		if _, err := os.Stat(deletedMarker); err == nil {
			log.Printf("Cluster '%s' was previously deleted. Clearing markers for fresh deployment...", *clusterName)
			os.Remove(deletedMarker)
			os.Remove(managedMarker)
		}
		
		// If .managed marker exists and user is providing a different config file, fail
		if _, err := os.Stat(managedMarker); err == nil {
			// Cluster is already managed
			if *configFile != existingConfigPath {
				// User is trying to create with a different config file
				log.Fatalf("Error: Cluster '%s' is already managed by ShiftLaunch.\n"+
					"  Workspace: %s\n"+
					"  To resume deployment: ./shiftlaunch -command create -cluster %s\n"+
					"  To delete cluster: ./shiftlaunch -command delete -cluster %s\n"+
					"  To check status: ./shiftlaunch -command status -cluster %s",
					*clusterName, workspaceDir, *clusterName, *clusterName, *clusterName)
			}
			// User is resuming with same cluster name (no config file specified)
			// This is allowed - the existing config will be used
			log.Printf("Resuming managed cluster: %s", *clusterName)
		} else {
			// New cluster - save the config
			var configBackupPath string
			if _, err := os.Stat(existingConfigPath); err == nil {
				// Config exists but no .managed marker (shouldn't happen, but handle it)
				timestamp := time.Now().Format("20060102-150405")
				configBackupPath = filepath.Join(workspaceDir, fmt.Sprintf("config.yaml.backup.%s", timestamp))
				if err := os.Rename(existingConfigPath, configBackupPath); err != nil {
					log.Printf("Warning: Failed to backup existing config: %v", err)
				} else {
					log.Printf("Backed up existing config to: %s", configBackupPath)
					
					// Record backup in state
					stateManager := types.NewStateManager(*clusterName)
					if state, err := stateManager.LoadState(); err == nil && state != nil {
						stateManager.AddConfigBackup(state, configBackupPath)
						stateManager.SaveState(state)
					}
				}
			}
			
			// Write new config
			os.WriteFile(existingConfigPath, data, 0644)
		}
	}

	// Setup logging
	var orchMutex sync.Mutex
	logFilePath := filepath.Join(workspaceDir, "deployment.log")
	appLogger, err := logger.New(*debug, logFilePath)
	if err != nil {
		// Fallback: If file logging fails, at least provide a console logger so we don't panic
		fmt.Printf("⚠️  Warning: Could not setup file logging: %v. Using console only.\n", err)
		appLogger, _ = logger.New(*debug, "") // Assuming your logger handles empty string as console-only
	}

	orchMutex.Lock()
	orch := orchestrator.NewOrchestrator(&cfg, daemonCfg, appLogger, workspaceDir, *debug)
	orchMutex.Unlock()

	// ---------------------------------------------------------
	// Auto-Resume Detection & Command Tracking
	// ---------------------------------------------------------
	autoResume := false
	stateManager := types.NewStateManager(*clusterName)
	var phasesBefore []string
	
	if *command == "create" {
		if state, err := stateManager.LoadState(); err == nil && state != nil {
			phasesBefore = append([]string{}, state.CompletedPhases...)
			if state.Status == "in_progress" && len(state.CompletedPhases) > 0 {
				autoResume = true
				state.ResumeCount++
				stateManager.SaveState(state)
				appLogger.Info("Detected existing deployment. Automatically resuming from last phase...",
					"cluster", *clusterName,
					"last_phase", state.CurrentPhase,
					"completed_phases", len(state.CompletedPhases),
					"resume_count", state.ResumeCount)
			}
		}
	}
	
	// Record command execution start
	hostname, _ := os.Hostname()
	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME")
	}
	
	cmdExec := types.CommandExecution{
		Command:      *command,
		StartTime:    time.Now().Format(time.RFC3339),
		Status:       "in_progress",
		User:         username,
		Hostname:     hostname,
		PID:          os.Getpid(),
		ConfigFile:   *configFile,
		PhasesBefore: phasesBefore,
		Flags: map[string]string{
			"debug":   fmt.Sprintf("%v", *debug),
			"cluster": *clusterName,
		},
	}

	// ---------------------------------------------------------
	// CLI Routing
	// ---------------------------------------------------------
	cmdStartTime := time.Now()
	err = runCLI(ctx, orch, &cfg, *command, *debug, autoResume)
	cmdDuration := time.Since(cmdStartTime)
	
	// Record command execution end
	cmdExec.EndTime = time.Now().Format(time.RFC3339)
	cmdExec.Duration = cmdDuration.String()
	if err != nil {
		cmdExec.Status = "failed"
		cmdExec.Error = err.Error()
	} else {
		cmdExec.Status = "success"
	}
	
	// Get phases after execution
	if state, loadErr := stateManager.LoadState(); loadErr == nil && state != nil {
		cmdExec.PhasesAfter = append([]string{}, state.CompletedPhases...)
		state.Version = version
		stateManager.AddCommandExecution(state, cmdExec)
		stateManager.SaveState(state)
	}
	
	if err != nil {
		fmt.Printf("\nError: %v\n", err)
		os.Exit(1)
	}
}

// runCLI routes the command execution safely
func runCLI(ctx context.Context, orch *orchestrator.Orchestrator, cfg *types.AgentConfig, command string, debug, resume bool) error {
	switch command {
	case "list":
		return cmd.List()
	case "validate":
		fmt.Println("Validating configuration...")
		exec := localexec.NewLocalClient(orch.GetLogger())
		v := validation.NewValidator(cfg, exec, debug)
		v.SetLogger(orch.GetLogger())
		
		// Set up HMC client for Phase 3 validation (LPAR existence checks)
		provider, err := compute.NewProvider(cfg, orch.GetLogger(), debug)
		if err != nil {
			orch.GetLogger().Warn("Could not connect to HMC for validation. Skipping Phase 3 (LPAR validation).", "error", err)
		} else {
			// Extract the HMC client from the provider
			if hmcProvider, ok := provider.(*compute.HMCProvider); ok {
				v.SetHMCClient(hmcProvider.GetHMCClient())
			}
		}
		
		return v.Validate()
	case "create":
		return orch.Deploy(resume)
	case "delete":
		return orch.Teardown()
	case "status":
		fmt.Println(orch.GetClusterStatus())
		return nil
	case "dump-config":
		return orch.DumpConfigs()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		return fmt.Errorf("invalid command provided")
	}
}

func printUsage() {
	fmt.Println("ShiftLaunch Local Agent - Boot OpenShift clusters on IBM Power Systems")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  shiftlaunch -command <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  validate     Validate cluster configuration against infrastructure")
	fmt.Println("  create       Execute cluster deployment pipeline")
	fmt.Println("  delete       Power off LPARs and remove local services")
	fmt.Println("  status       Show cluster deployment status and endpoints")
	fmt.Println("  list         List all managed clusters in the workspace")
	fmt.Println("  dump-config  Dump configuration requirements for unmanaged services")
	fmt.Println("  version      Show version information")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -config string")
	fmt.Println("        Path to configuration file (default: config.yaml)")
	fmt.Println("  -cluster string")
	fmt.Println("        Cluster name override")
	fmt.Println("  -debug")
	fmt.Println("        Enable debug output to terminal")
	fmt.Println()
	fmt.Println("Note: The 'create' command automatically resumes from the last completed phase")
	fmt.Println("      if an existing deployment is detected for the specified cluster.")
	fmt.Println()
}