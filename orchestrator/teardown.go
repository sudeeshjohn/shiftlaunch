package orchestrator

import (
	"context"
	"fmt"
	"time"

	compute "github.com/sudeeshjohn/shiftlaunch/infra/compute"
	"github.com/sudeeshjohn/shiftlaunch/infra/controller"
	"github.com/sudeeshjohn/shiftlaunch/services"
)

// Teardown safely powers off LPARs, removes local services, and marks workspace as deleted
func (o *Orchestrator) Teardown() error {
	// Check if already deleted
	if o.stateManager.IsDeleted() {
		o.logger.Info("⚠️ Cluster is already marked as deleted. Skipping teardown.", "cluster", o.cfg.OpenShift.ClusterName)
		return nil
	}

	// Acquire lock to prevent concurrent operations
	if err := o.stateManager.AcquireLock(); err != nil {
		return fmt.Errorf("failed to acquire cluster lock: %w", err)
	}
	// Ensure lock is always released, even on error
	defer func() {
		if err := o.stateManager.ReleaseLock(); err != nil {
			o.logger.Warn("Failed to release lock", "error", err)
		}
	}()

	o.logger.Info("🛑 Initiating Soft Teardown", "cluster", o.cfg.OpenShift.ClusterName)

	// 1. Power off LPARs via HMC (DO NOT DELETE LPARs - BYOI Rule)
	provider, err := compute.NewProvider(o.cfg, o.logger, o.debug)
	if err == nil {
		defer func() {
			if hmcProvider, ok := provider.(*compute.HMCProvider); ok {
				hmcProvider.Cleanup()
			}
		}()
		
		o.logger.Info("Powering off LPARs...")
		if err := provider.PowerOffNodes(context.Background()); err != nil {
			o.logger.Warn("Failed to power off some LPARs", "error", err)
		}
	}

	// 2. Clean up local services
	o.logger.Info("Cleaning up local network configurations...")

	if o.cfg.ManagedServices.DNS || o.cfg.ManagedServices.DHCP || o.cfg.ManagedServices.PXE {
		dnsmasq := services.NewDNSmasqManager(o.cfg, o.executor)
		dnsmasq.Cleanup()
	}

	if o.cfg.ManagedServices.LoadBalancer {
		o.executor.Execute(fmt.Sprintf("sudo rm -f /etc/haproxy/conf.d/10-%s.cfg", o.cfg.OpenShift.ClusterName))
		o.executor.SystemctlRestart("haproxy")

		o.logger.Info("Removing VIP alias from controller network interface...")
		
		// Use the NetworkManager to cleanly prune the IP from the connection profile
		netMgr := controller.NewNetworkManager(o.executor, o.debug, o.logger)
		vip := o.cfg.Network.LoadBalancerIP
		iface := o.cfg.Controller.NetworkInterface
		cidr := o.cfg.Network.MachineCIDR
		ctrlIP := o.cfg.Controller.IP

		if err := netMgr.RemoveVIPAlias(iface, vip, cidr, ctrlIP); err != nil {
			o.logger.Warn("Failed to cleanly remove VIP alias via nmcli", "error", err)
			
			// Fallback: Force remove it from the live interface using exact CIDR prefix
			prefix := controller.ExtractCIDRPrefix(cidr)
			o.executor.Execute(fmt.Sprintf("sudo ip addr del %s/%s dev %s", vip, prefix, iface))
		}
	}
	// 3. Clean up HTTP Server
	httpServer := services.NewHTTPServerManager(o.cfg, o.executor, o.logger)
	httpServer.Cleanup()

	// 4. Mark the workspace as deleted instead of wiping it
	o.logger.Info("Archiving local cluster workspace...")
	if err := o.stateManager.MarkDeleted(); err != nil {
		o.logger.Warn("Failed to create .deleted marker", "error", err)
	}

	// Update State
	o.state.Status = "deleted"
	o.state.CurrentPhase = "deleted"
	o.state.EndTime = time.Now().Format(time.RFC3339)
	_ = o.stateManager.SaveState(o.state)

	o.logger.Info(fmt.Sprintf("✅ Local services stopped. Workspace retained at %s", o.workspaceDir))
	return nil
}

// IsDeleted checks if the cluster has already been torn down
func (o *Orchestrator) IsDeleted() bool {
	return o.stateManager.IsDeleted()
}