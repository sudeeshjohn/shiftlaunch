package localexec

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sudeeshjohn/shiftlaunch/logger"
)

type LocalClient struct {
	logger *logger.Logger
}

func NewLocalClient(log *logger.Logger) *LocalClient {
	return &LocalClient{logger: log}
}

// Execute runs a bash command locally and returns stdout/stderr
func (l *LocalClient) Execute(command string) (string, error) {
	if l.logger != nil {
        l.logger.Debug("Executing local command", "command", command)
    }
	l.logger.Debug("Executing local command", "cmd", command)

	cmd := exec.Command("bash", "-c", command)
	output, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(output))

	if err != nil {
		l.logger.Debug("Command failed", "cmd", command, "error", err, "output", outStr)
		return outStr, fmt.Errorf("local execution failed: %w (output: %s)", err, outStr)
	}

	l.logger.Debug("Command succeeded", "output", outStr)
	return outStr, nil
}

// WriteFile writes content directly to the local filesystem (with sudo if needed)
func (l *LocalClient) WriteFile(path string, content []byte, perms os.FileMode) error {
	l.logger.Debug("Writing local file", "path", path)

	// Create temp file
	tmpPath := filepath.Join("/tmp", filepath.Base(path)+".tmp")
	if err := os.WriteFile(tmpPath, content, perms); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Move into place with sudo (required for /etc/ directories)
	mvCmd := fmt.Sprintf("sudo mv %s %s && sudo chmod %04o %s", tmpPath, path, perms, path)
	if _, err := l.Execute(mvCmd); err != nil {
		return fmt.Errorf("failed to move file into place: %w", err)
	}

	return nil
}

func (l *LocalClient) SystemctlRestart(service string) error {
	l.logger.Info("Restarting local service", "service", service)
	_, err := l.Execute(fmt.Sprintf("sudo systemctl restart %s", service))
	return err
}

func (l *LocalClient) SystemctlEnable(service string) error {
	_, err := l.Execute(fmt.Sprintf("sudo systemctl enable --now %s", service))
	return err
}