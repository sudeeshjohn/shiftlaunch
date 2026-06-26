package services

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/IBM/shiftlaunch/localexec"
	"github.com/IBM/shiftlaunch/types"
)

const installConfigTemplate = `apiVersion: v1
baseDomain: {{.BaseDomain}}
compute:
{{- if .IsSNO}}
- name: worker
  replicas: 0
{{- else}}
- hyperthreading: Enabled
  name: worker
  replicas: {{.WorkerReplicas}}
  architecture: ppc64le
{{- end}}
controlPlane:
{{- if .IsSNO}}
  name: master
  replicas: 1
{{- else}}
  hyperthreading: Enabled
  name: master
  replicas: {{.MasterReplicas}}
  architecture: ppc64le
{{- end}}
metadata:
  name: {{.ClusterName}}
networking:
  clusterNetwork:
  - cidr: {{.ClusterNetworkCIDR}}
    hostPrefix: {{.ClusterNetworkHostPrefix}}
  machineNetwork:
  - cidr: {{.MachineNetwork}}
  networkType: OVNKubernetes
  serviceNetwork:
  - {{.ServiceNetwork}}
platform:
  none: {}
{{- if .IsSNO}}
bootstrapInPlace:
  installationDisk: {{.DiskDevice}}
{{- else}}
fips: false
{{- end}}
{{- if .UseLocalRegistry}}
pullSecret: '{{.PullSecretUpdated}}'
{{- if .RegistryCert}}
additionalTrustBundle: |
{{.RegistryCert}}
{{- end}}
imageDigestSources:
{{- if eq .ReleaseType "ci"}}
- mirrors:
  - {{.LocalRegistry}}/{{.LocalRepo}}
  source: quay.io/openshift-release-dev/ocp-release
- mirrors:
  - {{.LocalRegistry}}/{{.LocalRepo}}
  source: quay.io/openshift-release-dev/ocp-v4.0-art-dev
{{- else}}
- mirrors:
  - {{.LocalRegistry}}/{{.LocalRepo}}/openshift/release-images
  - {{.LocalRegistry}}/{{.LocalRepo}}
  source: quay.io/openshift-release-dev/ocp-release
- mirrors:
  - {{.LocalRegistry}}/{{.LocalRepo}}/openshift/release
  - {{.LocalRegistry}}/{{.LocalRepo}}
  source: quay.io/openshift-release-dev/ocp-v4.0-art-dev
{{- end}}
{{- else}}
pullSecret: '{{.PullSecret}}'
{{- end}}
sshKey: '{{.SSHKey}}'
{{- if .UseProxy}}
proxy:
  httpProxy: {{.ProxyURL}}
  httpsProxy: {{.ProxyURL}}
  noProxy: .{{.ClusterName}}.{{.BaseDomain}},{{.NoProxy}}
{{- end}}
`

// agentConfigTemplate works for both SNO and multi-node clusters
// Matches the official OpenShift Agent-based Installer format
const agentConfigTemplate = `apiVersion: v1alpha1
kind: AgentConfig
metadata:
  name: {{.ClusterName}}
rendezvousIP: {{.RendezvousIP}}
{{- if .Hosts}}
hosts:
{{- range .Hosts}}
  - hostname: {{.Hostname}}
    role: {{.Role}}
    interfaces:
      - name: env2
        macAddress: {{.MACAddress}}
    networkConfig:
      interfaces:
        - name: env2
          type: ethernet
          state: up
          mtu: 1450
          ipv4:
            enabled: true
            dhcp: false
            address:
              - ip: {{.IP}}
                prefix-length: {{.PrefixLength}}
          ipv6:
            enabled: false
      dns-resolver:
        config:
          server:
            - {{.DNS}}
      routes:
        config:
          - destination: 0.0.0.0/0
            next-hop-address: {{.Gateway}}
            next-hop-interface: env2
            table-id: 254
{{- end}}
{{- end}}
`

// GenerateIgnition creates OpenShift ignition configs and installation artifacts for cluster bootstrap
func GenerateIgnition(ctx context.Context, cfg *types.AgentConfig, exec *localexec.LocalClient, workspaceDir string) error {
	// Branch based on boot method
	if cfg.Nodes.BootMethod == "agent" {
		return generateAgentISO(ctx, cfg, exec, workspaceDir)
	}

	// Existing netboot ignition generation
	return generateNetbootIgnition(ctx, cfg, exec, workspaceDir)
}

// generateNetbootIgnition handles traditional netboot ignition generation
func generateNetbootIgnition(ctx context.Context, cfg *types.AgentConfig, exec *localexec.LocalClient, workspaceDir string) error {
	targetDir := filepath.Join(workspaceDir, "install-dir")
	exec.Execute(ctx, fmt.Sprintf("mkdir -p %s", targetDir))

	// 1. Generate install-config.yaml
	installConfig, err := generateInstallConfigYAML(cfg, workspaceDir)
	if err != nil {
		return err
	}

	configPath := filepath.Join(targetDir, "install-config.yaml")
	if err := os.WriteFile(configPath, []byte(installConfig), 0644); err != nil {
		return fmt.Errorf("failed to write install-config.yaml: %w", err)
	}

	exec.Execute(ctx, fmt.Sprintf("cp %s %s.bak", configPath, configPath))
	installerPath := filepath.Join(workspaceDir, "tools", "openshift-install")

	// 2. Generate manifests first so we can inject custom MachineConfigs
	manifestsCmd := fmt.Sprintf("cd %s && %s create manifests --dir=.", targetDir, installerPath)
	if _, err := exec.Execute(ctx, manifestsCmd); err != nil {
		return fmt.Errorf("failed to create manifests: %w", err)
	}

	// ========================================================================
	// CI / NIGHTLY INJECTION (Connected OR Disconnected)
	// Inject the Insecure Policy to bypass Signature Validation for raw CI builds
	// ========================================================================
	if cfg.OpenShift.ReleaseType == "ci" {
		if err := injectInsecurePolicy(targetDir); err != nil {
			return fmt.Errorf("failed to inject insecure policy: %w", err)
		}
	}

	// 4. Create Ignition Configs
	var cmd string
	if cfg.IsSNO() {
		cmd = fmt.Sprintf("cd %s && %s create single-node-ignition-config --dir=.", targetDir, installerPath)
	} else {
		cmd = fmt.Sprintf("cd %s && %s create ignition-configs --dir=.", targetDir, installerPath)
	}

	if _, err := exec.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to create ignition configs: %w", err)
	}

	return nil
}

func generateInstallConfigYAML(cfg *types.AgentConfig, workspaceDir string) (string, error) {
	tmpl, err := template.New("installConfig").Parse(installConfigTemplate)
	if err != nil {
		return "", err
	}

	// 1. Safely read and validate the Pull Secret
	pullSecretPath := os.ExpandEnv(strings.ReplaceAll(cfg.OpenShift.PullSecretFile, "~", "$HOME"))
	pullSecret, err := os.ReadFile(pullSecretPath)
	if err != nil {
		return "", fmt.Errorf("FATAL: failed to read pull secret file at '%s': %w", pullSecretPath, err)
	}
	if len(strings.TrimSpace(string(pullSecret))) == 0 {
		return "", fmt.Errorf("FATAL: pull secret file at '%s' is empty", pullSecretPath)
	}

	// 2. Safely read and validate the SSH Public Key
	sshKeyPath := os.ExpandEnv(strings.ReplaceAll(cfg.OpenShift.SSHPublicKeyFile, "~", "$HOME"))
	sshKey, err := os.ReadFile(sshKeyPath)
	if err != nil {
		return "", fmt.Errorf("FATAL: failed to read SSH public key file at '%s': %w", sshKeyPath, err)
	}
	if len(strings.TrimSpace(string(sshKey))) == 0 {
		return "", fmt.Errorf("FATAL: SSH public key file at '%s' is empty", sshKeyPath)
	}

	// ---  Dynamic Worker Replica Assignment based on Boot Method ---
	workerReplicas := 0
	if cfg.Nodes.BootMethod == "agent" {
		workerReplicas = len(cfg.Nodes.Workers)
	}

	// Prepare data structure for template
	data := struct {
		BaseDomain               string
		WorkerReplicas           int
		MasterReplicas           int
		ClusterName              string
		ClusterNetworkCIDR       string
		ClusterNetworkHostPrefix int
		ServiceNetwork           string
		MachineNetwork           string
		IsSNO                    bool
		DiskDevice               string
		PullSecret               string
		PullSecretUpdated        string
		SSHKey                   string
		UseLocalRegistry         bool
		LocalRegistry            string
		LocalRepo                string
		RegistryCert             string
		ReleaseType              string
		UseProxy                 bool
		ProxyURL                 string
		NoProxy                  string
	}{
		BaseDomain:               cfg.OpenShift.BaseDomain,
		WorkerReplicas:           workerReplicas, // <--- Map the dynamic calculation variable here
		MasterReplicas:           len(cfg.Nodes.Masters),
		ClusterName:              cfg.OpenShift.ClusterName,
		ClusterNetworkCIDR:       cfg.OpenShift.ClusterNetworkCIDR,
		ClusterNetworkHostPrefix: cfg.OpenShift.HostPrefix,
		ServiceNetwork:           cfg.OpenShift.ServiceNetwork,
		MachineNetwork:           cfg.Network.MachineCIDR,
		IsSNO:                    cfg.IsSNO(),
		DiskDevice:               "/dev/sda",
		PullSecret:               strings.TrimSpace(string(pullSecret)),
		SSHKey:                   strings.TrimSpace(string(sshKey)),
		UseLocalRegistry:         cfg.Network.IsolationLevel == "air-gapped",
		ReleaseType:              cfg.OpenShift.ReleaseType,
		UseProxy:                 cfg.Services.Proxy.IsManaged(),
	}

	// Add disconnected registry configuration if enabled
	if data.UseLocalRegistry {
		registryHostname := cfg.Services.Registry.GetExternal()
		if cfg.Services.Registry.IsManaged() {
			registryHostname = cfg.Network.ControllerIP
		} else if registryHostname == "" {
			registryHostname = fmt.Sprintf("registry.%s.%s", cfg.OpenShift.ClusterName, cfg.OpenShift.BaseDomain)
		}

		data.LocalRegistry = fmt.Sprintf("%s:5000", registryHostname)
		if !cfg.Services.Registry.IsManaged() && strings.Contains(registryHostname, ":") {
			data.LocalRegistry = registryHostname
		}

		data.LocalRepo = cfg.Services.Registry.LocalRepo

		if cfg.Services.Registry.IsManaged() {
			updatedSecretPath := filepath.Join(workspaceDir, "pull-secret-updated.json")
			if updatedSecret, err := os.ReadFile(updatedSecretPath); err == nil {
				data.PullSecretUpdated = strings.TrimSpace(string(updatedSecret))
			} else {
				data.PullSecretUpdated = data.PullSecret
			}

			certPath := "/opt/registry/certs/domain.crt"
			if certData, err := os.ReadFile(certPath); err == nil {
				lines := strings.Split(string(certData), "\n")
				var indented []string
				for _, line := range lines {
					if line != "" {
						indented = append(indented, "  "+line)
					}
				}
				data.RegistryCert = strings.Join(indented, "\n")
			}
		} else {
			data.PullSecretUpdated = data.PullSecret

			if cfg.Services.Registry.GetCACert() != "" {
				caPath := os.ExpandEnv(strings.ReplaceAll(cfg.Services.Registry.GetCACert(), "~", "$HOME"))
				certData, err := os.ReadFile(caPath)
				if err != nil {
					return "", fmt.Errorf("failed to read external registry CA file at '%s': %w", caPath, err)
				}

				lines := strings.Split(strings.TrimSpace(string(certData)), "\n")
				var indented []string
				for _, line := range lines {
					indented = append(indented, "  "+line)
				}
				data.RegistryCert = strings.Join(indented, "\n")
			}
		}
	}

	// Add proxy configuration if enabled
	if cfg.Services.Proxy.IsManaged() || cfg.Services.Proxy.GetHTTP() != "" {
		data.UseProxy = true
		if cfg.Services.Proxy.IsManaged() {
			data.ProxyURL = fmt.Sprintf("http://%s:3128", cfg.Network.ControllerIP)
			data.NoProxy = fmt.Sprintf("127.0.0.1,localhost,%s,%s,.%s,%s",
				cfg.Network.MachineCIDR,
				cfg.Network.ControllerIP,
				cfg.OpenShift.BaseDomain,
				cfg.Services.LoadBalancer.GetVIP())
		} else {
			data.ProxyURL = cfg.Services.Proxy.GetHTTP()
			data.NoProxy = cfg.Services.Proxy.GetNoProxy()
		}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// generateAgentImage creates an Agent-based Installer image for both SNO and multi-node clusters
func generateAgentISO(ctx context.Context, cfg *types.AgentConfig, exec *localexec.LocalClient, workspaceDir string) error {
	targetDir := filepath.Join(workspaceDir, "install-dir")
	exec.Execute(ctx, fmt.Sprintf("mkdir -p %s", targetDir))

	// 1. Generate install-config.yaml
	installConfig, err := generateInstallConfigYAML(cfg, workspaceDir)
	if err != nil {
		return err
	}

	configPath := filepath.Join(targetDir, "install-config.yaml")
	if err := os.WriteFile(configPath, []byte(installConfig), 0644); err != nil {
		return fmt.Errorf("failed to write install-config.yaml: %w", err)
	}

	exec.Execute(ctx, fmt.Sprintf("cp %s %s.bak", configPath, configPath))

	// 2. Generate agent-config.yaml
	agentConfig, err := generateAgentConfigYAML(cfg)
	if err != nil {
		return err
	}

	agentConfigPath := filepath.Join(targetDir, "agent-config.yaml")
	if err := os.WriteFile(agentConfigPath, []byte(agentConfig), 0644); err != nil {
		return fmt.Errorf("failed to write agent-config.yaml: %w", err)
	}
	exec.Execute(ctx, fmt.Sprintf("cp %s %s.bak", agentConfigPath, agentConfigPath))

	// ========================================================================
	// PRODUCTION DISCONNECTED INJECTION (The Secret Sauce)
	// Copy the IDMS and Signature ConfigMaps generated by oc-mirror into
	// the installer's manifest directory so they are baked into the ISO.
	// ========================================================================
	if cfg.Network.IsolationLevel == "air-gapped" && cfg.Services.Registry.IsManaged() {
		manifestsDir := filepath.Join(targetDir, "openshift")
		exec.Execute(ctx, fmt.Sprintf("mkdir -p %s", manifestsDir))

		ocMirrorResources := filepath.Join(workspaceDir, "oc-mirror-workspace", "working-dir", "cluster-resources")

		// Copy all YAML/JSON files (Signatures, IDMS, ITMS, CatalogSources) into the payload
		copyManifestsCmd := fmt.Sprintf("cp %s/* %s/ 2>/dev/null || true", ocMirrorResources, manifestsDir)
		exec.Execute(ctx, copyManifestsCmd)
	}
	// ========================================================================

	// ========================================================================
	// CI / NIGHTLY INJECTION (Connected OR Disconnected)
	// Inject the Insecure Policy to bypass Signature Validation for raw CI builds
	// ========================================================================
	if cfg.OpenShift.ReleaseType == "ci" { // <--- UPDATED PATH
		if err := injectInsecurePolicy(targetDir); err != nil {
			return fmt.Errorf("failed to inject insecure policy into Agent ISO: %w", err)
		}
	}
	// ========================================================================

	// 3. Run openshift-install agent create image
	toolsDir := filepath.Join(workspaceDir, "tools")
	installerPath := filepath.Join(toolsDir, "openshift-install")

	cmdStr := fmt.Sprintf("cd %s && %s agent create image --dir=. --log-level=info", targetDir, installerPath)

	if cfg.Services.Proxy.IsManaged() || cfg.Services.Proxy.GetHTTP() != "" {
		var proxyURL, noProxy string
		if cfg.Services.Proxy.IsManaged() {
			proxyURL = fmt.Sprintf("http://%s:3128", cfg.Network.ControllerIP)
			noProxy = fmt.Sprintf("localhost,127.0.0.1,%s,%s,.%s.%s",
				cfg.Network.MachineCIDR, cfg.Network.ControllerIP, cfg.OpenShift.ClusterName, cfg.OpenShift.BaseDomain)
		} else {
			proxyURL = cfg.Services.Proxy.GetHTTP()
			noProxy = cfg.Services.Proxy.NoProxy
		}
		cmdStr = fmt.Sprintf("export HTTP_PROXY=%s HTTPS_PROXY=%s NO_PROXY='%s' && ", proxyURL, proxyURL, noProxy) + cmdStr
	}

	cmd := fmt.Sprintf("export PATH=%s:$PATH && %s", toolsDir, cmdStr)

	if _, err := exec.Execute(ctx, cmd); err != nil {
		return fmt.Errorf("failed to create agent ISO: %w", err)
	}

	return nil
}

// generateAgentConfigYAML creates the agent-config.yaml for Agent-based Installer
// Works for both SNO and multi-node clusters
func generateAgentConfigYAML(cfg *types.AgentConfig) (string, error) {
	tmpl, err := template.New("agentConfig").Parse(agentConfigTemplate)
	if err != nil {
		return "", err
	}

	// Calculate prefix length from CIDR
	_, ipNet, err := net.ParseCIDR(cfg.Network.MachineCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid machine CIDR: %w", err)
	}
	prefixLen, _ := ipNet.Mask.Size()

	// Get DNS server based on managed services configuration
	// If DNS is managed by shiftlaunch, use controller IP as DNS resolver
	// Otherwise, use the configured nameserver
	dnsServer := cfg.Services.DNS.GetExternal()
	if cfg.Services.DNS.IsManaged() {
		dnsServer = cfg.Network.ControllerIP
	}

	// Build host configurations for all nodes (SNO or multi-node)
	type HostConfig struct {
		Hostname     string
		Role         string
		MACAddress   string
		IP           string
		PrefixLength int
		Gateway      string
		DNS          string
	}

	var hosts []HostConfig
	for _, node := range cfg.GetAllNodes() {
		role := "master"
		if node.Role == "worker" {
			role = "worker"
		}

		hosts = append(hosts, HostConfig{
			Hostname:     node.Hostname,
			Role:         role,
			MACAddress:   node.MACAddress,
			IP:           node.IP,
			PrefixLength: prefixLen,
			Gateway:      cfg.Network.Gateway,
			DNS:          dnsServer,
		})
	}

	// Get rendezvous IP (first master/SNO node)
	rendezvousIP := ""
	if len(hosts) > 0 {
		rendezvousIP = hosts[0].IP
	}

	data := struct {
		ClusterName  string
		RendezvousIP string
		Hosts        []HostConfig
	}{
		ClusterName:  cfg.OpenShift.ClusterName,
		RendezvousIP: rendezvousIP,
		Hosts:        hosts,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// injectInsecurePolicy writes a MachineConfig that tells the nodes to accept unsigned CI images
func injectInsecurePolicy(targetDir string) error {
	manifestDir := filepath.Join(targetDir, "openshift")
	os.MkdirAll(manifestDir, 0755)

	policy := `apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: master
  name: 99-master-insecure-policy
spec:
  config:
    ignition:
      version: 3.2.0
    storage:
      files:
      - contents:
          source: data:text/plain;charset=utf-8;base64,ewogICJkZWZhdWx0IjogWwogICAgewogICAgICAidHlwZSI6ICJpbnNlY3VyZUFjY2VwdEFueXRoaW5nIgogICAgfQogIF0sCiAgInRyYW5zcG9ydHMiOiB7CiAgICAiZG9ja2VyLWRhZW1vbiI6IHsKICAgICAgIiI6IFsKICAgICAgICB7CiAgICAgICAgICAidHlwZSI6ICJpbnNlY3VyZUFjY2VwdEFueXRoaW5nIgogICAgICAgIH0KICAgICAgXQogICAgfQogIH0KfQ==
        mode: 420
        overwrite: true
        path: /etc/containers/policy.json
---
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: worker
  name: 99-worker-insecure-policy
spec:
  config:
    ignition:
      version: 3.2.0
    storage:
      files:
      - contents:
          source: data:text/plain;charset=utf-8;base64,ewogICJkZWZhdWx0IjogWwogICAgewogICAgICAidHlwZSI6ICJpbnNlY3VyZUFjY2VwdEFueXRoaW5nIgogICAgfQogIF0sCiAgInRyYW5zcG9ydHMiOiB7CiAgICAiZG9ja2VyLWRhZW1vbiI6IHsKICAgICAgIiI6IFsKICAgICAgICB7CiAgICAgICAgICAidHlwZSI6ICJpbnNlY3VyZUFjY2VwdEFueXRoaW5nIgogICAgICAgIH0KICAgICAgXQogICAgfQogIH0KfQ==
        mode: 420
        overwrite: true
        path: /etc/containers/policy.json`

	return os.WriteFile(filepath.Join(manifestDir, "99-insecure-policy.yaml"), []byte(policy), 0644)
}
