package services

import (
	"fmt"
	"strings"

	"github.com/sudeeshjohn/shiftlaunch/localexec"
	"github.com/sudeeshjohn/shiftlaunch/logger"
	"github.com/sudeeshjohn/shiftlaunch/types"
)

// ControllerSetup manages the local packages and firewalls on the machine running the agent
type ControllerSetup struct {
	cfg      *types.AgentConfig
	executor *localexec.LocalClient
	logger   *logger.Logger
}

func NewControllerSetup(cfg *types.AgentConfig, executor *localexec.LocalClient, log *logger.Logger) *ControllerSetup {
	return &ControllerSetup{
		cfg:      cfg,
		executor: executor,
		logger:   log,
	}
}

// getRequiredPackages figures out what dnf packages we need based on the YAML toggles
func (c *ControllerSetup) getRequiredPackages() []string {
	var pkgs []string
	
	// We always need httpd for Ignition/ISO hosting, and firewalld for port management
	pkgs = append(pkgs, "httpd", "firewalld")

	if c.cfg.ManagedServices.DNS || c.cfg.ManagedServices.DHCP || c.cfg.ManagedServices.PXE {
		pkgs = append(pkgs, "dnsmasq")
	}
	if c.cfg.ManagedServices.PXE {
		pkgs = append(pkgs, "tftp-server", "syslinux-tftpboot")
	}
	if c.cfg.ManagedServices.LoadBalancer {
		pkgs = append(pkgs, "haproxy")
	}

	return pkgs
}

// InstallPackages uses localexec to run dnf install
func (c *ControllerSetup) InstallPackages() error {
	pkgs := c.getRequiredPackages()
	c.logger.Info("Installing required local packages...", "packages", strings.Join(pkgs, ", "))

	installCmd := fmt.Sprintf("sudo dnf install -y %s", strings.Join(pkgs, " "))
	if _, err := c.executor.Execute(installCmd); err != nil {
		return fmt.Errorf("failed to install local packages: %w", err)
	}

	c.logger.Info("✓ Packages installed successfully")
	return nil
}

// ConfigureFirewall opens the required ports locally based on the YAML toggles
func (c *ControllerSetup) ConfigureFirewall() error {
	c.logger.Info("Configuring local firewall...")

	// 1. Ensure firewalld is running
	if _, err := c.executor.Execute("sudo systemctl enable --now firewalld"); err != nil {
		return fmt.Errorf("failed to start firewalld: %w", err)
	}

	var ports []string
	
	// HTTP for Ignition is always required on port 8080 (to avoid HAProxy collision on 80)
	ports = append(ports, "8080/tcp")

	if c.cfg.ManagedServices.DNS {
		ports = append(ports, "53/tcp", "53/udp")
	}
	if c.cfg.ManagedServices.DHCP {
		ports = append(ports, "67/udp")
	}
	if c.cfg.ManagedServices.PXE {
		ports = append(ports, "69/udp")
	}
	if c.cfg.ManagedServices.LoadBalancer {
		ports = append(ports, "6443/tcp", "22623/tcp", "80/tcp", "443/tcp")
	}

	// Apply Ports
	portArgs := ""
	for _, port := range ports {
		portArgs += fmt.Sprintf(" --add-port=%s", port)
	}
	
	if _, err := c.executor.Execute("sudo firewall-cmd --permanent" + portArgs); err != nil {
		return fmt.Errorf("failed to add firewall ports: %w", err)
	}

	// Reload
	if _, err := c.executor.Execute("sudo firewall-cmd --reload"); err != nil {
		return fmt.Errorf("failed to reload firewall: %w", err)
	}

	c.logger.Info("✓ Local firewall configured successfully")
	return nil
}