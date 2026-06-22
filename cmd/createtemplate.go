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
	genRegistry         bool
	genExternalRegistry bool
)

var generateConfigCmd = &cobra.Command{
	Use:     "generate-config",
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

	if genProxy && genExternalProxy {
		return fmt.Errorf("cannot specify both --proxy (managed) and --external-proxy. Choose one")
	}
	if genRegistry && genExternalRegistry {
		return fmt.Errorf("cannot specify both --registry (managed) and --external-registry. Choose one")
	}

	isolationMode := "connected"
	if genDisconnected {
		isolationMode = "fully-disconnected"
	} else if genProxy || genExternalProxy {
		isolationMode = "soft-disconnected"
	}

	data := TemplateData{
		IsSNO:          configType == "sno",
		BootMethod:     bootMethod,
		IsolationLevel: isolationMode,
		ManageProxy:    genProxy,
		ExtProxy:       genExternalProxy,
		ManageRegistry: genRegistry || (genDisconnected && !genExternalRegistry),
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

	return nil
}

type TemplateData struct {
	IsSNO          bool
	BootMethod     string
	IsolationLevel string
	ManageProxy    bool
	ExtProxy       bool
	ManageRegistry bool
	ExtRegistry    bool
	ReleaseType    string
}

const configTemplate = `# =============================================================================
# ShiftLaunch Configuration Reference
# Topology: {{if .IsSNO}}SNO (Single Node OpenShift){{else}}Multi-Node Cluster{{end}}
# Boot Method: {{if eq .BootMethod "agent"}}Agent Installer{{else}}Network Boot (PXE){{end}}
# =============================================================================

# -----------------------------------------------------------------------------
# 1. INFRASTRUCTURE SERVICES
# Define who manages each network service locally on the controller node.
# -----------------------------------------------------------------------------
services:
  dns:
    enabled: true
    external_nameserver: "192.168.100.5" # Used if enabled: false
    dns_forwarders:                # Used if enabled: true
      - "8.8.8.8"
      - "1.1.1.1"

  dhcp:
    enabled: {{if eq .BootMethod "agent"}}false{{else}}true{{end}}                       # Required for netboot. Ignored for agent boot.
    external_dhcp_server: "192.168.100.1"      
  pxe:
    enabled: {{if eq .BootMethod "netboot"}}true{{else}}false{{end}}                       # Required for netboot. Ignored for agent boot.
    external_pxe_server: "192.168.100.1"       
  load_balancer:
    enabled: true                       
    vip: "192.168.100.50"                # Required entry-point IP for the cluster API / Ingress.
  nfs:
    enabled: {{if eq .BootMethod "agent"}}true{{else}}false{{end}}                       # Required for agent boot to transfer the ISO to the VIOS.
    external_nfs_server: "192.168.100.1"       

  # --- BOUNDARY SERVICES ---
  proxy:
    enabled: {{if .ManageProxy}}true{{else}}false{{end}}                       # true = ShiftLaunch builds a local Squid proxy.
    external_http_proxy: {{if .ExtProxy}}"http://proxy.mycompany.com:8080"{{else}}""{{end}}
    external_https_proxy: {{if .ExtProxy}}"http://proxy.mycompany.com:8080"{{else}}""{{end}}
    no_proxy: "127.0.0.1,localhost,.example.local,192.168.100.0/24"

  registry:
    enabled: {{if .ManageRegistry}}true{{else}}false{{end}}                       # true = ShiftLaunch builds a local Podman registry.
    external_reqistry_server: {{if .ExtRegistry}}"registry.mycompany.com"{{else}}""{{end}} 
    username: "admin"                    
    password: "admin"                    
    ca_cert_file: "/path/ca.crt"         

# -----------------------------------------------------------------------------
# 2. CORE NETWORK & ARCHITECTURE MODE
# -----------------------------------------------------------------------------
network:
  # ARCHITECTURE TARGETS:
  #   "connected"         -> Nodes have direct outbound internet access. (Ignores proxy & registry fields)
  #   "soft-disconnected" -> Nodes are isolated but can access the internet EXCLUSIVELY via a Proxy gateway.
  #   "fully-disconnected"-> Nodes are strictly airgapped with ZERO internet path. Enforces local/external Registry data.
  isolation_level: "{{.IsolationLevel}}"
  
  controller_interface: "eth0"           # Physical interface to bind the VIP and local services to.
  machine_network_cidr: "192.168.100.0/24" # The primary subnet the OpenShift nodes reside on.
  gateway: "192.168.100.1"               # The default gateway for the cluster nodes.

# -----------------------------------------------------------------------------
# 3. OPENSHIFT CLUSTER SETTINGS
# -----------------------------------------------------------------------------
openshift:
  cluster_name: "my-cluster"
  version: "4.21.18"
  release_type: "{{.ReleaseType}}"               
  base_domain: "example.local"
  pull_secret_file: "./pull-secret.json"
  ssh_public_key_file: "~/.ssh/id_rsa.pub"
  force_ocp_download: false              

# -----------------------------------------------------------------------------
# 4. HMC CREDENTIALS
# -----------------------------------------------------------------------------
hmc:
  ip: "192.168.100.10"
  username: "hscroot"
  password: "YOUR_HMC_PASSWORD"

# -----------------------------------------------------------------------------
# 5. NODE TOPOLOGY (HMC Target LPARs)
# -----------------------------------------------------------------------------
nodes:
  boot_method: "{{.BootMethod}}"
{{if .IsSNO}}
  masters:
    - name: "sno-0"
      ip: "192.168.100.12"
      existing_lpar_name: "my-sno-lpar"
      system_name: "SYSTEM-NAME"
{{else}}
{{- if eq .BootMethod "netboot"}}
  bootstrap:
    - name: "bootstrap"
      ip: "192.168.100.11"
      existing_lpar_name: "my-bootstrap-lpar"
      system_name: "SYSTEM-NAME"
{{- end}}
  masters:
    - name: "master-0"
      ip: "192.168.100.12"
      existing_lpar_name: "my-master-0"
      system_name: "SYSTEM-NAME"
    - name: "master-1"
      ip: "192.168.100.13"
      existing_lpar_name: "my-master-1"
      system_name: "SYSTEM-NAME"
    - name: "master-2"
      ip: "192.168.100.14"
      existing_lpar_name: "my-master-2"
      system_name: "SYSTEM-NAME"

  workers:
    - name: "worker-0"
      ip: "192.168.100.15"
      existing_lpar_name: "my-worker-0"
      system_name: "SYSTEM-NAME"
    - name: "worker-1"
      ip: "192.168.100.16"
      existing_lpar_name: "my-worker-1"
      system_name: "SYSTEM-NAME"
{{end}}`

// Made with Bob
