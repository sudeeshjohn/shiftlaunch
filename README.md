# ShiftLaunch 🚀

ShiftLaunch is a turnkey, zero-to-cluster local orchestration agent designed to automate the deployment of Red Hat OpenShift clusters on IBM Power Systems (`ppc64le`). By bridging the gap between local infrastructure services (DNS, DHCP, HAProxy) and IBM's Hardware Management Console (HMC), ShiftLaunch provides a seamless, Docker-like CLI experience for deploying, scaling, and managing OpenShift clusters on Power hardware.

## ✨ Key Features

* **Infrastructure as Code:** Automatically configures local `dnsmasq`, `haproxy`, `squid`, and `podman` registries based on a single `config.yaml`
* **Intelligent HMC Orchestration:** Interacts directly with the IBM HMC REST API to discover LPARs, mount Virtual Optical Media (ISO) via NFS to the VIOS, and automate boot sequences
* **Airgap & Disconnected Support:** Built-in `oc-mirror` v2 integration to automatically stand up local container registries and mirror OpenShift payloads for fully disconnected deployments
* **Idempotent Auto-Resume:** Tracks deployment phases in a local `state.json`. If a network drop or crash occurs, ShiftLaunch resumes exactly where it left off without duplicating work
* **Day-2 Scaling:** Seamlessly scale clusters by appending worker nodes to your config and running `shiftlaunch scale`
* **BYOI (Bring Your Own Infrastructure):** Use your existing enterprise Load Balancers, DNS, DHCP, or let ShiftLaunch manage them locally

## 📦 Installation

ShiftLaunch is written in Go and cross-compiles easily.

```bash
# Clone the repository
git clone https://github.ibm.com/sudeeshjohn/shiftlaunch.git
cd shiftlaunch

# Build and install (requires Go 1.26+)
make build
make install
```

**Note:** To cross-compile for an IBM Power System from a Mac/Windows machine, run `make build-ppc64le`.

## 🚀 Quick Start

### 1. Generate a Configuration Template

Create a starter YAML file tailored to your environment.

```bash
# For a standard connected, Agent-boot cluster:
shiftlaunch generate-config -t multi -b agent -o my-cluster.yaml

# For a fully airgapped, netboot cluster:
shiftlaunch generate-config -t multi -b netboot --disconnected -o my-cluster.yaml
```

### 2. Validate Your Infrastructure

Run pre-flight checks against your network, disk space, and HMC to ensure readiness.

```bash
shiftlaunch validate --config my-cluster.yaml
```

### 3. Deploy the Cluster

Execute the orchestration pipeline.

```bash
shiftlaunch create --config my-cluster.yaml
```

### 4. Monitor Status

Watch the deployment status and retrieve your cluster credentials.

```bash
shiftlaunch status --cluster my-cluster
```

## 🛠️ Command Reference

| Command | Description |
|---------|-------------|
| **Core Commands:** | |
| `create` | Execute the cluster deployment pipeline (auto-resumes if failed) |
| `scale` | Scale an existing cluster by adding new worker nodes |
| `remove` / `delete` | Power off LPARs and cleanly remove local services and configurations |
| `list` | List all active managed clusters in the workspace |
| `status` / `info` | Show cluster endpoints, credentials, and deployment phase |
| `logs` | Fetch or stream (`-f`) the deployment logs |
| `prune` | Permanently reclaim disk space from deleted cluster workspaces |
| **Utility Commands:** | |
| `validate` | Run pre-flight validation against YAML and HMC infrastructure |
| `generate-config` | Create a starter config.yaml based on target topology |
| `service-configs` | Dump external unmanaged service configurations for network admins |
| `export kubeconfig` | Export cluster kubeconfig to your local environment |
| `oc` | Wrapper to execute oc commands safely against a specific managed cluster |

## 🏗️ Architecture Overview

### Boot Methods

ShiftLaunch supports two distinct boot mechanisms:

#### 1. Agent-based Installer (Recommended for Production)

- ✅ **Simplified Deployment**: Single ISO contains all installation artifacts
- ✅ **No DHCP/PXE Required**: Eliminates network boot complexity (uses static IPs via NMState)
- ✅ **NFS-Based**: ISO served via NFS from the controller directly to the VIOS
- ✅ **Better for Disconnected**: All artifacts can be embedded in the ISO
- ✅ **Automatic Cleanup**: ISO mappings and NFS exports are removed after installation

#### 2. User Provisioned Infrastructure (UPI / PXE Boot)

- ✅ **Standard PXE Workflow**: Traditional UPI boot method
- ✅ **HMC REST API**: Automated netboot using IBM's Hardware Management Console
- ✅ **DHCP/TFTP/HTTP**: Full network boot stack managed locally
- ✅ **MAC-Based Config**: Automated, per-node GRUB configurations

### Network Isolation Topologies

ShiftLaunch natively supports three enterprise network boundaries:

* **Connected (`connected`):** Nodes have direct outbound internet access
* **Soft Disconnected (`soft-disconnected`):** Nodes are isolated but reach the internet exclusively through a proxy. ShiftLaunch can build a local `squid` proxy or route through a corporate gateway
* **Fully Disconnected (`fully-disconnected`):** Strict airgap. The controller strips all proxy shell variables. It dynamically generates a local `podman` container registry, provisions SSL certificates, and utilizes `oc-mirror` v2 to sync the OpenShift release payload

### Single VIP Architecture

- **One IP per Cluster**: Replaces the traditional dual VIP (API + Ingress) approach
- **Port-Based Routing**: HAProxy routes traffic via Layer 4 TCP based strictly on ports (6443 for API, 22623 for Machine Config, 80/443 for Ingress)
- **Simplified DNS**: All DNS records (api, api-int, *.apps) point to a single VIP

## 📋 Prerequisites

### Infrastructure Requirements

1. **Additional IP Address (VIP)**
   - One dedicated IP address per cluster for the Load Balancer VIP
   - This IP will be aliased to the controller's network interface
   - Must be in the same subnet as your cluster nodes

2. **IBM Power LPARs**

   The number of LPARs required depends on your deployment type:

   | Deployment Type | Boot Method | Minimum LPARs Required | Configuration |
   |-----------------|-------------|------------------------|---------------|
   | **Single Node OpenShift (SNO)** | ISO or Netboot | **1** | 1 Master (combined control plane + worker) |
   | **Multi-Node (Agent ISO)** | ISO | **5** | 3 Masters + 2 Workers |
   | **Multi-Node (Network Boot)** | Netboot | **6** | 1 Bootstrap + 3 Masters + 2 Workers |

   > **Note:** Network Boot requires an additional bootstrap node that is automatically removed after the cluster installation completes.

3. **Controller Node**
   - RHEL 9/10 or CentOS 9/10
   - Network connectivity to HMC and cluster nodes
   - Sufficient disk space for OpenShift artifacts (~10GB per cluster)

### Supported Component Versions

| Component | Supported Versions / Firmware |
|-----------|-------------------------------|
| **Controller Node (OS)** | RHEL 9/10, CentOS 9/10 |
| **Hardware Management Console (HMC)** | V11R2 (Build Level: 2604091530, Service Pack: 1120)<br>V11R1 (Build Level: 2502191030, Service Pack: 1110)<br>V10R3 M1063 |
| **IBM Power Systems (PowerFW)** | RB1120_fw1120.00<br>ML1060_fw1060.51 (148) |
| **Virtual I/O Server (VIOS)** | 4.1.2.0<br>4.1.1.10 |

## 🔌 Bring Your Own Infrastructure (BYOI)

ShiftLaunch supports flexible infrastructure management through the `services` configuration block. You can choose to:

1. **Fully Managed** - ShiftLaunch installs and manages all services locally on the controller (DNS, DHCP, PXE, HAProxy, NFS)
2. **Partially Managed** - Mix and match locally managed services with your external enterprise services
3. **BYOI Mode** - Disable all local services. Use your external F5 Load Balancers, InfoBlox DNS, and enterprise DHCP. ShiftLaunch will act purely as an orchestrator for HMC LPAR provisioning, Ignition generation, and installation monitoring

### Example: Fully Managed (ISO Boot)

```yaml
services:
  dns:
    enabled: true
  dhcp:
    enabled: false          # Not required for NMState-driven ISO boot
  pxe:
    enabled: false          # Not required for ISO boot
  load_balancer:
    enabled: true
  nfs:
    enabled: true           # Required to host Agent ISOs to the VIOS
```

### Example: BYOI Mode (Netboot)

```yaml
services:
  dns:
    enabled: false          # Using enterprise InfoBlox
  dhcp:
    enabled: false          # Using enterprise DHCP
  pxe:
    enabled: false          # Using enterprise PXE server
  load_balancer:
    enabled: false          # Using enterprise F5 Big-IP
  nfs:
    enabled: false
```

## 🌐 Disconnected & Airgapped Deployments

ShiftLaunch natively supports disconnected OpenShift deployments by spinning up a local Podman container registry and a Squid proxy gateway directly on the controller node.

| Architecture | Behavior | Best For |
|--------------|----------|----------|
| **Standard** | Directly pulls from `quay.io`. No local registry or proxy. | Datacenters with open internet access |
| **Corp Proxy** | Routes `quay.io` pulls through a local Squid proxy | Environments requiring strict egress filtering |
| **Strict Airgap** | Creates local registry. Scrubs all host proxy variables. Nodes have **zero** outbound routing | Dark sites, defense, or highly secure financial environments |
| **Soft Airgap** | Creates local registry **and** local proxy | Environments that use local payloads but still need to reach external NTP, LDAP, or third-party operators |

> **Note on CI Builds:** If you set `release_type: "ci"` in your configuration, ShiftLaunch will automatically inject a custom `MachineConfig` to bypass cryptographic signature validation, allowing you to boot raw nightly payloads.

## 🔧 State Management and Idempotency

Every deployment is tracked via `/opt/shiftlaunch/clusters/<name>/state.json`.

* **Locking:** Executions are protected by a PID-aware file lock (`.lock`) that verifies process health via `syscall.Signal(0)` to prevent zombie lockouts
* **Resume Logic:** If a phase (e.g., `downloads` or `services`) succeeds, it is marked complete in the JSON state. A subsequent `shiftlaunch create` command will instantly skip to the failed phase
* **Self-Healing:** The state manager validates the JSON schema upon load and executes a `RecoverState` routine to scrub duplicate events and fix corrupted histories

## 🗑️ Safe Teardown Lifecycle

The `remove` / `delete` command ensures no ghost infrastructure is left behind:

1. **LPAR Power Off:** Transitions LPARs to a safe state before touching storage
2. **VIOS Cleanup:** Unmaps virtual optical media, deletes the ISO from the Media Repository, and unmounts the NFS link
3. **Service Reversion:** Dynamically parses `ip` and `nmcli` to unbind the cluster's VIP without disrupting the controller's primary IP connection
4. **Archival:** Workspaces are marked `.deleted` instead of physically destroyed immediately, allowing admins to inspect logs before running `shiftlaunch prune`

ShiftLaunch uses **Intelligent Partial Failure Handling**. It tracks exactly which resources (LPARs, Virtual Disks, Optical Media) fail to delete. If a deletion fails (e.g., a disk is locked), it preserves the failure in the state file. Re-running the delete command will safely retry *only* the failed resources, ensuring zero orphaned infrastructure.

## 📚 Example Configurations

ShiftLaunch provides pre-configured example YAML files in the `example/` directory:

- `config.yaml`: Basic multi-node configuration template
- `config-sno.yaml`: Single-Node OpenShift (SNO) example
- `config-disc-agent.yaml`: Disconnected deployment with Agent-based installer
- `config-multi-netboot.yaml`: Multi-node cluster with traditional network boot

## 🐛 Troubleshooting

### Common Issues

1. **"File 'config.yaml' already exists. Refusing to overwrite"**
   - **Cause**: You are running `generate-config` but the target file already exists
   - **Solution**: Delete the existing file or specify a different output path using `-config new-name.yaml`

2. **"Cluster is already managed and fully deployed"**
   - **Cause**: You attempted to run `create` on a cluster directory that contains a `.managed` marker (indicating a healthy, finished cluster)
   - **Solution**: To redeploy, you must explicitly `delete` the cluster first to prevent accidental data loss

## 👥 Authors

- **Sudeesh John** (@sudeeshjohn)

## 📄 License

This project is maintained under the `shiftlaunch` repository. See the LICENSE file for details.
