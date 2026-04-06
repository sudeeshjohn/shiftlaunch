# OpenShift UPI Deployer for IBM Power Systems

A comprehensive Go-based tool for deploying multiple OpenShift clusters (SNO or Multi-Node) on IBM Power Systems using User-Provisioned Infrastructure (UPI) method with **full automation** from LPAR creation to cluster ready state.

## Overview

This tool provides **end-to-end automation** for OpenShift deployment on IBM Power Systems:
- ✅ **Network Boot (Netboot)**: Automated PXE boot using HMC REST API
- ✅ **Installation Monitoring**: Automated monitoring with `openshift-install` commands
- ✅ **Single VIP Architecture**: One IP per cluster (50% IP savings)
- ✅ **Multi-Cluster Support**: Deploy multiple clusters from single helper node
- ✅ **Per-Cluster HTTP Directories**: Isolated `/var/www/html/{cluster-name}` structure
- ✅ **Automatic MAC Capture**: Services configured after LPAR creation
- ✅ **Resume Functionality**: Resume failed deployments from last completed phase
- ✅ **State Management**: Track deployment progress with JSON state files
- ✅ **Cluster Directory Structure**: Isolated directories for each cluster with config and state
- ✅ **Granular Dnsmasq Configuration**: Separate DNS, DHCP, and PXE phases for better modularity

## Key Features

### 🚀 Network Boot Implementation
- **Automated Network Boot**: Uses HMC REST API for netboot with static IP configuration
- **MAC to Location Code Translation**: Automatically translates MAC addresses to physical location codes
- **PXE Boot Flow**: Complete automation from DHCP → TFTP → HTTP → Ignition
- **No Manual Intervention**: LPARs boot and install automatically after network boot command

### 📊 Installation Monitoring
- **Automated Monitoring**: Uses `openshift-install wait-for` commands
- **Type-Aware**: Different handling for SNO (skip bootstrap) vs Multi-Node
- **Real-time Feedback**: See installation progress with detailed output
- **Credential Extraction**: Automatically displays console URL and kubeadmin password
- **Kubeconfig Management**: Optionally saves kubeconfig locally

### 🏗️ Architecture

#### Single VIP Architecture
- **One IP per Cluster**: Replaces traditional dual VIP (API + Ingress) approach
- **50% IP Savings**: Deploy twice as many clusters with same IP pool
- **Port-Based Routing**: HAProxy routes traffic based on port (6443, 22623, 80, 443)
- **Simplified DNS**: All DNS records point to single VIP

#### Multi-Cluster Support
- **IP Aliasing**: Each cluster gets dedicated VIP on helper node
- **Per-Cluster Isolation**: Separate directories, configs, and service instances
- **HTTP Directory Structure**: `/var/www/html/{cluster-name}/{ignition,rhcos,tools,scripts}`
- **Standard Ports**: All clusters use standard ports via IP aliasing

### Components

1. **Configuration Management** (`types.go`, `validator.go`)
   - Multi-cluster configuration with per-cluster settings
   - Comprehensive validation of all configuration aspects
   - Support for SNO and Multi-Node topologies

2. **SSH Client** (`ssh.go`)
   - Remote command execution on helper node
   - File upload/download capabilities
   - Streaming output support

3. **Service Configuration Generators**
   - **DNSmasq** (`dnsmasq.go`): Per-cluster DNS, DHCP, and TFTP
   - **HAProxy** (`haproxy.go`): Per-cluster load balancing with VIP binding
   - **HTTP Server** (`httpserver.go`, `downloader.go`, `httphelper.go`): Per-cluster web server for ignition files and RHCOS images

4. **Ignition Generator** (`ignition.go`)
   - Creates `install-config.yaml` from cluster configuration
   - Generates manifests and ignition configs using `openshift-install`
   - Handles SNO vs Multi-Node differences
   - Copies ignition files to HTTP directory

5. **PXE Boot Manager** (`pxeboot.go`)
   - Generates GRUB configs for network booting
   - MAC-based boot configuration
   - Per-node boot parameters

6. **LPAR Provisioner** (`lpar.go`)
   - LPAR creation and management via HMC REST API
   - Storage attachment (VIOS/SVC)
   - MAC address capture and configuration update
   - **Network boot implementation** with HMC API

7. **Orchestrator** (`orchestrator.go`)
   - 13-phase deployment workflow (expanded from 12 phases)
   - Granular dnsmasq configuration (DNS, DHCP, PXE as separate phases)
   - Split installation monitoring (wait_bootstrap and wait_installation phases)
   - Resume functionality for failed deployments
   - State management with cluster-specific JSON files
   - Real-time phase tracking and status updates

## Project Status

### ✅ Fully Implemented Components

1. **Design Document** (`DESIGN.md`)
   - Complete architecture documentation
   - Single VIP architecture explanation
   - Per-cluster service configuration
   - Network boot and installation monitoring flows

2. **Configuration Types** (`types.go`)
   - Multi-cluster configuration structure
   - SNO and Multi-Node support
   - Deployment state tracking
   - Resume functionality support

3. **Validator** (`validator.go`)
   - Comprehensive configuration validation
   - Helper node, HMC, VIP pool validation
   - Power systems, storage, network validation

4. **SSH Client** (`ssh.go`)
   - Full implementation with all required methods
   - Command execution, file transfer, streaming output

5. **Service Generators**
   - **DNSmasq** (`dnsmasq.go`): DNS, DHCP, TFTP with MAC-based static bindings
   - **HAProxy** (`haproxy.go`): Single VIP load balancing with port-based routing
   - **HTTP Server** (`httpserver.go`): Per-cluster directory structure in `/var/www/html`

6. **Ignition Generator** (`ignition.go`)
   - Complete workflow implementation
   - install-config.yaml generation
   - Manifest and ignition config generation
   - File copying to per-cluster HTTP directory

7. **PXE Boot Manager** (`pxeboot.go`)
   - GRUB configuration generation
   - MAC-based boot files (`grub.cfg-01-{mac}`)
   - Per-node boot parameters with cluster-specific URLs

8. **LPAR Provisioner** (`lpar.go`)
   - Full LPAR lifecycle management
   - Network boot implementation using HMC REST API
   - MAC address capture and configuration update
   - Storage attachment (VIOS virtual disks)

9. **Orchestrator** (`orchestrator.go`)
   - 12-phase deployment workflow with granular dnsmasq configuration
   - **Network boot** instead of simple power-on
   - **Installation monitoring** with `openshift-install` commands
   - Resume functionality for failed deployments
   - State management with cluster-specific JSON files in `clusters/<name>/state.json`

10. **Example Configurations**
    - SNO configuration (`cluster-sno.yaml`, `cluster-sno-test.yaml`)
    - Multi-cluster configuration (`config.yaml`, `config-test.yaml`)

### 🎯 Recent Implementations

1. **Greenfield Transformation & Credential Caching (2026-04-07)**
   - **Complete Refactoring**: Removed all backward compatibility code for a 100% greenfield codebase
   - **Node-Level Configuration**: All hardware and storage settings now at node level (single source of truth)
   - **Automatic Credential Caching**: Kubeconfig and kubeadmin-password automatically downloaded to `clusters/{name}/` after deployment
   - **Enhanced Status Display**: Shows service endpoints, credentials, and ready-to-use `/etc/hosts` entries
   - **Comprehensive Validation**: Added VLAN ID (>0), vswitch_name (required), and storage_type ('vios'/'svc') validation
   - **Bug Fixes**:
     - Fixed PowerOffPartition API parameter order
     - Fixed TYPE column YAML parsing for BYOI detection
     - Fixed BYOI power-on handling for running LPARs
     - Fixed status command to show 'completed' deployments
   - **Configuration Simplification**: Removed cluster-level `PowerSystems` and `Storage` blocks - all config embedded at node level
   - **Zero Legacy Support**: No fallback code, no external config files, embedded configs only

2. **Cluster Directory Structure**
   - Single binary manages multiple clusters
   - Each cluster has an isolated directory: `clusters/<cluster-name>/`
   - Configuration is preserved automatically during deployment
   - State is tracked per cluster
   - `list` command shows all managed clusters
   - Delete can optionally remove the cluster directory after cleanup

2. **Intelligent Deletion with Partial Failure Handling**
   - Tracks failed deletions and preserves them in state
   - Idempotent: safe to re-run delete command
   - Only retries resources that previously failed to delete
   - Clear error reporting with resource-specific context
   - No orphaned resources left untracked

3. **Granular DNS/DHCP/PXE Phases**
   - Split monolithic dnsmasq setup into `setup_dns`, `setup_dhcp`, and `setup_pxe`
   - Better debugging and clearer phase tracking
   - Numbered config files ensure deterministic load order

4. **Network Boot and Installation Monitoring**
   - Uses HMC REST API network boot instead of a simple power-on flow
   - Supports MAC-to-location-code translation and retry logic
   - Uses `openshift-install wait-for` commands for automated progress tracking

5. **Resume and State Management**
   - Supports `-resume` deployments from the last failed phase
   - Persists state in `clusters/<name>/state.json`
   - Supports cluster status and list operations from preserved state

### 📚 Documentation

- [`README.md`](README.md) - Getting started, usage, prerequisites, and troubleshooting
- [`DESIGN.md`](DESIGN.md) - Architecture, workflow, configuration model, and deployment phases

### ⚠️ Known Limitations

1. **Environment-Specific Validation**
   - Successful deployment still depends on correct HMC, helper node, storage, DNS, DHCP, and firewall configuration
   - Production environments should be validated carefully before running full deployments

2. **Documentation Drift Risk**
   - The project was recently moved into [`shiftlaunch`](shiftlaunch), so external references in other repositories may still point to the older location until they are updated

3. **Operational Testing Coverage**
   - The design and workflow are documented, but cluster deployment behavior still depends on real infrastructure, helper-node services, and OpenShift installer compatibility

## Configuration

### Multi-Cluster Configuration

The tool uses a two-level configuration:

1. **Top-level config** (`config.yaml`): Defines helper node, HMC, and cluster references
2. **Per-cluster config** (`cluster-sno.yaml`, `cluster-multi.yaml`): Defines cluster-specific settings

Example top-level configuration:

```yaml
helper_node:
  hostname: helper.example.local
  ip: 192.168.1.10
  ssh_user: root
  ssh_key_file: ~/.ssh/id_rsa
  network_interface: eth0
  vip_pool:
    start: 192.168.1.100
    end: 192.168.1.200

hmc:
  ip: 192.168.1.5
  username: YOUR_HMC_USERNAME
  password: EXAMPLE_PASSWORD

clusters:
  - name: ocp-sno
    type: sno
    ocp_version: "4.21"
    vip: 192.168.1.100  # Single VIP for both API and Ingress
    cluster_config:
      pre_provisioned: false
      sno_node:
        ip: "192.168.1.50"
        system_name: "Server-1234"
        vswitch_name: "ETHERNET0"
        vlan_id: 100
        storage_type: "vios"
        # ... rest of node configuration
      network:
        domain: "example.local"
        # ... rest of network configuration
  
  - name: ocp-prod
    type: multi-node
    ocp_version: "4.21"
    vip: 192.168.1.110  # Single VIP for both API and Ingress
    cluster_config:
      pre_provisioned: false
      masters:
        nodes:
          - name: "master-0"
            ip: "192.168.1.60"
            system_name: "Server-1234"
            vswitch_name: "ETHERNET0"
            vlan_id: 100
            storage_type: "vios"
          # ... additional masters
      network:
        domain: "example.local"
        # ... rest of network configuration
```

### Cluster Configuration Structure

All cluster configuration is **embedded** within the main config file using the `cluster_config` block. See `config-sno.yaml` and `config-multi.yaml` for complete examples.

Key sections:
- **Node-level configuration**: All hardware and storage settings at node level
- **storage**: VIOS or SVC storage configuration
- **network**: Cluster networking (domain, CIDR, gateway, etc.)
- **openshift**: OpenShift configuration (version, pull secret, SSH key, etc.)
- **sno_node** / **bootstrap** / **masters** / **workers**: Node definitions
- **deployment**: Deployment phases and timeouts
- **advanced**: Advanced options (parallel operations, monitoring, etc.)

## Usage

### Cluster Management Commands

The deployer provides several commands for managing multiple OpenShift clusters:

```bash
# List all managed clusters
./ocp-upi-deployer -command list

# Deploy a new cluster (config file required)
./ocp-upi-deployer -command deploy -config config.yaml -cluster ocp-sno

# Resume a failed deployment (loads config from cluster directory)
./ocp-upi-deployer -command deploy -cluster ocp-sno -resume

# Check cluster status
./ocp-upi-deployer -command status -cluster ocp-sno

# Delete a cluster (loads config from cluster directory)
./ocp-upi-deployer -command delete -cluster ocp-sno
```

### Cluster Deletion with Partial Failure Handling

The delete command now includes **intelligent partial failure handling** that ensures safe and reliable cleanup:

**Key Features**:
- ✅ **Idempotent**: Safe to re-run multiple times
- ✅ **Partial Failure Recovery**: Tracks and retries only failed deletions
- ✅ **State Preservation**: Failed resources remain in state for retry
- ✅ **Clear Reporting**: Shows exactly what succeeded and what failed
- ✅ **No Orphaned Resources**: Everything tracked until successfully deleted

**Deletion Process** (4 steps):
1. **Close Virtual Terminals & Power Off LPARs** - Graceful shutdown
2. **Unmap Storage** - Batch unmapping from LPARs
3. **Delete Volumes** - Remove virtual disks from VIOS/SVC
4. **Delete LPARs** - Remove partitions from HMC

**Example - Partial Failure Scenario**:
```bash
$ ./ocp-upi-deployer -command delete -cluster ocp-sno

Step 3: Deleting storage volumes...
  Deleting volume: snonew5-n-b-a3f9...
    ⚠ Failed to delete disk snonew5-n-b-a3f9: disk in use
  Deleting volume: snonew5-n-d-b7c2...
    ✅ Deleted virtual disk: snonew5-n-d-b7c2

Step 4: Deleting LPARs...
  Deleting LPAR: sno-new-5...
    ✅ LPAR deleted successfully

Error: infrastructure deletion completed with errors.
The following resources remain: Volume: snonew5-n-b-a3f9

# State file now contains ONLY the failed volume
# Re-run delete to retry only the failed resource:
$ ./ocp-upi-deployer -command delete -cluster ocp-sno
```

**After successful deletion**, you'll be prompted:
```
Do you want to remove the cluster directory? (y/n):
```
- **Yes**: Removes `clusters/ocp-sno/` completely
- **No**: Preserves directory for audit/reference

### Cluster Directory Structure

Each cluster is managed in its own directory under `clusters/<cluster-name>/`:

```
./
├── ocp-upi-deployer              # Single binary
├── clusters/                     # Root directory for all clusters
│   ├── ocp-sno/
│   │   ├── config.yaml          # Copy of config used for deployment
│   │   ├── state.json           # Deployment state tracking
│   │   ├── kubeconfig           # Auto-downloaded after deployment
│   │   └── kubeadmin-password   # Auto-downloaded after deployment
│   ├── ocp-prod/
│   │   ├── config.yaml
│   │   ├── state.json
│   │   ├── kubeconfig
│   │   └── kubeadmin-password
│   └── ocp-test/
│       ├── config.yaml
│       ├── state.json
│       ├── kubeconfig
│       └── kubeadmin-password
```

**Benefits**:
- Single binary manages unlimited clusters
- Each cluster has isolated configuration and state
- Config file automatically preserved during deployment
- Credentials automatically cached locally after deployment
- Easy to backup/restore individual clusters
- Optional directory cleanup on deletion
- Status command reads credentials from local files (no SSH required)

### Prerequisites

1. **Helper/Bastion Node**:
   - RHEL 8/9 or compatible Linux
   - Root SSH access
   - Network connectivity to Power systems and HMC
   - Sufficient disk space for RHCOS images (~5GB per cluster)

2. **HMC**:
   - HMC with REST API enabled
   - User credentials with LPAR management permissions

3. **Power Systems**:
   - Managed by HMC
   - Sufficient resources (CPU, memory, storage)
   - Virtual switches configured

4. **Storage**:
   - VIOS with volume groups OR
   - SVC with storage pools

5. **Network**:
   - DHCP range for cluster nodes
   - DNS forwarders configured
   - VIP pool for API and Ingress endpoints

### Installation

```bash
# Clone the repository that contains the project
git clone <repository-url>
cd shiftlaunch

# Build the tool
go build -o ocp-upi-deployer .

# Verify installation
./ocp-upi-deployer -command version
```

### Deployment Workflow

The CLI entry point is implemented in [`main.go`](main.go) and currently supports `validate`, `deploy`, `delete`, `status`, `list`, and `version`.

```bash
# 1. Validate all configured clusters
./ocp-upi-deployer -command validate -config config.yaml

# 2. Deploy all clusters in the config
./ocp-upi-deployer -command deploy -config config.yaml

# 3. Deploy a specific cluster
./ocp-upi-deployer -command deploy -config config.yaml -cluster ocp-sno

# 4. Resume a failed deployment
./ocp-upi-deployer -command deploy -cluster ocp-sno -resume

# 5. Check cluster status (shows credentials, endpoints, /etc/hosts entries)
./ocp-upi-deployer -command status -cluster ocp-sno

# 6. Delete a cluster
./ocp-upi-deployer -command delete -cluster ocp-sno
```

### Manual Testing of Components

Since the orchestrator is not yet complete, you can test individual components:

```go
// Example: Test SSH connection
sshClient, err := NewSSHClient(helperConfig)
if err != nil {
    log.Fatal(err)
}
defer sshClient.Close()

output, err := sshClient.ExecuteCommand("hostname")
fmt.Println(output)

// Example: Generate DNSmasq config
dnsmasq := NewDNSmasqManager(ctx, sshClient)
if err := dnsmasq.Configure(); err != nil {
    log.Fatal(err)
}

// Example: Generate ignition configs
ignition := NewIgnitionGenerator(ctx, sshClient)
if err := ignition.Generate(); err != nil {
    log.Fatal(err)
}
```

## Implementation Roadmap

### Current State

The project already includes an implemented CLI entry point in [`main.go`](main.go), command handlers under [`cmd/`](cmd), deployment workflow logic under [`orchestrator/`](orchestrator), and supporting packages such as [`communication/`](communication), [`infrastructure/`](infrastructure), [`services/`](services), [`types/`](types), and [`validation/`](validation).

### Near-Term Priorities

1. **Expand Real-World Test Coverage**
   - Add more validation against SNO and multi-node environments
   - Capture expected helper-node prerequisites and failure modes
   - Document verified platform and OpenShift version combinations

2. **Improve Documentation and Runbooks**
   - Keep README and design documentation aligned with the current CLI
   - Add example deployment scenarios and operator runbooks
   - Document migration details now that the project lives in [`shiftlaunch`](shiftlaunch)

3. **Add More Automated Tests**
   - Unit tests for configuration parsing and validation
   - Integration-style tests for command workflows where practical
   - Regression coverage for state management and resume behavior

## Directory Structure

```
shiftlaunch/
├── README.md                    # Project overview and usage
├── DESIGN.md                    # Architecture and workflow documentation
├── go.mod                       # Go module definition
├── go.sum                       # Go dependencies
├── main.go                      # CLI entry point
├── config-sno.yaml              # SNO example configuration
├── config-multi.yaml            # Multi-node example configuration
├── cmd/                         # CLI command handlers
├── communication/               # SSH and remote execution support
├── infrastructure/              # LPAR, network, and storage provisioning
├── orchestrator/                # Deployment and deletion workflow orchestration
├── services/                    # dnsmasq, HAProxy, HTTP, ignition, download logic
├── types/                       # Configuration and state types
├── validation/                  # Configuration validation logic
└── clusters/                    # Per-cluster config/state data created at runtime
```

## Troubleshooting

### Common Issues

1. **SSH Connection Failures**:
   - Verify SSH key permissions (`chmod 600 ~/.ssh/id_rsa`)
   - Check helper node firewall rules
   - Verify SSH user has sudo privileges

2. **HMC Connection Failures**:
   - Verify HMC IP and credentials
   - Check HMC REST API is enabled
   - Verify network connectivity

3. **LPAR Creation Failures**:
   - Check Power system resources (CPU, memory)
   - Verify VIOS/SVC configuration
   - Check HMC logs for detailed errors

4. **PXE Boot Failures**:
   - Verify DHCP configuration
   - Check TFTP directory permissions
   - Verify GRUB configuration files
   - Check network boot order in LPAR profile

5. **Ignition Failures**:
   - Verify pull secret is valid
   - Check SSH public key format
   - Verify RHCOS image URLs are accessible
   - Check ignition file syntax

6. **Network Boot "No Network Adapters Found" Error**:
   ```
   [HMC] Job status: FAILED_BEFORE_COMPLETION
   lpar_netboot : No network adapters found
   lpar_netboot: Unable to obtain network adapter information. Quitting.
   ```
   
   **Root Cause**: LPAR exists but has no network adapter attached. This typically occurs when:
   - LPAR was created in a previous run but network adapter creation failed
   - Network adapter was manually deleted from the LPAR
   - Attempting to netboot an LPAR created without a network adapter
   
   **Solution**: The deployer now verifies network adapter exists before attempting netboot and provides clear recovery steps:
   ```
   LPAR sno-new-2 has no network adapters attached
   This usually means the LPAR was created but network adapter creation failed.
   Solution:
     1. Delete the LPAR from HMC
     2. Delete deployment-state-sno-new-2.json
     3. Re-run deployment from create_lpars phase:
        ./main -command deploy -config config.yaml -cluster sno-new-2 -phases create_lpars,setup_dnsmasq,power_on
   ```
   
   **Prevention**: The fix ensures proper state management by:
   - Storing both `SystemName` and `SystemUUID` in LPARState
   - Verifying network adapter exists before netboot attempt
   - Providing actionable error messages with recovery steps
   
   See [`NETBOOT_FIX.md`](NETBOOT_FIX.md) for technical details.

7. **Network Boot Location Code Errors**:
   ```
   lpar_netboot: can not find physical location U9105.22A.789C301-V10-C2-T1.
   actual location is U9105.22A.789C301-V10-C2-T0
   ```
   
   **Root Cause**: HMC API returns location codes without port suffix (`-T0` or `-T1`), but netboot requires the full location code with suffix.
   
   **Solution**: The deployer automatically:
   - Fetches base location code from HMC using `GetClientNetworkAdapters()`
   - Tries netboot with `-T0` suffix first (most common)
   - If that fails with location error, retries with `-T1` suffix
   - Caches the working suffix for future boots
   
   **Manual Verification**: Use the test utility to check location codes:
   ```bash
   cd powerhmc-go/examples/test-location-code
   go build
   ./test-location-code \
     -hmc-ip 10.20.27.162 \
     -hmc-user YOUR_HMC_USERNAME \
     -hmc-pass <password> \
     -system-name <system-name> \
     -lpar-name <lpar-name>
   ```
   
   Then verify in LPAR SMS menu: `Boot Options` → `Select Boot Options` → `Network` to see the actual location code with suffix.

## Contributing

Contributions are welcome! Please focus on:

1. **Completing LPAR Provisioner**: Full implementation using powerhmc-go
2. **Creating Orchestrator**: Coordinate all deployment phases
3. **Creating Main Entry Point**: CLI interface
4. **Adding Tests**: Unit and integration tests
5. **Improving Documentation**: Usage examples, troubleshooting

## References

- [OpenShift UPI Documentation](https://docs.openshift.com/container-platform/latest/installing/installing_ibm_power/installing-ibm-power.html)
- [ocp4-helpernode](https://github.com/RedHatOfficial/ocp4-helpernode)
- [powerhmc-go](https://github.com/sudeeshjohn/powerhmc-go)
- [IBM Power Systems Documentation](https://www.ibm.com/docs/en/power-systems)

## License

This project is currently maintained as [`shiftlaunch`](shiftlaunch). Confirm and document the repository license in this directory as part of project packaging and release preparation.

## Authors

- Sudeesh John (@sudeeshjohn)
- Built with assistance from Bob (AI Assistant)

## Acknowledgments

- Based on the ocp4-helpernode Ansible playbooks
- Uses powerhmc-go for HMC REST API interactions
- Inspired by the need for native Go implementation without Ansible dependencies