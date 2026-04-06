package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	compute "github.com/sudeeshjohn/shiftlaunch/infra/compute" // <--- Add the 'compute' alias here
	"github.com/sudeeshjohn/shiftlaunch/infra/controller"
	"github.com/sudeeshjohn/shiftlaunch/localexec"
	"github.com/sudeeshjohn/shiftlaunch/logger"
	"github.com/sudeeshjohn/shiftlaunch/services"
	"github.com/sudeeshjohn/shiftlaunch/types"
)

// Orchestrator manages the local execution of the BYOI Boot Agent
type Orchestrator struct {
	cfg          *types.AgentConfig
	logger       *logger.Logger
	executor     *localexec.LocalClient
	workspaceDir string
	state        *types.DeploymentState
	stateManager *types.StateManager
	debug        bool
}

// NewOrchestrator initializes the phase engine and loads/creates the state tracker
func NewOrchestrator(cfg *types.AgentConfig, log *logger.Logger, workspaceDir string, debug bool) *Orchestrator {
	stateManager := types.NewStateManager(cfg.OpenShift.ClusterName)
	
	// Attempt to load existing state for resumes
	state, err := stateManager.LoadState()
	if err != nil || state == nil {
		// Fresh deployment
		state = &types.DeploymentState{
			ClusterName:     cfg.OpenShift.ClusterName,
			Status:          "in_progress",
			StartTime:       time.Now().Format(time.RFC3339),
			CompletedPhases: []string{},
		}
	}

	return &Orchestrator{
		cfg:          cfg,
		logger:       log,
		executor:     localexec.NewLocalClient(log),
		workspaceDir: workspaceDir,
		state:        state,
		stateManager: stateManager,
		debug:        debug,
	}
}
// GetDebug returns the debug flag status
func (o *Orchestrator) GetDebug() bool {
    return o.debug
}

// saveState records phase completion to the local state.json file
func (o *Orchestrator) saveState(phase string) {
	o.state.CurrentPhase = phase
	if !contains(o.state.CompletedPhases, phase) {
		o.state.CompletedPhases = append(o.state.CompletedPhases, phase)
	}
	_ = o.stateManager.SaveState(o.state)
}

// Deploy executes the linear installation pipeline
func (o *Orchestrator) Deploy(resume bool) error {
	// Check if cluster was previously deleted
	if o.stateManager.IsDeleted() && !resume {
		o.logger.Info("⚠️  Cluster was previously deleted. Clearing deleted marker for new deployment...", "cluster", o.cfg.OpenShift.ClusterName)
		if err := o.stateManager.ClearDeleted(); err != nil {
			return fmt.Errorf("failed to clear deleted marker: %w", err)
		}
		// Reset state for fresh deployment
		o.state = &types.DeploymentState{
			ClusterName:     o.cfg.OpenShift.ClusterName,
			Status:          "in_progress",
			StartTime:       time.Now().Format(time.RFC3339),
			CompletedPhases: []string{},
		}
	}

	// Acquire lock to prevent concurrent deployments
	if err := o.stateManager.AcquireLock(); err != nil {
		return fmt.Errorf("failed to acquire cluster lock: %w", err)
	}
	// Ensure lock is always released, even on error
	defer func() {
		if err := o.stateManager.ReleaseLock(); err != nil {
			o.logger.Warn("Failed to release lock", "error", err)
		}
	}()

	// Mark cluster as managed by ShiftLaunch
	if err := o.stateManager.MarkManaged(); err != nil {
		o.logger.Warn("Failed to create managed marker", "error", err)
	}

	o.logger.Info("🚀 Starting ShiftLaunch Local Agent Orchestration...", "cluster", o.cfg.OpenShift.ClusterName)

// --- PHASE 1: DISCOVERY ---
	if !resume || !contains(o.state.CompletedPhases, "discovery") {
		o.logger.Info("\n[Phase 1] Pre-Flight & HMC Discovery")
		
		provider, err := compute.NewProvider(o.cfg, o.logger, o.debug)
		if err != nil {
			return fmt.Errorf("failed to initialize compute provider: %w", err)
		}
		defer func() {
			if hmcProvider, ok := provider.(*compute.HMCProvider); ok {
				hmcProvider.Cleanup()
			}
		}()
		
		if err := provider.DiscoverMetadata(context.Background()); err != nil {
			return fmt.Errorf("failed to discover LPAR metadata from HMC: %w", err)
		}
		
// --- NEW: Print Comprehensive Discovered Metadata Summary ---
		fmt.Println("\n=========================================================================================================================================================")
		fmt.Println(" DISCOVERED LPAR METADATA SUMMARY")
		fmt.Println("=========================================================================================================================================================")
		fmt.Printf("%-10s | %-20s | %-15s | %-18s | %-25s | %-17s | %-20s | %s\n",
			"ROLE", "HOSTNAME", "IP ADDRESS", "SYSTEM NAME", "LPAR NAME", "MAC ADDRESS", "LOCATION CODE", "UUID")
		fmt.Println(strings.Repeat("-", 153))
		
		for _, node := range o.cfg.GetAllNodes() {
			mac := node.MACAddress
			if mac == "" {
				mac = "<pending>"
			}
			loc := node.LocationCode
			if loc == "" {
				loc = "<pending>"
			}
			uuid := node.UUID
			if uuid == "" {
				uuid = "<pending>"
			}

			fmt.Printf("%-10s | %-20s | %-15s | %-18s | %-25s | %-17s | %-20s | %s\n",
				strings.ToUpper(node.Role),
				node.Hostname,
				node.IP,
				node.SystemName,
				node.ExistingLPARName,
				mac,
				loc,
				uuid)
		}
		fmt.Println(strings.Repeat("-", 153))
		fmt.Println()

		o.saveState("discovery")
	}
	// --- PHASE 2: DOWNLOADS ---
	if !resume || !contains(o.state.CompletedPhases, "downloads") {
		o.logger.Info("\n[Phase 2] Downloading OpenShift Artifacts")
		
		downloader := services.NewDownloader(o.cfg, o.executor, o.logger)
		if err := downloader.DownloadAll(o.workspaceDir); err != nil {
			return err
		}
		
		o.saveState("downloads")
	}

	// --- PHASE 3: MANAGED SERVICES ---
	if !resume || !contains(o.state.CompletedPhases, "services") {
		o.logger.Info("\n[Phase 3] Configuring Managed Infrastructure Services")

		setup := services.NewControllerSetup(o.cfg, o.executor, o.logger)
		if err := setup.InstallPackages(); err != nil {
			return fmt.Errorf("failed to setup controller dependencies: %w", err)
		}
		if err := setup.ConfigureFirewall(); err != nil {
			return fmt.Errorf("failed to configure local firewall: %w", err)
		}

		if o.cfg.ManagedServices.LoadBalancer {
			o.logger.Info(" -> Configuring Local HAProxy...")

			// 1. Allow HAProxy to bind to the VIP immediately (fixes the systemd crash)
			o.executor.Execute("sudo sysctl -w net.ipv4.ip_nonlocal_bind=1")
			
			// 2. Allow HAProxy to route OpenShift's custom ports through SELinux
			o.executor.Execute("sudo setsebool -P haproxy_connect_any 1")

			// 3. Bind the VIP to the controller's physical network interface
			netMgr := controller.NewNetworkManager(o.executor, o.debug, o.logger)
			vip := o.cfg.Network.LoadBalancerIP
			iface := o.cfg.Controller.NetworkInterface
			cidr := o.cfg.Network.MachineCIDR
			
			if err := netMgr.AddVIPAlias(iface, vip, cidr); err != nil {
				o.logger.Warn("Failed to configure VIP via nmcli. HAProxy will start, but routing may fail.", "error", err)
			}

			// 4. Generate config and start the service
			if err := services.SetupHAProxy(o.cfg, o.executor); err != nil {
				return err
			}
		} else {
			o.logger.Info(" -> Skipping HAProxy (User Managed)")
		}

		dnsmasq := services.NewDNSmasqManager(o.cfg, o.executor)

		if o.cfg.ManagedServices.DNS {
			o.logger.Info(" -> Configuring Local DNS...")
			if err := dnsmasq.SetupDNS(); err != nil {
				return err
			}
		} else {
			o.logger.Info(" -> Skipping DNS (User Managed)")
		}

		if o.cfg.ManagedServices.DHCP {
			o.logger.Info(" -> Configuring Local DHCP...")
			if err := dnsmasq.SetupDHCP(); err != nil {
				return err
			}
		} else {
			o.logger.Info(" -> Skipping DHCP (User Managed)")
		}

		if o.cfg.ManagedServices.PXE {
			o.logger.Info(" -> Configuring Local PXE Service...")
			if err := dnsmasq.SetupPXEService(); err != nil {
				return err
			}
			o.logger.Info(" -> Staging PXE Artifacts...")
			if err := dnsmasq.ConfigurePXEBoot(o.workspaceDir); err != nil {
				return err
			}
		} else {
			o.logger.Info(" -> Skipping PXE (User Managed)")
		}

		o.saveState("services")
	}

	// --- PHASE 4: IGNITION GENERATION ---
	if !resume || !contains(o.state.CompletedPhases, "ignition") {
		o.logger.Info("\n[Phase 4] Generating OpenShift Ignition Payload")
		
		if err := services.GenerateIgnition(o.cfg, o.executor, o.workspaceDir); err != nil {
			return err
		}

		o.logger.Info(" -> Setting up Local HTTP Server (Port 8080)...")
		
		// --- NEW: Configure and Start Apache HTTPD ---
		if err := services.ConfigureHTTPD(o.executor); err != nil {
			return err
		}

		httpServer := services.NewHTTPServerManager(o.cfg, o.executor, o.logger)
		if err := httpServer.Setup(o.workspaceDir); err != nil {
			return err
		}
		
		o.logger.Info(" -> Staging files to HTTP Server...")
		if err := httpServer.StageFiles(o.workspaceDir); err != nil {
			return err
		}

		o.saveState("ignition")
	}

	// --- PHASE 5: BOOT ---
	if !resume || !contains(o.state.CompletedPhases, "boot") {
		o.logger.Info("\n[Phase 5] Initiating Cluster Boot")
		
		provider, err := compute.NewProvider(o.cfg, o.logger, o.debug)
		if err != nil {
			return err
		}
		// Ensure HMC session is cleaned up after boot phase
		defer func() {
			if hmcProvider, ok := provider.(*compute.HMCProvider); ok {
				hmcProvider.Cleanup()
			}
		}()

		// Re-discover metadata if resuming (UUIDs may not be populated)
		if resume {
			o.logger.Info("Re-discovering LPAR metadata for resume...")
			if err := provider.DiscoverMetadata(context.Background()); err != nil {
				return fmt.Errorf("failed to re-discover LPAR metadata: %w", err)
			}
		}

		// Iterate through nodes and check individual boot states
		for _, node := range o.cfg.GetAllNodes() {
			bootMarker := "booted_" + node.Hostname
			
			// Skip this specific node if it was successfully booted in a previous run
			if resume && contains(o.state.CompletedPhases, bootMarker) {
				o.logger.Info("Skipping already booted node", "node", node.Hostname)
				continue
			}

			if err := provider.BootNode(context.Background(), node); err != nil {
				return fmt.Errorf("HMC boot sequence failed for %s: %w", node.Hostname, err)
			}
			
			// Mark this specific node as successfully booted in state.json
			o.saveState(bootMarker)
		}
		
		// Mark the entire phase as fully complete
		o.saveState("boot")
	}

	// --- PHASE 6: WAIT FOR INSTALLATION ---
	if !resume || !contains(o.state.CompletedPhases, "wait") {
		o.logger.Info("\n[Phase 6] Waiting for OpenShift Installation")
		
		if err := o.WaitForBootstrap(context.Background()); err != nil {
			return err
		}

		if err := o.WaitForInstall(context.Background()); err != nil {
			return err
		}

		o.saveState("wait")
	}

	// Deployment Complete! Update State.
	o.state.Status = "completed"
	o.state.CurrentPhase = "done"
	o.state.EndTime = time.Now().Format(time.RFC3339)
	_ = o.stateManager.SaveState(o.state)

	o.logger.Info("\n🎉 ShiftLaunch Agent Execution Complete! OpenShift is ready.")
	return nil
}


// GetClusterStatus returns a formatted string of the current deployment state
func (o *Orchestrator) GetClusterStatus() string {
	state := o.state
	
	status := fmt.Sprintf(`Cluster: %s
Status: %s
Current Phase: %s
Completed Phases: %v
Started: %s
`, state.ClusterName, state.Status, state.CurrentPhase, state.CompletedPhases, state.StartTime)

	if state.EndTime != "" {
		status += fmt.Sprintf("Ended: %s\n", state.EndTime)
	}

	if state.Status == "completed" {
		status += "\n=== Service Endpoints ===\n"
		baseDomain := o.cfg.OpenShift.BaseDomain
		clusterDomain := fmt.Sprintf("%s.%s", state.ClusterName, baseDomain)
		
		status += fmt.Sprintf("API Server:       https://api.%s:6443\n", clusterDomain)
		status += fmt.Sprintf("Web Console:      https://console-openshift-console.apps.%s\n", clusterDomain)
		
		// Add kubeadmin credentials if available locally
		kubeconfigPath := filepath.Join(o.workspaceDir, "auth", "kubeconfig")
		pwPath := filepath.Join(o.workspaceDir, "auth", "kubeadmin-password")
		
		if _, err := os.Stat(kubeconfigPath); err == nil {
			status += fmt.Sprintf("\nKubeconfig:       %s\n", kubeconfigPath)
		}
		if pwData, err := os.ReadFile(pwPath); err == nil {
			status += fmt.Sprintf("Password:         %s\n", string(pwData))
		}
	}

	return status
}

// DumpConfigs outputs required configuration records for Enterprise Admins
func (o *Orchestrator) DumpConfigs() error {
	o.logger.Info("Dumping configuration requirements for unmanaged services...")

	if !o.cfg.ManagedServices.DNS {
		fmt.Printf("\n--- REQUIRED DNS RECORDS ---\n")
		fmt.Printf("%s\tapi.%s.%s\n", o.cfg.Network.LoadBalancerIP, o.cfg.OpenShift.ClusterName, o.cfg.OpenShift.BaseDomain)
		fmt.Printf("%s\tapi-int.%s.%s\n", o.cfg.Network.LoadBalancerIP, o.cfg.OpenShift.ClusterName, o.cfg.OpenShift.BaseDomain)
		fmt.Printf("%s\t*.apps.%s.%s\n", o.cfg.Network.LoadBalancerIP, o.cfg.OpenShift.ClusterName, o.cfg.OpenShift.BaseDomain)
	}

	if !o.cfg.ManagedServices.LoadBalancer {
		fmt.Printf("\n--- REQUIRED LOAD BALANCER POOLS (Target: %s) ---\n", o.cfg.Network.LoadBalancerIP)
		fmt.Println("Port 6443 (API)   -> Masters & Bootstrap")
		fmt.Println("Port 22623 (MCS)  -> Masters & Bootstrap")
		fmt.Println("Port 80/443 (App) -> Workers (or Masters if SNO/Compact)")
	}

	if !o.cfg.ManagedServices.DHCP {
		fmt.Printf("\n--- REQUIRED DHCP OPTIONS ---\n")
		fmt.Printf("Option 66 (Next-Server): %s\n", o.cfg.Controller.IP)
		fmt.Printf("Option 67 (Bootfile):    %s/core.elf\n", o.cfg.OpenShift.ClusterName)
	}

	return nil
}

// Helper to check if a string exists in a slice
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
// GetLogger returns the orchestrator's logger instance
func (o *Orchestrator) GetLogger() *logger.Logger {
	return o.logger
}
// GetClusterName returns the cluster name from the configuration
func (o *Orchestrator) GetClusterName() string {
	return o.cfg.OpenShift.ClusterName
}

