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

	"gopkg.in/yaml.v3"

	"github.com/sudeeshjohn/shiftlaunch/cmd"
	"github.com/sudeeshjohn/shiftlaunch/infra/compute"
	"github.com/sudeeshjohn/shiftlaunch/infra/controller"
	"github.com/sudeeshjohn/shiftlaunch/localexec"
	"github.com/sudeeshjohn/shiftlaunch/logger"
	"github.com/sudeeshjohn/shiftlaunch/orchestrator"
	"github.com/sudeeshjohn/shiftlaunch/types"
	"github.com/sudeeshjohn/shiftlaunch/validation"
)

const version = "0.2.0-local-agent"

func main() {
	// Define command-line flags
	command := flag.String("command", "", "Command to execute: create, delete, validate, status, list, dump-config, version")
	configFile := flag.String("config", "config.yaml", "Path to agent configuration file")
	clusterName := flag.String("cluster", "", "Cluster name override")
	debug := flag.Bool("debug", false, "Enable debug output to terminal (all logs always saved to workspace)")
	resume := flag.Bool("resume", false, "Resume deployment from last failed phase")
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

	// ---------------------------------------------------------
	// Configuration Loading & Workspace Setup
	// ---------------------------------------------------------
	var cfg types.AgentConfig
	configPath := *configFile

	// If deleting, checking status, or resuming without a provided config, look in the workspace
	if (*command == "delete" || *command == "status" || (*command == "create" && *resume)) && *clusterName != "" {
		workspaceConfig := filepath.Join("/opt/shiftlaunch/clusters", *clusterName, "config.yaml")
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

	workspaceDir := filepath.Join("/opt/shiftlaunch/clusters", *clusterName)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		log.Fatalf("Failed to create workspace directory: %v", err)
	}

	// Save a master copy of the config to the workspace for future resume/delete/status commands
	if *command == "create" {
		os.MkdirAll(workspaceDir, 0755)
		os.WriteFile(filepath.Join(workspaceDir, "config.yaml"), data, 0644)
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
	orch := orchestrator.NewOrchestrator(&cfg, appLogger, workspaceDir, *debug)
	orchMutex.Unlock()

	// ---------------------------------------------------------
	// CLI Routing
	// ---------------------------------------------------------
	if err := runCLI(ctx, orch, &cfg, *command, *debug, *resume); err != nil {
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
	fmt.Println("  -resume")
	fmt.Println("        Resume cluster creation from last failed phase")
	fmt.Println()
}