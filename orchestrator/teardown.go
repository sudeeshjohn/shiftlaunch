package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	compute "github.com/IBM/shiftlaunch/infra/compute"
	"github.com/IBM/shiftlaunch/infra/controller"
	"github.com/IBM/shiftlaunch/services"
)

// Teardown safely powers off LPARs, removes local services, and marks workspace as deleted
func (o *Orchestrator) Teardown(ctx context.Context) error {
	// Check if already deleted
	if o.stateManager.IsDeleted() {
		o.logger.Info("Cluster is already marked as deleted. Skipping teardown.", "cluster", o.cfg.OpenShift.ClusterName)
		return nil
	}

	// Acquire lock to prevent concurrent operations
	if err := o.stateManager.AcquireLock(); err != nil {
		o.logger.Error("Failed to start teardown", "error", err)
		return err
	}
	// Ensure lock is always released, even on error
	defer func() {
		if err := o.stateManager.ReleaseLock(); err != nil {
			o.logger.Warn("Failed to release lock", "error", err)
		}
	}()

	// Downgraded to Debug to keep the terminal clean
	o.logger.Debug("Initiating Soft Teardown", "cluster", o.cfg.OpenShift.ClusterName)

	// ---Centralized HMC Connection Phase ---
	o.logger.StartPhase("Connecting to HMC...")
	provider, err := compute.NewProviderWithState(o.cfg, o.logger, o.debug, o.stateManager)
	if err != nil {
		o.logger.EndPhase(false, "Failed to connect to HMC")
		return fmt.Errorf("failed to initialize compute provider: %w", err)
	}
	o.logger.EndPhase(true, "Connected to HMC")

	defer func() {
		if hmcProvider, ok := provider.(*compute.HMCProvider); ok {
			hmcProvider.Cleanup()
		}
	}()

	// Phase 1: Power off LPARs (MUST happen before unmapping ISO)
	phaseExec := o.startPhase("teardown_poweroff")
	o.logger.StartPhase("Powering off cluster LPARs...")
	var phaseErr error
	func() {
		// We no longer initialize the provider here!
		shieldedCtx := context.WithoutCancel(ctx)
		if err := provider.PowerOffNodes(shieldedCtx); err != nil {
			o.logger.Warn("Failed to power off some LPARs", "error", err)
			phaseErr = err
		}
	}()
	o.logger.EndPhase(phaseErr == nil, "Cluster LPARs powered off")
	o.endPhase(phaseExec, phaseErr)

	// ========================================================================
	// PHASE 2: CLEAN UP AGENT MAPPINGS
	// ========================================================================
	// HARDENED LOGIC: We must check the state file for orphaned mappings!
	// If the user changed the boot method to 'netboot' after a failed 'agent' run,
	// relying strictly on the YAML config will permanently orphan the ISO on the VIOS.
	hasOrphanedISOs := o.state != nil && len(o.state.ISOMappings) > 0

	if o.cfg.Nodes.BootMethod == "agent" || hasOrphanedISOs {
		phaseExec = o.startPhase("teardown_agent_cleanup")
		o.logger.StartPhase("Cleaning up VIOS ISO Mappings...")
		phaseErr = nil
		func() {
			// SHIELDED CONTEXT: If the user hits Ctrl+C while the VIOS is deleting the
			// 1GB payload, it will corrupt the VIOS filesystem!
			shieldedCtx := context.WithoutCancel(ctx)

			if hmcProvider, ok := provider.(*compute.HMCProvider); ok {
				if err := hmcProvider.CleanupISOMappings(shieldedCtx); err != nil {
					o.logger.Warn("Failed to clean up some Agent ISO mappings", "error", err)
					phaseErr = err
				}
			}
		}()
		o.logger.EndPhase(phaseErr == nil, "VIOS Agent ISO Mappings cleaned up")
		o.endPhase(phaseExec, phaseErr)
	}

	// ========================================================================
	// PHASE 3: CLEAN UP LOCAL SERVICES
	// ========================================================================
	phaseExec = o.startPhase("teardown_services")
	o.logger.StartPhase("Removing local network and service configurations...")
	phaseErr = nil
	func() {
		shieldedCtx := context.WithoutCancel(ctx)

		// 1. DNS/DHCP/PXE - Unconditional file deletion based on cluster name
		// Safe to run even if disabled, as it targets specific file string matches.
		dnsmasq := services.NewDNSmasqManager(o.cfg, o.daemonCfg, o.executor)
		dnsmasq.Cleanup(shieldedCtx)

		// 2. HAProxy & VIP Cleanup
		// We must check if the LB was previously managed during this deployment
		wasLBManaged := o.cfg.Services.LoadBalancer.IsManaged()
		if !wasLBManaged && o.state != nil {
			for _, svc := range o.state.ConfiguredServices {
				if svc.Name == "haproxy" && svc.Managed {
					wasLBManaged = true
					break
				}
			}
		}

		// Remove cluster HAProxy file unconditionally
		o.executor.Execute(shieldedCtx, fmt.Sprintf("sudo rm -f /etc/haproxy/conf.d/10-%s.cfg", o.cfg.OpenShift.ClusterName))
		o.executor.SystemctlRestart(shieldedCtx, "haproxy")

		if wasLBManaged && o.cfg.Services.LoadBalancer.GetVIP() != "" {
			netMgr := controller.NewNetworkManager(o.executor, o.debug, o.logger)
			vip := o.cfg.Services.LoadBalancer.GetVIP()
			iface := o.cfg.Network.ControllerInterface
			cidr := o.cfg.Network.MachineCIDR
			ctrlIP := o.cfg.Network.ControllerIP

			// SAFETY CHECK: ONLY attempt to remove the VIP alias if it's a dedicated floating IP!
			// If it matches the Controller IP, do NOT delete it, or we knock the host offline!
			if vip != ctrlIP {
				if err := netMgr.RemoveVIPAlias(shieldedCtx, iface, vip, cidr, ctrlIP); err != nil {
					o.logger.Warn("Failed to cleanly remove VIP alias via nmcli", "error", err)
					prefix := controller.ExtractCIDRPrefix(cidr)
					o.executor.Execute(shieldedCtx, fmt.Sprintf("sudo ip addr del %s/%s dev %s", vip, prefix, iface))
				}
			} else {
				o.logger.Debug("VIP matches Controller IP. Bypassing network alias teardown.")
			}
		}

		// 3. Proxy and Registry (State-aware execution)
		wasProxyManaged := o.cfg.Services.Proxy.IsManaged()
		wasRegistryManaged := (o.cfg.Network.IsolationLevel == "air-gapped" && o.cfg.Services.Registry.IsManaged())

		if o.state != nil {
			for _, svc := range o.state.ConfiguredServices {
				if svc.Name == "squid-proxy" && svc.Managed {
					wasProxyManaged = true
				}
				if svc.Name == "local-registry" && svc.Managed {
					wasRegistryManaged = true
				}
			}
		}

		if wasProxyManaged {
			squidMgr := services.NewSquidManager(o.cfg, o.executor, o.logger, o.workspaceDir)
			_ = squidMgr.Cleanup(shieldedCtx)
		}

		if wasRegistryManaged {
			registryMgr := services.NewRegistryManager(o.cfg, o.executor, o.logger, o.stateManager, o.state, o.workspaceDir)
			_ = registryMgr.Cleanup(shieldedCtx)
		}

		// 4. /etc/hosts & HTTP/NFS Cleanup
		netMgr := controller.NewNetworkManager(o.executor, o.debug, o.logger)
		_ = netMgr.RemoveHostsEntry(shieldedCtx, o.cfg.OpenShift.ClusterName)

		// HTTP directory is named after the cluster, safe to remove unconditionally
		httpServer := services.NewHTTPServerManager(o.cfg, o.daemonCfg, o.executor, o.logger)
		httpServer.Cleanup(shieldedCtx)

		// NFS cleanup - removes the cluster-named export file unconditionally
		nfsMgr := services.NewNFSManager(o.cfg, o.executor, o.logger, o.workspaceDir)
		nfsMgr.Cleanup(shieldedCtx)

	}()
	o.logger.EndPhase(phaseErr == nil, "Local network and services removed")
	o.endPhase(phaseExec, phaseErr)

	// Phase 4: Mark workspace as deleted
	phaseExec = o.startPhase("teardown_finalize")
	o.logger.StartPhase("Archiving cluster workspace...")
	phaseErr = nil
	func() {
		if err := o.stateManager.MarkDeleted(); err != nil {
			o.logger.Warn("Failed to create .deleted marker", "error", err)
			phaseErr = err
		}

		managedMarkerPath := filepath.Join(o.workspaceDir, ".managed")
		if err := os.Remove(managedMarkerPath); err != nil && !os.IsNotExist(err) {
			o.logger.Warn("Failed to remove .managed marker", "error", err)
		}

		failedMarkerPath := filepath.Join(o.workspaceDir, ".failed")
		if err := os.Remove(failedMarkerPath); err != nil && !os.IsNotExist(err) {
			o.logger.Warn("Failed to remove .failed marker", "error", err)
		}
	}()
	o.logger.EndPhase(phaseErr == nil, fmt.Sprintf("Workspace retained at %s", o.workspaceDir))
	o.endPhase(phaseExec, phaseErr)

	// Update State
	o.state.Status = "deleted"
	o.state.CurrentPhase = "deleted"
	o.state.EndTime = time.Now().Format(time.RFC3339)
	_ = o.stateManager.SaveState(o.state)

	return nil
}

// IsDeleted checks if the cluster has already been torn down
func (o *Orchestrator) IsDeleted() bool {
	return o.stateManager.IsDeleted()
}
