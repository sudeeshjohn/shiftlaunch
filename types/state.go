package types

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.ibm.com/sudeeshjohn/shiftlaunch/logger"
)

// CommandExecution tracks details of each command run
type CommandExecution struct {
	Command      string            `json:"command"` // create, delete, validate, status, etc.
	StartTime    string            `json:"start_time"`
	EndTime      string            `json:"end_time,omitempty"`
	Duration     string            `json:"duration,omitempty"`
	Status       string            `json:"status"` // success, failed, in_progress
	Error        string            `json:"error,omitempty"`
	User         string            `json:"user"`
	Hostname     string            `json:"hostname"`
	PID          int               `json:"pid"`
	ConfigFile   string            `json:"config_file,omitempty"`
	Flags        map[string]string `json:"flags,omitempty"`
	PhasesBefore []string          `json:"phases_before,omitempty"` // Phases completed before this command
	PhasesAfter  []string          `json:"phases_after,omitempty"`  // Phases completed after this command
}

// PhaseExecution tracks details of each phase execution
type PhaseExecution struct {
	Phase     string `json:"phase"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time,omitempty"`
	Duration  string `json:"duration,omitempty"`
	Status    string `json:"status"` // success, failed, skipped
	Error     string `json:"error,omitempty"`
	Attempts  int    `json:"attempts,omitempty"` // Number of retry attempts
}

// DownloadedArtifact tracks downloaded files during downloads phase
type DownloadedArtifact struct {
	Name        string `json:"name"` // e.g., "openshift-install", "oc", "kernel", "initramfs"
	Type        string `json:"type"` // e.g., "tool", "rhcos", "iso"
	URL         string `json:"url"`
	Destination string `json:"destination"`
	Size        int64  `json:"size,omitempty"`     // File size in bytes
	Checksum    string `json:"checksum,omitempty"` // Expected checksum
	Status      string `json:"status"`             // "downloading", "completed", "failed", "skipped"
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Duration    string `json:"duration,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ConfiguredService tracks service configuration during services phase
type ConfiguredService struct {
	Name        string `json:"name"`                   // e.g., "DNS", "DHCP", "PXE", "HAProxy", "NFS"
	Type        string `json:"type"`                   // e.g., "network", "storage", "loadbalancer"
	Status      string `json:"status"`                 // "configuring", "completed", "failed", "skipped"
	Managed     bool   `json:"managed"`                // true if managed by shiftlaunch
	ConfigFile  string `json:"config_file,omitempty"`  // Path to config file
	ServiceName string `json:"service_name,omitempty"` // Systemd service name
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Duration    string `json:"duration,omitempty"`
	Error       string `json:"error,omitempty"`
	Details     string `json:"details,omitempty"` // Additional info
}

// DiscoveredNode tracks node metadata discovered from HMC
type DiscoveredNode struct {
	Hostname     string `json:"hostname"`
	Role         string `json:"role"` // master, worker, bootstrap
	IP           string `json:"ip"`
	MACAddress   string `json:"mac_address"`
	UUID         string `json:"uuid"`
	ProfileUUID  string `json:"profile_uuid"`
	LocationCode string `json:"location_code"`
	SystemName   string `json:"system_name"`
	LPARName     string `json:"lpar_name"`
	DiscoveredAt string `json:"discovered_at"` // Timestamp
}

// NFSMount tracks NFS mount points on VIOS for cleanup
type NFSMount struct {
	VIOSUUID   string `json:"vios_uuid"`
	VIOSName   string `json:"vios_name"`
	SystemName string `json:"system_name"`
	MountPoint string `json:"mount_point"`
	NFSServer  string `json:"nfs_server"`
	ExportPath string `json:"export_path"`
	MountedAt  string `json:"mounted_at"` // Timestamp
}

// ISOMapping tracks ISO media mappings for cleanup
type ISOMapping struct {
	NodeName   string `json:"node_name"`
	MediaName  string `json:"media_name"`
	VIOSUUID   string `json:"vios_uuid"`
	VIOSName   string `json:"vios_name"`
	LparUUID   string `json:"lpar_uuid"`
	SystemName string `json:"system_name"`
	MountPoint string `json:"mount_point"` // Reference to NFSMount
	MappedAt   string `json:"mapped_at"`   // Timestamp
}

// DownloadProgress tracks individual file download progress
type DownloadProgress struct {
	URL             string `json:"url"`
	Destination     string `json:"destination"`
	Status          string `json:"status"` // "pending", "downloading", "completed", "failed", "verified"
	BytesDownloaded int64  `json:"bytes_downloaded,omitempty"`
	TotalBytes      int64  `json:"total_bytes,omitempty"`
	Checksum        string `json:"checksum,omitempty"`
	StartedAt       string `json:"started_at,omitempty"`
	CompletedAt     string `json:"completed_at,omitempty"`
	Error           string `json:"error,omitempty"`
}

// ServiceOperation tracks individual service configuration operations
type ServiceOperation struct {
	Service     string `json:"service"`
	Operation   string `json:"operation"` // "install_packages", "configure_firewall", "add_vip", etc.
	Status      string `json:"status"`    // "pending", "in_progress", "completed", "failed"
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Details     string `json:"details,omitempty"`
	Error       string `json:"error,omitempty"`
}

// NodeBootStatus tracks per-node boot operation status
type NodeBootStatus struct {
	NodeName        string `json:"node_name"`
	Role            string `json:"role"`
	NFSMounted      bool   `json:"nfs_mounted,omitempty"`
	NFSMountPoint   string `json:"nfs_mount_point,omitempty"`
	ISOMapped       bool   `json:"iso_mapped,omitempty"`
	ISOMediaName    string `json:"iso_media_name,omitempty"`
	NetworkBooted   bool   `json:"network_booted,omitempty"`
	PoweredOn       bool   `json:"powered_on,omitempty"`
	BootStartedAt   string `json:"boot_started_at,omitempty"`
	BootCompletedAt string `json:"boot_completed_at,omitempty"`
}

// CleanupProgress tracks teardown operation progress
type CleanupProgress struct {
	StartedAt         string   `json:"started_at"`
	PowerOffCompleted []string `json:"power_off_completed,omitempty"` // List of node names
	ISOUnmapped       []string `json:"iso_unmapped,omitempty"`        // List of node names
	NFSUnmounted      []string `json:"nfs_unmounted,omitempty"`       // List of VIOS UUIDs
	ServicesRemoved   []string `json:"services_removed,omitempty"`    // List of service names
	VIPRemoved        bool     `json:"vip_removed,omitempty"`
	Status            string   `json:"status"` // "in_progress", "completed", "failed"
	CompletedAt       string   `json:"completed_at,omitempty"`
	Error             string   `json:"error,omitempty"`
}

// DeploymentState tracks the progress of the local agent execution
type DeploymentState struct {
	StateVersion    int      `json:"state_version"` // Schema version for migration support
	ClusterName     string   `json:"cluster_name"`
	Status          string   `json:"status"` // e.g., "in_progress", "completed", "failed", "deleted"
	CurrentPhase    string   `json:"current_phase"`
	CompletedPhases []string `json:"completed_phases"`
	StartTime       string   `json:"start_time"`
	EndTime         string   `json:"end_time,omitempty"`
	Error           string   `json:"error,omitempty"`
	Locked          bool     `json:"locked"` // Indicates if deployment is currently running
	LockTime        string   `json:"lock_time,omitempty"`

	// Enhanced tracking
	CommandHistory      []CommandExecution   `json:"command_history,omitempty"`
	PhaseHistory        []PhaseExecution     `json:"phase_history,omitempty"`
	ConfigBackups       []string             `json:"config_backups,omitempty"` // List of config backup files
	LastConfigUpdate    string               `json:"last_config_update,omitempty"`
	TotalDuration       string               `json:"total_duration,omitempty"`
	ResumeCount         int                  `json:"resume_count,omitempty"`         // Number of times resumed
	Version             string               `json:"version,omitempty"`              // ShiftLaunch version
	DownloadedArtifacts []DownloadedArtifact `json:"downloaded_artifacts,omitempty"` // Downloaded files tracking
	ConfiguredServices  []ConfiguredService  `json:"configured_services,omitempty"`  // Services configuration tracking
	DiscoveredNodes     []DiscoveredNode     `json:"discovered_nodes,omitempty"`     // Nodes discovered from HMC
	NFSMounts           []NFSMount           `json:"nfs_mounts,omitempty"`           // NFS mounts on VIOS for cleanup
	ISOMappings         []ISOMapping         `json:"iso_mappings,omitempty"`         // ISO media mappings for cleanup

	// VIOS admin user management
	VIOSAdminUsername  string `json:"vios_admin_username,omitempty"`   // viosadmin username
	VIOSAdminPassword  string `json:"vios_admin_password,omitempty"`   // viosadmin password
	VIOSAdminCreated   bool   `json:"vios_admin_created,omitempty"`    // true if we created the user, false if it already existed
	VIOSAdminCheckedAt string `json:"vios_admin_checked_at,omitempty"` // timestamp when user was checked/created

	// Granular event tracking (NEW)
	CompletedEvents   []string                    `json:"completed_events,omitempty"`   // Individual operation markers
	FailedEvents      []string                    `json:"failed_events,omitempty"`      // Failed operation records
	DownloadProgress  map[string]DownloadProgress `json:"download_progress,omitempty"`  // Per-file download tracking
	ServiceOperations []ServiceOperation          `json:"service_operations,omitempty"` // Per-operation service tracking
	NodeBootStatus    map[string]NodeBootStatus   `json:"node_boot_status,omitempty"`   // Per-node boot tracking
	CleanupProgress   *CleanupProgress            `json:"cleanup_progress,omitempty"`   // Teardown progress tracking
}

// StateManager handles state file operations with locking and backup
type StateManager struct {
	clusterName  string
	workspaceDir string
	lockFile     *os.File
	logger       *logger.Logger
}

// NewStateManager creates a new state manager for a cluster
func NewStateManager(clusterName string) *StateManager {
	// Create a simple logger for state manager (no file logging)
	log, _ := logger.New(false, "")
	return &StateManager{
		clusterName:  clusterName,
		workspaceDir: filepath.Join("/opt/shiftlaunch/clusters", clusterName),
		logger:       log,
	}
}

// NewStateManagerWithLogger creates a new state manager with a custom logger
func NewStateManagerWithLogger(clusterName string, log *logger.Logger) *StateManager {
	return &StateManager{
		clusterName:  clusterName,
		workspaceDir: filepath.Join("/opt/shiftlaunch/clusters", clusterName),
		logger:       log,
	}
}

// GetStatePath returns the path to the state file
func (sm *StateManager) GetStatePath() string {
	return filepath.Join(sm.workspaceDir, "state.json")
}

// GetLockPath returns the path to the lock file
func (sm *StateManager) GetLockPath() string {
	return filepath.Join(sm.workspaceDir, ".lock")
}

// GetDeletedMarkerPath returns the path to the deleted marker file
func (sm *StateManager) GetDeletedMarkerPath() string {
	return filepath.Join(sm.workspaceDir, ".deleted")
}

// GetManagedMarkerPath returns the path to the managed marker file
func (sm *StateManager) GetManagedMarkerPath() string {
	return filepath.Join(sm.workspaceDir, ".managed")
}

// AcquireLock attempts to acquire an exclusive lock on the cluster
func (sm *StateManager) AcquireLock() error {
	// Ensure workspace directory exists
	if err := os.MkdirAll(sm.workspaceDir, 0755); err != nil {
		return fmt.Errorf("failed to create workspace directory: %w", err)
	}

	lockPath := sm.GetLockPath()

	// Check if lock file exists
	if _, err := os.Stat(lockPath); err == nil {
		// Read the PID from the lock file
		lockData, readErr := os.ReadFile(lockPath)
		if readErr == nil {
			var lockedPID int
			// Parse the PID from the format: "Locked at: <time>\nPID: <pid>\n"
			lines := strings.Split(string(lockData), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "PID: ") {
					fmt.Sscanf(line, "PID: %d", &lockedPID)
					break
				}
			}

			// Active Process Check: Does this PID actually exist?
			if lockedPID > 0 {
				process, err := os.FindProcess(lockedPID)
				if err == nil {
					// Sending signal 0 checks if the process is alive without actually killing it
					if sigErr := process.Signal(syscall.Signal(0)); sigErr != nil {
						// Process is dead! The lock is a zombie.
						sm.logger.Debug("Detected stale lock from a dead process. Pruning lock automatically.")
						os.Remove(lockPath)
					} else {
						return fmt.Errorf("cluster '%s' is actively locked by a running process (PID: %d). Refusing to overwrite", sm.clusterName, lockedPID)
					}
				}
			}
		} else {
			// If we can't read it, fall back to the 1-hour safety limit
			info, _ := os.Stat(lockPath)
			if time.Since(info.ModTime()) > time.Hour {
				os.Remove(lockPath)
			} else {
				return fmt.Errorf("cluster '%s' is locked. Remove %s manually if you are sure no deployment is running", sm.clusterName, lockPath)
			}
		}
	}

	// Try to open existing lock file or create new one
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}

	// Apply file lock (advisory lock) - this will fail if another process holds it
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lockFile.Close()
		return fmt.Errorf("cluster '%s' is actively locked by another process", sm.clusterName)
	}

	// Truncate and write lock metadata
	lockFile.Truncate(0)
	lockFile.Seek(0, 0)
	lockData := fmt.Sprintf("Locked at: %s\nPID: %d\n", time.Now().Format(time.RFC3339), os.Getpid())
	lockFile.WriteString(lockData)
	lockFile.Sync()

	sm.lockFile = lockFile
	return nil
}

// ReleaseLock releases the cluster lock
func (sm *StateManager) ReleaseLock() error {
	if sm.lockFile == nil {
		return nil
	}

	// Release file lock
	syscall.Flock(int(sm.lockFile.Fd()), syscall.LOCK_UN)

	// Close and remove lock file
	sm.lockFile.Close()
	os.Remove(sm.GetLockPath())
	sm.lockFile = nil

	return nil
}

// LoadState reads the state file from disk
func (sm *StateManager) LoadState() (*DeploymentState, error) {
	path := sm.GetStatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var state DeploymentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	// Initialize StateVersion if not set (backward compatibility)
	if state.StateVersion == 0 {
		state.StateVersion = 2 // Current version with granular tracking
	}

	// Evaluate the state for corruption or legacy structures
	issues := sm.ValidateState(&state)
	if len(issues) > 0 {
		sm.logger.Debug("State inconsistencies detected upon load. Triggering auto-recovery...", "issues", len(issues))
		// Execute the self-healing routine
		if err := sm.RecoverState(&state); err != nil {
			sm.logger.Warn("Failed to auto-recover corrupted state file", "error", err)
		}
	}

	return &state, nil
}

// SaveState writes the state file to disk with backup
func (sm *StateManager) SaveState(state *DeploymentState) error {
	if err := os.MkdirAll(sm.workspaceDir, 0755); err != nil {
		return err
	}

	path := sm.GetStatePath()

	// Create backup if state file exists (Terraform-style)
	if _, err := os.Stat(path); err == nil {
		backupPath := path + ".backup"

		// Read existing state
		existingData, err := os.ReadFile(path)
		if err == nil {
			// Write backup
			if err := os.WriteFile(backupPath, existingData, 0600); err != nil {
				// Log warning but don't fail
				sm.logger.Warn("Failed to create state backup", "error", err)
			}
		}
	}

	// Marshal state to JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	// Write to temporary file first (atomic write)
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return err
	}

	// Rename temp file to actual state file (atomic operation)
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return err
	}

	return nil
}

// IsDeleted checks if the cluster has been deleted
func (sm *StateManager) IsDeleted() bool {
	_, err := os.Stat(sm.GetDeletedMarkerPath())
	return err == nil
}

// MarkDeleted creates the .deleted marker file
func (sm *StateManager) MarkDeleted() error {
	deletedPath := sm.GetDeletedMarkerPath()
	content := fmt.Sprintf("Deleted at: %s\n", time.Now().Format(time.RFC3339))
	return os.WriteFile(deletedPath, []byte(content), 0644)
}

// ClearDeleted removes the .deleted marker file (for new deployments)
func (sm *StateManager) ClearDeleted() error {
	deletedPath := sm.GetDeletedMarkerPath()
	if _, err := os.Stat(deletedPath); err == nil {
		return os.Remove(deletedPath)
	}
	return nil
}

// MarkManaged creates the .managed marker file
func (sm *StateManager) MarkManaged() error {
	managedPath := sm.GetManagedMarkerPath()
	content := fmt.Sprintf("Managed by ShiftLaunch\nCreated at: %s\n", time.Now().Format(time.RFC3339))
	return os.WriteFile(managedPath, []byte(content), 0644)
}

// IsManaged checks if the cluster is managed by ShiftLaunch
func (sm *StateManager) IsManaged() bool {
	_, err := os.Stat(sm.GetManagedMarkerPath())
	return err == nil
}

// AddCommandExecution adds a command execution record to the state
func (sm *StateManager) AddCommandExecution(state *DeploymentState, cmd CommandExecution) {
	if state.CommandHistory == nil {
		state.CommandHistory = []CommandExecution{}
	}
	state.CommandHistory = append(state.CommandHistory, cmd)
}

// AddPhaseExecution adds a phase execution record to the state
func (sm *StateManager) AddPhaseExecution(state *DeploymentState, phase PhaseExecution) {
	if state.PhaseHistory == nil {
		state.PhaseHistory = []PhaseExecution{}
	}
	state.PhaseHistory = append(state.PhaseHistory, phase)
}

// AddConfigBackup records a config backup file
func (sm *StateManager) AddConfigBackup(state *DeploymentState, backupPath string) {
	if state.ConfigBackups == nil {
		state.ConfigBackups = []string{}
	}
	state.ConfigBackups = append(state.ConfigBackups, backupPath)
	state.LastConfigUpdate = time.Now().Format(time.RFC3339)
}

// Legacy functions for backward compatibility

// LoadState reads the state file from disk (legacy function)
func LoadState(clusterName string) (*DeploymentState, error) {
	sm := NewStateManager(clusterName)
	return sm.LoadState()
}

// Save writes the state file to disk (legacy method)
func (s *DeploymentState) Save() error {
	sm := NewStateManager(s.ClusterName)
	return sm.SaveState(s)
}

// GetFailedMarkerPath returns the path to the failed marker file
func (sm *StateManager) GetFailedMarkerPath() string {
	return filepath.Join(sm.workspaceDir, ".failed")
}

// MarkFailed creates the .failed marker file
func (sm *StateManager) MarkFailed() error {
	failedPath := sm.GetFailedMarkerPath()
	content := fmt.Sprintf("Deployment failed at: %s\n", time.Now().Format(time.RFC3339))
	return os.WriteFile(failedPath, []byte(content), 0644)
}

// ClearFailed removes the .failed marker file
func (sm *StateManager) ClearFailed() error {
	failedPath := sm.GetFailedMarkerPath()
	if _, err := os.Stat(failedPath); err == nil {
		return os.Remove(failedPath)
	}
	return nil
}

// IsFailed checks if the cluster deployment is in a failed state
func (sm *StateManager) IsFailed() bool {
	_, err := os.Stat(sm.GetFailedMarkerPath())
	return err == nil
}

// RecordCompletedEvent marks an event as completed and saves state
func (sm *StateManager) RecordCompletedEvent(state *DeploymentState, event string) error {
	if state.CompletedEvents == nil {
		state.CompletedEvents = []string{}
	}

	// Check if already recorded
	for _, e := range state.CompletedEvents {
		if e == event {
			return nil // Already recorded
		}
	}

	state.CompletedEvents = append(state.CompletedEvents, event)
	return sm.SaveState(state)
}

// RecordFailedEvent marks an event as failed with error details and saves state
func (sm *StateManager) RecordFailedEvent(state *DeploymentState, event string, err error) error {
	if state.FailedEvents == nil {
		state.FailedEvents = []string{}
	}

	failedEvent := fmt.Sprintf("%s: %v", event, err)
	state.FailedEvents = append(state.FailedEvents, failedEvent)
	return sm.SaveState(state)
}

// IsEventCompleted checks if a specific event was completed
func (sm *StateManager) IsEventCompleted(state *DeploymentState, event string) bool {
	for _, e := range state.CompletedEvents {
		if e == event {
			return true
		}
	}
	return false
}

// UpdateDownloadProgress updates download tracking for a specific file
func (sm *StateManager) UpdateDownloadProgress(state *DeploymentState, key string, progress DownloadProgress) error {
	if state.DownloadProgress == nil {
		state.DownloadProgress = make(map[string]DownloadProgress)
	}
	state.DownloadProgress[key] = progress
	return sm.SaveState(state)
}

// GetDownloadProgress retrieves download progress for a specific file
func (sm *StateManager) GetDownloadProgress(state *DeploymentState, key string) (DownloadProgress, bool) {
	if state.DownloadProgress == nil {
		return DownloadProgress{}, false
	}
	progress, exists := state.DownloadProgress[key]
	return progress, exists
}

// UpdateNodeBootStatus updates per-node boot tracking
func (sm *StateManager) UpdateNodeBootStatus(state *DeploymentState, nodeName string, status NodeBootStatus) error {
	if state.NodeBootStatus == nil {
		state.NodeBootStatus = make(map[string]NodeBootStatus)
	}
	state.NodeBootStatus[nodeName] = status
	return sm.SaveState(state)
}

// GetNodeBootStatus retrieves boot status for a specific node
func (sm *StateManager) GetNodeBootStatus(state *DeploymentState, nodeName string) (NodeBootStatus, bool) {
	if state.NodeBootStatus == nil {
		return NodeBootStatus{}, false
	}
	status, exists := state.NodeBootStatus[nodeName]
	return status, exists
}

// AddServiceOperation records a service operation
func (sm *StateManager) AddServiceOperation(state *DeploymentState, op ServiceOperation) error {
	if state.ServiceOperations == nil {
		state.ServiceOperations = []ServiceOperation{}
	}
	state.ServiceOperations = append(state.ServiceOperations, op)
	return sm.SaveState(state)
}

// InitializeCleanupProgress initializes cleanup progress tracking
func (sm *StateManager) InitializeCleanupProgress(state *DeploymentState) error {
	if state.CleanupProgress == nil {
		state.CleanupProgress = &CleanupProgress{
			StartedAt:         time.Now().Format(time.RFC3339),
			Status:            "in_progress",
			PowerOffCompleted: []string{},
			ISOUnmapped:       []string{},
			NFSUnmounted:      []string{},
			ServicesRemoved:   []string{},
		}
		return sm.SaveState(state)
	}
	return nil
}

// RecordNodePowerOff records that a node was powered off during cleanup
func (sm *StateManager) RecordNodePowerOff(state *DeploymentState, nodeName string) error {
	if state.CleanupProgress == nil {
		sm.InitializeCleanupProgress(state)
	}

	// Check if already recorded
	for _, n := range state.CleanupProgress.PowerOffCompleted {
		if n == nodeName {
			return nil
		}
	}

	state.CleanupProgress.PowerOffCompleted = append(state.CleanupProgress.PowerOffCompleted, nodeName)
	return sm.SaveState(state)
}

// RecordISOUnmapped records that an ISO was unmapped from a node
func (sm *StateManager) RecordISOUnmapped(state *DeploymentState, nodeName string) error {
	if state.CleanupProgress == nil {
		sm.InitializeCleanupProgress(state)
	}

	// Check if already recorded
	for _, n := range state.CleanupProgress.ISOUnmapped {
		if n == nodeName {
			return nil
		}
	}

	state.CleanupProgress.ISOUnmapped = append(state.CleanupProgress.ISOUnmapped, nodeName)
	return sm.SaveState(state)
}

// RecordNFSUnmounted records that NFS was unmounted from a VIOS
func (sm *StateManager) RecordNFSUnmounted(state *DeploymentState, viosUUID string) error {
	if state.CleanupProgress == nil {
		sm.InitializeCleanupProgress(state)
	}

	// Check if already recorded
	for _, v := range state.CleanupProgress.NFSUnmounted {
		if v == viosUUID {
			return nil
		}
	}

	state.CleanupProgress.NFSUnmounted = append(state.CleanupProgress.NFSUnmounted, viosUUID)
	return sm.SaveState(state)
}

// RecordServiceRemoved records that a service was removed during cleanup
func (sm *StateManager) RecordServiceRemoved(state *DeploymentState, serviceName string) error {
	if state.CleanupProgress == nil {
		sm.InitializeCleanupProgress(state)
	}

	// Check if already recorded
	for _, s := range state.CleanupProgress.ServicesRemoved {
		if s == serviceName {
			return nil
		}
	}

	state.CleanupProgress.ServicesRemoved = append(state.CleanupProgress.ServicesRemoved, serviceName)
	return sm.SaveState(state)
}

// RecordVIPRemoved records that the VIP was removed during cleanup
func (sm *StateManager) RecordVIPRemoved(state *DeploymentState) error {
	if state.CleanupProgress == nil {
		sm.InitializeCleanupProgress(state)
	}

	state.CleanupProgress.VIPRemoved = true
	return sm.SaveState(state)
}

// IsNodePoweredOff checks if a node was already powered off during cleanup
func (sm *StateManager) IsNodePoweredOff(state *DeploymentState, nodeName string) bool {
	if state.CleanupProgress == nil {
		return false
	}
	for _, n := range state.CleanupProgress.PowerOffCompleted {
		if n == nodeName {
			return true
		}
	}
	return false
}

// IsISOUnmapped checks if an ISO was already unmapped from a node
func (sm *StateManager) IsISOUnmapped(state *DeploymentState, nodeName string) bool {
	if state.CleanupProgress == nil {
		return false
	}
	for _, n := range state.CleanupProgress.ISOUnmapped {
		if n == nodeName {
			return true
		}
	}
	return false
}

// IsNFSUnmounted checks if NFS was already unmounted from a VIOS
func (sm *StateManager) IsNFSUnmounted(state *DeploymentState, viosUUID string) bool {
	if state.CleanupProgress == nil {
		return false
	}
	for _, v := range state.CleanupProgress.NFSUnmounted {
		if v == viosUUID {
			return true
		}
	}
	return false
}

// IsServiceRemoved checks if a service was already removed
func (sm *StateManager) IsServiceRemoved(state *DeploymentState, serviceName string) bool {
	if state.CleanupProgress == nil {
		return false
	}
	for _, s := range state.CleanupProgress.ServicesRemoved {
		if s == serviceName {
			return true
		}
	}
	return false
}

// IsVIPRemoved checks if the VIP was already removed
func (sm *StateManager) IsVIPRemoved(state *DeploymentState) bool {
	if state.CleanupProgress == nil {
		return false
	}
	return state.CleanupProgress.VIPRemoved
}

// ValidateState performs integrity checks on the deployment state
func (sm *StateManager) ValidateState(state *DeploymentState) []string {
	var issues []string

	// Check StateVersion
	if state.StateVersion == 0 {
		issues = append(issues, "StateVersion is not set (should be 2)")
	}

	// Check ClusterName
	if state.ClusterName == "" {
		issues = append(issues, "ClusterName is empty")
	}

	// Check Status validity
	validStatuses := []string{"in_progress", "completed", "failed", "deleted"}
	statusValid := false
	for _, s := range validStatuses {
		if state.Status == s {
			statusValid = true
			break
		}
	}
	if !statusValid {
		issues = append(issues, fmt.Sprintf("Invalid status: %s", state.Status))
	}

	// Check StartTime is set
	if state.StartTime == "" {
		issues = append(issues, "StartTime is not set")
	}

	// Check completed phases are valid
	validPhases := []string{"discovery", "downloads", "services", "ignition", "boot", "wait", "iso_cleanup"}
	for _, phase := range state.CompletedPhases {
		phaseValid := false
		for _, vp := range validPhases {
			if phase == vp || strings.HasPrefix(phase, "booted_") {
				phaseValid = true
				break
			}
		}
		if !phaseValid {
			issues = append(issues, fmt.Sprintf("Unknown completed phase: %s", phase))
		}
	}

	// NOTE: ISO mappings and NFS mounts are intentionally left in state after deployment
	// completion because cleanup is deferred to the 'shiftlaunch delete' command.
	// This allows the cluster to remain operational for debugging and Day-2 operations.

	// Check cleanup progress consistency
	if state.CleanupProgress != nil {
		if state.Status != "deleted" && state.CleanupProgress.Status == "completed" {
			issues = append(issues, "CleanupProgress is completed but state is not deleted")
		}
	}

	// Check for duplicate completed events
	eventMap := make(map[string]bool)
	for _, event := range state.CompletedEvents {
		if eventMap[event] {
			issues = append(issues, fmt.Sprintf("Duplicate completed event: %s", event))
		}
		eventMap[event] = true
	}

	return issues
}

// RecoverState attempts to recover from an inconsistent state
func (sm *StateManager) RecoverState(state *DeploymentState) error {
	sm.logger.Debug("Attempting state recovery...")

	// Set StateVersion if missing
	if state.StateVersion == 0 {
		state.StateVersion = 2
		sm.logger.Debug("Set StateVersion to 2")
	}

	// Initialize nil maps
	if state.DownloadProgress == nil {
		state.DownloadProgress = make(map[string]DownloadProgress)
		sm.logger.Debug("Initialized DownloadProgress map")
	}

	if state.NodeBootStatus == nil {
		state.NodeBootStatus = make(map[string]NodeBootStatus)
		sm.logger.Debug("Initialized NodeBootStatus map")
	}

	// Initialize nil slices
	if state.CompletedEvents == nil {
		state.CompletedEvents = []string{}
		sm.logger.Debug("Initialized CompletedEvents slice")
	}

	if state.FailedEvents == nil {
		state.FailedEvents = []string{}
		sm.logger.Debug("Initialized FailedEvents slice")
	}

	// Remove duplicate completed events
	uniqueEvents := make(map[string]bool)
	cleanedEvents := []string{}
	for _, event := range state.CompletedEvents {
		if !uniqueEvents[event] {
			uniqueEvents[event] = true
			cleanedEvents = append(cleanedEvents, event)
		}
	}
	if len(cleanedEvents) != len(state.CompletedEvents) {
		duplicateCount := len(state.CompletedEvents) - len(cleanedEvents)
		state.CompletedEvents = cleanedEvents
		sm.logger.Debug("Removed duplicate completed events", "count", duplicateCount)
	}

	// Fix status if inconsistent
	if state.Status == "" {
		if state.EndTime != "" {
			state.Status = "completed"
		} else {
			state.Status = "in_progress"
		}
		sm.logger.Debug("Fixed missing status", "status", state.Status)
	}

	// Save recovered state
	if err := sm.SaveState(state); err != nil {
		return fmt.Errorf("failed to save recovered state: %w", err)
	}

	sm.logger.Debug("State recovery completed successfully")
	return nil
}

// CreateStateBackup creates a timestamped backup of the current state
func (sm *StateManager) CreateStateBackup() error {
	statePath := sm.GetStatePath()

	// Check if state file exists
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		return fmt.Errorf("state file does not exist: %s", statePath)
	}

	// Read current state
	data, err := os.ReadFile(statePath)
	if err != nil {
		return fmt.Errorf("failed to read state file: %w", err)
	}

	// Create backup with timestamp
	timestamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("%s.backup-%s", statePath, timestamp)

	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	return nil
}

// RestoreStateFromBackup restores state from the most recent backup
func (sm *StateManager) RestoreStateFromBackup() error {
	stateDir := sm.workspaceDir

	// Find all backup files
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return fmt.Errorf("failed to read workspace directory: %w", err)
	}

	var backupFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "state.json.backup-") {
			backupFiles = append(backupFiles, entry.Name())
		}
	}

	if len(backupFiles) == 0 {
		return fmt.Errorf("no backup files found")
	}

	// Sort to get most recent (lexicographic sort works with our timestamp format)
	sort.Strings(backupFiles)
	mostRecent := backupFiles[len(backupFiles)-1]

	// Read backup
	backupPath := filepath.Join(stateDir, mostRecent)
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup file: %w", err)
	}

	// Write to state file
	statePath := sm.GetStatePath()
	if err := os.WriteFile(statePath, data, 0600); err != nil {
		return fmt.Errorf("failed to restore state file: %w", err)
	}

	return nil
}

// GetStateHistory returns a summary of state changes over time
func (sm *StateManager) GetStateHistory(state *DeploymentState) string {
	var history strings.Builder

	history.WriteString(fmt.Sprintf("Cluster: %s\n", state.ClusterName))
	history.WriteString(fmt.Sprintf("State Version: %d\n", state.StateVersion))
	history.WriteString(fmt.Sprintf("Status: %s\n", state.Status))
	history.WriteString(fmt.Sprintf("Started: %s\n", state.StartTime))

	if state.EndTime != "" {
		history.WriteString(fmt.Sprintf("Ended: %s\n", state.EndTime))
	}

	if state.ResumeCount > 0 {
		history.WriteString(fmt.Sprintf("Resumed: %d times\n", state.ResumeCount))
	}

	history.WriteString(fmt.Sprintf("\nCompleted Phases: %d\n", len(state.CompletedPhases)))
	for _, phase := range state.CompletedPhases {
		history.WriteString(fmt.Sprintf("  - %s\n", phase))
	}

	history.WriteString(fmt.Sprintf("\nCompleted Events: %d\n", len(state.CompletedEvents)))
	if len(state.CompletedEvents) > 0 {
		// Show first 10 and last 10
		showCount := 10
		if len(state.CompletedEvents) <= showCount*2 {
			for _, event := range state.CompletedEvents {
				history.WriteString(fmt.Sprintf("  - %s\n", event))
			}
		} else {
			for i := 0; i < showCount; i++ {
				history.WriteString(fmt.Sprintf("  - %s\n", state.CompletedEvents[i]))
			}
			history.WriteString(fmt.Sprintf("  ... (%d more) ...\n", len(state.CompletedEvents)-showCount*2))
			for i := len(state.CompletedEvents) - showCount; i < len(state.CompletedEvents); i++ {
				history.WriteString(fmt.Sprintf("  - %s\n", state.CompletedEvents[i]))
			}
		}
	}

	if len(state.FailedEvents) > 0 {
		history.WriteString(fmt.Sprintf("\nFailed Events: %d\n", len(state.FailedEvents)))
		for _, event := range state.FailedEvents {
			history.WriteString(fmt.Sprintf("  - %s\n", event))
		}
	}

	if len(state.PhaseHistory) > 0 {
		history.WriteString(fmt.Sprintf("\nPhase Execution History: %d\n", len(state.PhaseHistory)))
		for _, phase := range state.PhaseHistory {
			history.WriteString(fmt.Sprintf("  - %s: %s (%s)\n", phase.Phase, phase.Status, phase.Duration))
		}
	}

	return history.String()
}
