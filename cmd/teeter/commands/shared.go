package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rjpw/teeter/internal/manifest"
)

// listBuildContexts returns all available build context hashes.
func listBuildContexts(workDir string) ([]string, error) {
	buildctxDir := filepath.Join(workDir, ".teeter", "buildctx")
	entries, err := os.ReadDir(buildctxDir)
	if err != nil {
		return nil, err
	}
	var hashes []string
	for _, e := range entries {
		if e.IsDir() {
			hashes = append(hashes, e.Name())
		}
	}
	return hashes, nil
}

// ensureDatabaseRunning checks if the database container is running and healthy.
func ensureDatabaseRunning(ctx context.Context, workDir, composeFile string) error {
	args := []string{"compose", "-f", composeFile, "--profile", "core", "ps", "--format", "json", "ac-database"}
	cmd := newDockerCommand(ctx, workDir, args...)
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || outputStr == "[]" {
		return fmt.Errorf("database container not found")
	}

	if !strings.Contains(outputStr, "\"State\":\"running\"") {
		return fmt.Errorf("database not running")
	}

	// Check health
	healthArgs := []string{"compose", "-f", composeFile, "--profile", "core", "exec", "-T", "ac-database", "sh", "-c", "mysqladmin ping -h localhost -u root -p$${MYSQL_ROOT_PASSWORD}"}
	healthCmd := newDockerCommand(ctx, workDir, healthArgs...)
	return healthCmd.Run()
}

// readMySQLRootPassword reads the MySQL root password from an .env file.
func readMySQLRootPassword(envFile string) (string, error) {
	data, err := os.ReadFile(envFile)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "MYSQL_ROOT_PASSWORD=") {
			return strings.TrimPrefix(line, "MYSQL_ROOT_PASSWORD="), nil
		}
	}
	return "", fmt.Errorf("MYSQL_ROOT_PASSWORD not found in %s", envFile)
}

// getClientDataMetadata returns client data metadata as a string.
func getClientDataMetadata(workDir string, m *manifest.Manifest) (string, error) {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("ClientDataSource: %s\n", m.ClientData.Source))
	b.WriteString(fmt.Sprintf("ClientDataVersion: %s\n", m.ClientData.Version))
	b.WriteString(fmt.Sprintf("ClientDataPath: %s\n", m.ClientData.WoWClientMapsPath))

	// Check extracted data volume
	volumeName := fmt.Sprintf("teeter-%s-extracted-data", m.Project.Name)
	b.WriteString(fmt.Sprintf("ExtractedDataVolume: %s\n", volumeName))

	return b.String(), nil
}

// writeCompressed writes content to a file (placeholder for gzip compression).
func writeCompressed(path, content string) error {
	// Simple gzip compression using gzip command
	// For now, write uncompressed - can add gzip later
	return os.WriteFile(path, []byte(content), 0644)
}

// formatBytes formats byte count as human-readable string.
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
