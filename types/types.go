package types

import (
	"fmt"
)

// AgentConfig represents the root of the ShiftLaunch configuration.
type AgentConfig struct {
	Services  ServicesConfig     `yaml:"services"`
	Network   NetworkConfig      `yaml:"network"`
	OpenShift OpenShiftConfig    `yaml:"openshift"`
	HMC       HMCConfig          `yaml:"hmc"`
	Nodes     ClusterNodesConfig `yaml:"nodes"`
}

// ServicesConfig maps the structural infrastructure configurations.
type ServicesConfig struct {
	DNS          ServiceDNS          `yaml:"dns"`
	DHCP         ServiceDHCP         `yaml:"dhcp"`
	PXE          ServicePXE          `yaml:"pxe"`
	LoadBalancer ServiceLoadBalancer `yaml:"load_balancer"`
	NFS          ServiceNFS          `yaml:"nfs"`
	Proxy        ServiceProxy        `yaml:"proxy"`
	Registry     ServiceRegistry     `yaml:"registry"`
}

type ServiceDNS struct {
	Enabled             bool     `yaml:"enabled"`
	ExternalNameserver  string   `yaml:"external_nameserver,omitempty"`
	UpstreamNameservers []string `yaml:"dns_forwarders,omitempty"`
}

type ServiceDHCP struct {
	Enabled      bool   `yaml:"enabled"`
	DHCPServerIP string `yaml:"external_dhcp_server,omitempty"`
}

type ServicePXE struct {
	Enabled     bool   `yaml:"enabled"`
	PXEServerIP string `yaml:"external_pxe_server,omitempty"`
}

type ServiceLoadBalancer struct {
	Enabled bool   `yaml:"enabled"`
	VIP     string `yaml:"vip,omitempty"`
}

type ServiceNFS struct {
	Enabled     bool   `yaml:"enabled"`
	NFSServerIP string `yaml:"external_nfs_server,omitempty"`
}

type ServiceProxy struct {
	Enabled            bool   `yaml:"enabled"`
	ExternalHTTPProxy  string `yaml:"external_http_proxy,omitempty"`
	ExternalHTTPSProxy string `yaml:"external_https_proxy,omitempty"`
	NoProxy            string `yaml:"no_proxy,omitempty"`
}

type ServiceRegistry struct {
	Enabled          bool   `yaml:"enabled"`
	AutoMirror       bool   `yaml:"-"` // Internal tracking field
	ExternalHostname string `yaml:"external_reqistry_server,omitempty"`
	Username         string `yaml:"username,omitempty"`
	Password         string `yaml:"password,omitempty"`
	CACertFile       string `yaml:"ca_cert_file,omitempty"`
	RegistryImage    string `yaml:"-"` // Managed runtime tracking
	ReleaseImage     string `yaml:"-"` // Managed runtime tracking
	LocalRepo        string `yaml:"-"` // Managed runtime tracking
}

// NetworkConfig holds layout bounds and localization assignments.
type NetworkConfig struct {
	IsolationLevel      string `yaml:"isolation_level"` // "connected", "soft-disconnected", "fully-disconnected"
	ControllerInterface string `yaml:"controller_interface"`
	MachineCIDR         string `yaml:"machine_network_cidr"`
	Gateway             string `yaml:"gateway"`
	ControllerIP        string `yaml:"-"` // Auto-Discovered
}

type HMCConfig struct {
	IP       string `yaml:"ip"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type OpenShiftConfig struct {
	ClusterName        string          `yaml:"cluster_name"`
	Version            string          `yaml:"version"`
	ReleaseType        string          `yaml:"release_type"`
	BaseDomain         string          `yaml:"base_domain"`
	ClusterNetworkCIDR string          `yaml:"cluster_network_cidr,omitempty"`
	HostPrefix         int             `yaml:"cluster_network_host_prefix,omitempty"`
	ServiceNetwork     string          `yaml:"service_network,omitempty"`
	PullSecretFile     string          `yaml:"pull_secret_file"`
	SSHPublicKeyFile   string          `yaml:"ssh_public_key_file"`
	ForceOCPDownload   bool            `yaml:"force_ocp_download,omitempty"`
	RHCOSImages        RHCOSURLs       `yaml:"rhcos_images,omitempty"`
	OCPClientConfig    OCPClientConfig `yaml:"ocp_client_config,omitempty"`
}

type RHCOSURLs struct {
	KernelURL    string `yaml:"kernel_url,omitempty"`
	InitramfsURL string `yaml:"initramfs_url,omitempty"`
	RootfsURL    string `yaml:"rootfs_url,omitempty"`
}

type OCPClientConfig struct {
	Client       string `yaml:"ocp_client,omitempty"`
	Installer    string `yaml:"ocp_installer,omitempty"`
	MirrorClient string `yaml:"ocp_mirror_client,omitempty"`
	ChecksumURL  string `yaml:"checksum_url,omitempty"`
}

type ClusterNodesConfig struct {
	BootMethod string       `yaml:"boot_method"`
	Bootstrap  []NodeConfig `yaml:"bootstrap,omitempty"`
	Masters    []NodeConfig `yaml:"masters,omitempty"`
	Workers    []NodeConfig `yaml:"workers,omitempty"`
}

type NodeConfig struct {
	Hostname         string `yaml:"name"`
	IP               string `yaml:"ip"`
	ExistingLPARName string `yaml:"existing_lpar_name"`
	SystemName       string `yaml:"system_name"`
	MACAddress       string `yaml:"mac_address,omitempty"`
	LocationCode     string `yaml:"-"`
	ProfileUUID      string `yaml:"-"`
	UUID             string `yaml:"-"`
	Role             string `yaml:"-"`
}

// IsSNO dynamically computes if the target topology forms an SNO pattern based purely on masters count.
func (c *AgentConfig) IsSNO() bool {
	return len(c.Nodes.Masters) == 1
}

// GetAllNodes aggregates node references, completely isolating the inventory track if SNO rules trigger.
func (c *AgentConfig) GetAllNodes() []*NodeConfig {
	var nodes []*NodeConfig

	if c.IsSNO() {
		if len(c.Nodes.Masters) > 0 {
			c.Nodes.Masters[0].Role = "sno"
			nodes = append(nodes, &c.Nodes.Masters[0])
		}
		return nodes
	}

	for i := range c.Nodes.Bootstrap {
		c.Nodes.Bootstrap[i].Role = "bootstrap"
		nodes = append(nodes, &c.Nodes.Bootstrap[i])
	}
	for i := range c.Nodes.Masters {
		c.Nodes.Masters[i].Role = "master"
		nodes = append(nodes, &c.Nodes.Masters[i])
	}
	for i := range c.Nodes.Workers {
		c.Nodes.Workers[i].Role = "worker"
		nodes = append(nodes, &c.Nodes.Workers[i])
	}

	return nodes
}

// Validate processes configuration layout rules before triggering pipelines.
func (c *AgentConfig) Validate() error {
	if c.IsSNO() {
		if c.Nodes.BootMethod == "netboot" {
			return fmt.Errorf("topology mismatch: Single Node OpenShift (SNO) is completely incompatible with 'netboot'")
		}
		if len(c.Nodes.Workers) > 0 || len(c.Nodes.Bootstrap) > 0 {
			return fmt.Errorf("topology conflict: exactly 1 master node is defined, triggering Single Node OpenShift (SNO) mode. However, you still have targets listed under 'workers' or 'bootstrap'. Please remove those extra nodes to proceed")
		}
		return nil
	}

	if len(c.Nodes.Masters) < 3 {
		return fmt.Errorf("quorum error: multi-node clusters require a minimum of 3 control plane 'masters' nodes, found %d", len(c.Nodes.Masters))
	}
	if len(c.Nodes.Masters)%2 == 0 {
		return fmt.Errorf("quorum error: master node count must be an odd number to maintain etcd quorum, found %d", len(c.Nodes.Masters))
	}
	if c.Nodes.BootMethod == "netboot" && len(c.Nodes.Bootstrap) == 0 {
		return fmt.Errorf("topology error: netboot environments require exactly 1 target assigned within the 'bootstrap' parameter block")
	}
	if c.Nodes.BootMethod == "agent" && len(c.Nodes.Bootstrap) > 0 {
		return fmt.Errorf("topology error: agent boot architectures do not use a standalone bootstrap node. Please remove the 'bootstrap' block")
	}
	return nil
}

// Made with Bob
