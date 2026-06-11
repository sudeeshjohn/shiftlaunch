package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
	"github.ibm.com/sudeeshjohn/shiftlaunch/logger"
)

var (
	genConfigType   string
	genBootMethod   string
	genOutputPath   string
	genDisconnected bool
	genProxy        bool
	genReleaseType  string
)

var generateConfigCmd = &cobra.Command{
	Use:     "create-template",
	Short:   "Create a starter config.yaml template",
	GroupID: "utils",
	Long: `Creates a starter configuration template based on topology, boot method, and network environment.
The create-template command creates:
- A cluster configuration file (config.yaml)
- An agent daemon configuration file (agent.yaml) if it doesn't exist

Supported network environments:
- Standard Connected: No extra flags
- Corporate Proxy:    --proxy
- Strict Airgap:      --disconnected
- Soft Airgap:        --disconnected --proxy`,
	RunE: runGenerateConfig,
}

func init() {
	rootCmd.AddCommand(generateConfigCmd)

	generateConfigCmd.Flags().StringVarP(&genConfigType, "type", "t", "sno", "Cluster topology: 'sno' or 'multi'")
	generateConfigCmd.Flags().StringVarP(&genBootMethod, "boot", "b", "agent", "Boot method: 'agent' or 'netboot'")
	generateConfigCmd.Flags().StringVarP(&genOutputPath, "output", "o", "config.yaml", "Path to save the generated file")
	
	// Network Architecture Flags
	generateConfigCmd.Flags().BoolVarP(&genDisconnected, "disconnected", "a", false, "Generate a template for a disconnected/airgapped environment")
	generateConfigCmd.Flags().BoolVarP(&genProxy, "proxy", "p", false, "Enable local or corporate proxy management in the template")
  generateConfigCmd.Flags().StringVarP(&genReleaseType, "release-type", "r", "official", "Payload type: 'official' or 'ci'")
}

func runGenerateConfig(cmd *cobra.Command, args []string) error {
	return GenerateConfig(genConfigType, genBootMethod, genOutputPath, genDisconnected, genProxy)
}

const configTemplate = `# =============================================================================
# ShiftLaunch Agent Configuration Template
# Topology: {{if .IsSNO}}SNO (Single Node OpenShift){{else}}Multi-Node Cluster{{end}}
# Boot Method: {{if eq .BootMethod "agent"}}Agent Installer{{else}}Network Boot (PXE){{end}}
# Environment: {{if and .Disconnected .UseProxy}}Soft Airgap (Disconnected + Proxy){{else if .Disconnected}}Strict Airgap (Disconnected, No Outbound){{else if .UseProxy}}Connected with Proxy{{else}}Standard Connected{{end}}
# =============================================================================

# -----------------------------------------------------------------------------
# 1. MANAGED SERVICES (The "Who")
# Tell the Agent which services to install and manage locally on this machine.
# -----------------------------------------------------------------------------
managed_services:
  # Setup local dnsmasq to answer for the cluster domain (api, *.apps, etc.)
  dns: true
  
  # Setup local DHCP to assign static IPs to LPARs based on MAC addresses
  dhcp: {{if eq .BootMethod "agent"}}false{{else}}true{{end}}
  
  # Setup local TFTP server (Required for Netboot, ignored for Agent boot)
  pxe: {{if eq .BootMethod "netboot"}}true{{else}}false{{end}}
  
  # Setup local HAProxy to route traffic for the loadbalancer_ip (VIP)
  load_balancer: true
  
  # Setup local NFS server to host Agent images to the VIOS (Required for Agent boot)
  nfs: {{if eq .BootMethod "agent"}}true{{else}}false{{end}}

  # Setup local Squid proxy for controlled outbound or internet routing
  proxy: {{if .UseProxy}}true{{else}}false{{end}}
  
  # Setup local Podman container registry for airgapped mirroring
  registry: {{if .Disconnected}}true{{else}}false{{end}}

# -----------------------------------------------------------------------------
# 2. CONTROLLER NODE (The "Where")
# -----------------------------------------------------------------------------
controller:
  network_interface: "eth0"

# -----------------------------------------------------------------------------
# 3. HMC CREDENTIALS
# -----------------------------------------------------------------------------
hmc:
  ip: "10.20.x.x"
  username: "YOUR_HMC_USERNAME"
  password: "password"

# -----------------------------------------------------------------------------
# 4. NETWORK CONFIGURATION
# -----------------------------------------------------------------------------
network:
  loadbalancer_ip: "10.20.x.y"
  machine_network_cidr: "10.20.x.0/24" 
  gateway: "10.20.x.1"
  nameserver: ""
  dns_forwarders:
    - "198.51.100.1"
    - "198.51.100.2"

# -----------------------------------------------------------------------------
# 5. OPENSHIFT CLUSTER SETTINGS
# -----------------------------------------------------------------------------
openshift:
  cluster_name: "<Cluster Name>"
  version: "4.21"
  release_type: "{{.ReleaseType}}" # "official" (oc-mirror v2 IDMS) or "ci" (oc adm flat mirror)
  base_domain: "example.local"
  cluster_network_cidr: "10.128.0.0/14"
  cluster_network_host_prefix: 23
  service_network: "172.30.0.0/16"
  pull_secret_file: "./pull-secret.json"
  ssh_public_key_file: "~/.ssh/id_rsa.pub"
{{if eq .BootMethod "netboot"}}
  rhcos_images:
    kernel_url: "<URL>/rhcos-live-kernel.ppc64le"
    initramfs_url: "<URL>/rhcos-live-initramfs.ppc64le.img"
    rootfs_url: "<URL>/rhcos-live-rootfs.ppc64le.img"
{{end}}
  ocp_client_config:
    ocp_client: "<URL>/openshift-client-linux.tar.gz"
    ocp_installer: "<URL>/openshift-install-linux.tar.gz"

{{if .Disconnected}}
# -----------------------------------------------------------------------------
# 6. DISCONNECTED / AIRGAP CONFIGURATION
# -----------------------------------------------------------------------------
disconnected:
  enabled: true
  
  # If bringing your own registry instead of using ShiftLaunch's managed registry:
  # registry_hostname: "harbor.mycompany.com" 
  # local_repo: "openshift4/ocp4"
  # registry_ca_file: "/path/to/harbor-ca.crt"
{{end}}

# -----------------------------------------------------------------------------
# {{if .Disconnected}}7{{else}}6{{end}}. NODE TOPOLOGY (HMC Target LPARs)
# -----------------------------------------------------------------------------
nodes:
  boot_method: "{{.BootMethod}}"

{{if .IsSNO}}
  sno:
    - name: "sno-0"
      ip: "10.20.x.10"
      existing_lpar_name: "<SNO-LPARNAME>"
      system_name: "SYSTEM-NAME"
{{else}}
{{- if eq .BootMethod "netboot"}}
  bootstrap:
    - name: "bootstrap"
      ip: "10.20.x.11"
      existing_lpar_name: "<BOOTSTRAP-LPARNAME>"
      system_name: "SYSTEM-NAME"
{{- end}}
  masters:
    - name: "master-0"
      ip: "10.20.x.12"
      existing_lpar_name: "<MASTER0-LPARNAME>"
      system_name: "SYSTEM-NAME"
    - name: "master-1"
      ip: "10.20.x.13"
      existing_lpar_name: "<MASTER1-LPARNAME>"
      system_name: "SYSTEM-NAME"
    - name: "master-2"
      ip: "10.20.x.14"
      existing_lpar_name: "<MASTER2-LPARNAME>"
      system_name: "SYSTEM-NAME"

  workers:
    - name: "worker-0"
      ip: "10.20.x.15"
      existing_lpar_name: "<WORKER0-LPARNAME>"
      system_name: "SYSTEM-NAME"
    - name: "worker-1"
      ip: "10.20.x.16"
      existing_lpar_name: "<WORKER1-LPARNAME>"
      system_name: "SYSTEM-NAME"
{{end}}`

const agentConfigTemplate = `network:\n  http_port: 8080\n\npaths:\n  workspace_dir: \"/opt/shiftlaunch/clusters\"\n  dnsmasq_conf_dir: \"/etc/dnsmasq.d\"\n  haproxy_conf_dir: \"/etc/haproxy/conf.d\"\n  httpd_doc_root: \"/var/www/html\"\n  tftp_root: \"/var/lib/tftpboot\"\n  install_device: \"/dev/sda\"\n\ntimeouts:\n  hmc_api_retries: 3\n  download_timeout_sec: 1800\n`
//TemplateData struct is used to pass data to the template
type TemplateData struct {
	IsSNO        bool
	BootMethod   string
	Disconnected bool
	UseProxy     bool
	ReleaseType  string
}
// GenerateConfig generates the config file for the agent
func GenerateConfig(configType, bootMethod, outputPath string, disconnected, proxy bool) error {
	configType = strings.ToLower(configType)
	bootMethod = strings.ToLower(bootMethod)

	if configType != "sno" && configType != "multi" {
		return fmt.Errorf("invalid config type: '%s'. Must be 'sno' or 'multi'", configType)
	}
	if bootMethod != "agent" && bootMethod != "netboot" {
		return fmt.Errorf("invalid boot method: '%s'. Must be 'agent' or 'netboot'", bootMethod)
	}

	if _, err := os.Stat(outputPath); err == nil {
		return fmt.Errorf("file '%s' already exists. Refusing to overwrite", outputPath)
	}

	data := TemplateData{
		IsSNO:        configType == "sno",
		BootMethod:   bootMethod,
		Disconnected: disconnected,
		UseProxy:     proxy,
		ReleaseType:  genReleaseType,
	}

	tmpl, err := template.New("configGen").Parse(configTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse config template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to generate template output: %w", err)
	}

	if err := os.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}

	log, _ := logger.New(false, "")
	log.Info("Successfully generated cluster template", 
		"topology", configType, 
		"boot_method", bootMethod, 
		"disconnected", disconnected, 
		"proxy", proxy, 
		"path", outputPath)

	agentPath := "agent.yaml"
	if _, err := os.Stat(agentPath); os.IsNotExist(err) {
		_ = os.WriteFile(agentPath, []byte(agentConfigTemplate), 0644)
	}

	return nil
}