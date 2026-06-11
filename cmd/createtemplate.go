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
	genConfigType       string
	genBootMethod       string
	genOutputPath       string
	genDisconnected     bool
	genReleaseType      string
	genProxy            bool
	genExternalProxy    bool
	genRegistry         bool // Standalone managed registry flag
	genExternalRegistry bool
)

var generateConfigCmd = &cobra.Command{
	Use:     "create-template",
	Short:   "Create a starter config.yaml template",
	GroupID: "utils",
	Long: `Creates a starter configuration template based on topology, boot method, and network environment.

NETWORK ARCHITECTURE MATRIX:
  Standard Connected:   (No flags)
  Strict Airgap:        --disconnected (Auto-enables managed registry)
  
PROXY CONTROLS:
  Managed Squid Proxy:  --proxy
  External Corp Proxy:  --external-proxy

REGISTRY CONTROLS:
  Managed Registry:     --registry (Useful for connected caching)
  External Registry:    --external-registry`,
	RunE: runGenerateConfig,
}

func init() {
	rootCmd.AddCommand(generateConfigCmd)

	generateConfigCmd.Flags().StringVarP(&genConfigType, "type", "t", "sno", "Cluster topology: 'sno' or 'multi'")
	generateConfigCmd.Flags().StringVarP(&genBootMethod, "boot", "b", "agent", "Boot method: 'agent' or 'netboot'")
	generateConfigCmd.Flags().StringVarP(&genOutputPath, "output", "o", "config.yaml", "Path to save the generated file")
	generateConfigCmd.Flags().StringVar(&genReleaseType, "release-type", "official", "Payload type: 'official' or 'ci'")

	// Network Boundary
	generateConfigCmd.Flags().BoolVarP(&genDisconnected, "disconnected", "a", false, "Generate a template for a strictly disconnected/airgapped environment")
	
	// Proxy Toggles
	generateConfigCmd.Flags().BoolVarP(&genProxy, "proxy", "p", false, "Enable local managed Squid proxy")
	generateConfigCmd.Flags().BoolVar(&genExternalProxy, "external-proxy", false, "Use an external corporate proxy")

	// Registry Toggles
	generateConfigCmd.Flags().BoolVar(&genRegistry, "registry", false, "Enable local managed Podman registry")
	generateConfigCmd.Flags().BoolVar(&genExternalRegistry, "external-registry", false, "Use an external enterprise registry")
}

func runGenerateConfig(cmd *cobra.Command, args []string) error {
	configType := strings.ToLower(genConfigType)
	bootMethod := strings.ToLower(genBootMethod)

	if configType != "sno" && configType != "multi" {
		return fmt.Errorf("invalid config type: '%s'. Must be 'sno' or 'multi'", configType)
	}
	if bootMethod != "agent" && bootMethod != "netboot" {
		return fmt.Errorf("invalid boot method: '%s'. Must be 'agent' or 'netboot'", bootMethod)
	}

	if _, err := os.Stat(genOutputPath); err == nil {
		return fmt.Errorf("file '%s' already exists. Refusing to overwrite", genOutputPath)
	}

	// FAIL FAST: Prevent Proxy contradictions
	if genProxy && genExternalProxy {
		return fmt.Errorf("cannot specify both --proxy (managed) and --external-proxy. Choose one")
	}

	// FAIL FAST: Prevent Registry contradictions
	if genRegistry && genExternalRegistry {
		return fmt.Errorf("cannot specify both --registry (managed) and --external-registry. Choose one")
	}

	// SMART DEFAULTS: 
	// If disconnected is passed, auto-enable the managed registry UNLESS they brought their own
	manageRegistry := genRegistry || (genDisconnected && !genExternalRegistry)

	data := TemplateData{
		IsSNO:          configType == "sno",
		BootMethod:     bootMethod,
		Disconnected:   genDisconnected,
		ManageProxy:    genProxy,
		ExtProxy:       genExternalProxy,
		ManageRegistry: manageRegistry,
		ExtRegistry:    genExternalRegistry,
		ReleaseType:    genReleaseType,
	}

	tmpl, err := template.New("configGen").Parse(configTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse config template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to generate template output: %w", err)
	}

	if err := os.WriteFile(genOutputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}

	log, _ := logger.New(false, "")
	log.Info("Successfully generated cluster template", "path", genOutputPath)

	agentPath := "agent.yaml"
	if _, err := os.Stat(agentPath); os.IsNotExist(err) {
		_ = os.WriteFile(agentPath, []byte(agentConfigTemplate), 0644)
	}
	return nil
}
// TemplateData is a struct that holds the data used to generate the configuration template.
type TemplateData struct {
	IsSNO          bool
	BootMethod     string
	Disconnected   bool
	ManageProxy    bool
	ExtProxy       bool
	ManageRegistry bool
	ExtRegistry    bool
	ReleaseType    string
}

const configTemplate = `# =============================================================================
# ShiftLaunch Agent Configuration Template
# Topology: {{if .IsSNO}}SNO (Single Node OpenShift){{else}}Multi-Node Cluster{{end}}
# Boot Method: {{if eq .BootMethod "agent"}}Agent Installer{{else}}Network Boot (PXE){{end}}
# =============================================================================

# -----------------------------------------------------------------------------
# 1. MANAGED SERVICES (The "Who")
# -----------------------------------------------------------------------------
managed_services:
  dns: true
  dhcp: {{if eq .BootMethod "agent"}}false{{else}}true{{end}}
  pxe: {{if eq .BootMethod "netboot"}}true{{else}}false{{end}}
  load_balancer: true
  nfs: {{if eq .BootMethod "agent"}}true{{else}}false{{end}}

  # Network Boundary Controls
  proxy: {{if .ManageProxy}}true{{else}}false{{end}}
  registry: {{if .ManageRegistry}}true{{else}}false{{end}}

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

# -----------------------------------------------------------------------------
# 5. OPENSHIFT CLUSTER SETTINGS
# -----------------------------------------------------------------------------
openshift:
  cluster_name: "<Cluster Name>"
  version: "4.21"
  release_type: "{{.ReleaseType}}" # "official" or "ci"
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
{{- if and .ManageRegistry (eq .ReleaseType "official")}}
    ocp_mirror_client: "<URL>/oc-mirror.tar.gz"
{{- end}}

{{if .Disconnected}}
# -----------------------------------------------------------------------------
# DISCONNECTED / AIRGAP CONFIGURATION
# -----------------------------------------------------------------------------
disconnected:
  enabled: true
{{- if .ExtRegistry}}
  
  # EXTERNAL REGISTRY ENABLED: Provide your enterprise registry details below:
  registry_hostname: "harbor.mycompany.com" 
  local_repo: "openshift4/ocp4"
  # registry_ca_file: "/path/to/harbor-ca.crt" # Uncomment if using a self-signed CA
{{- else}}

  # If bringing your own registry instead of using ShiftLaunch's managed registry:
  # registry_hostname: "harbor.mycompany.com" 
  # local_repo: "openshift4/ocp4"
  # registry_ca_file: "/path/to/harbor-ca.crt"
{{- end}}
{{end}}

{{if .ExtProxy}}
# -----------------------------------------------------------------------------
# EXTERNAL PROXY CONFIGURATION
# -----------------------------------------------------------------------------
external_proxy:
  # Provide your corporate proxy details below:
  http_proxy: "http://proxy.mycompany.com:8080"
  https_proxy: "http://proxy.mycompany.com:8080"
  # Provide a comma-separated list of domains/IPs to bypass the proxy
  no_proxy: "127.0.0.1,localhost,.example.local,10.20.x.0/24"
{{end}}

# -----------------------------------------------------------------------------
# NODE TOPOLOGY (HMC Target LPARs)
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
{{end}}`

const agentConfigTemplate = `network:
  http_port: 8080

paths:
  workspace_dir: "/opt/shiftlaunch/clusters"
  dnsmasq_conf_dir: "/etc/dnsmasq.d"
  haproxy_conf_dir: "/etc/haproxy/conf.d"
  httpd_doc_root: "/var/www/html"
  tftp_root: "/var/lib/tftpboot"
  install_device: "/dev/sda"

timeouts:
  hmc_api_retries: 3
  download_timeout_sec: 1800
`

