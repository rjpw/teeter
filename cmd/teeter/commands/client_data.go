package commands

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rjpw/teeter/internal/manifest"
	"github.com/spf13/cobra"
)

func NewClientDataCmd() *cobra.Command {
	var (
		manifestPath string
		force        bool
	)

	cmd := &cobra.Command{
		Use:   "client-data",
		Short: "Manage client data (download pre-extracted maps/vmaps/mmaps)",
		Long:  `Download and prepare pre-extracted client data from wowgaming/client-data releases.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return nil
		},
	}

	cmd.AddCommand(newClientDataPullCmd(&manifestPath, &force))

	return cmd
}

func newClientDataPullCmd(manifestPath *string, force *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Download and extract pre-extracted client data to Docker volume",
		Long: `Download Data.zip from wowgaming/client-data GitHub releases and extract
to the 'extracted-data' Docker volume used by the server stack.

The version is read from manifest.client_data.version (e.g., "v20.0").`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// Resolve manifest path
			resolvedManifestPath, err := ResolveManifestPath(cmd, *manifestPath)
			if err != nil {
				return err
			}

			// Load manifest
			m, err := manifest.ParseFile(resolvedManifestPath)
			if err != nil {
				return err
			}

			manifestDir := filepath.Dir(resolvedManifestPath)
			if manifestDir == "." {
				manifestDir, _ = os.Getwd()
			}
			if err := manifest.Validate(m, manifestDir); err != nil {
				return err
			}

			// Validate client_data config
			if m.ClientData.Source != "static" {
				return fmt.Errorf("client_data.source must be 'static' for pull; got %q", m.ClientData.Source)
			}
			if m.ClientData.Version == "" {
				return fmt.Errorf("client_data.version is required for static source")
			}

			version := m.ClientData.Version
			if !strings.HasPrefix(version, "v") {
				version = "v" + version
			}

			url := fmt.Sprintf("https://github.com/wowgaming/client-data/releases/download/%s/Data.zip", version)

			// Get Docker volume name for extracted-data
			volumeName := m.Project.Name + "_extracted-data"

			// Check if already populated
			if !(*force) && isVolumePopulated(ctx, volumeName) {
				volumePath, _ := getExtractedDataVolumePath(ctx, m.Project.Name)
				fmt.Printf("extracted-data volume already populated at %s (use --force to re-download)\n", volumePath)
				return nil
			}

			// Download and extract using Docker container
			if err := downloadAndExtractViaContainer(ctx, url, volumeName); err != nil {
				return fmt.Errorf("download and extract: %w", err)
			}

			fmt.Printf("Client data %s extracted to volume %s\n", version, volumeName)
			fmt.Println("Run 'teeter up' to start the server with client data mounted.")
			return nil

		},
	}

	cmd.Flags().StringVar(manifestPath, "manifest", "", "Path to manifest file (default: teeter.yaml or .teeter/manifest.yaml)")
	cmd.Flags().BoolVar(force, "force", false, "Re-download even if volume already populated")

	return cmd
}

func getExtractedDataVolumePath(ctx context.Context, projectName string) (string, error) {
	// Docker volume name follows pattern: <project>_extracted-data
	// Use docker volume inspect to get the mountpoint
	volumeName := projectName + "_extracted-data"

	cmd := exec.CommandContext(ctx, "docker", "volume", "inspect", volumeName, "--format", "{{.Mountpoint}}")
	output, err := cmd.Output()
	if err != nil {
		// Volume might not exist yet - create it
		fmt.Printf("  Creating Docker volume %s...\n", volumeName)
		createCmd := exec.CommandContext(ctx, "docker", "volume", "create", volumeName)
		if err := createCmd.Run(); err != nil {
			return "", fmt.Errorf("create volume: %w", err)
		}
		// Inspect again
		cmd = exec.CommandContext(ctx, "docker", "volume", "inspect", volumeName, "--format", "{{.Mountpoint}}")
		output, err = cmd.Output()
		if err != nil {
			return "", fmt.Errorf("inspect volume after create: %w", err)
		}
	}

	mountpoint := strings.TrimSpace(string(output))
	if mountpoint == "" {
		return "", fmt.Errorf("empty mountpoint for volume %s", volumeName)
	}
	return mountpoint, nil
}

func isVolumePopulated(ctx context.Context, volumeName string) bool {
	// Use Docker container to check volume contents (avoids permission issues)
	args := []string{
		"run", "--rm",
		"-v", volumeName + ":/data",
		"alpine:latest",
		"sh", "-c",
		"test -d /data/maps && test -n \"$(ls -A /data/maps)\" && test -d /data/vmaps && test -n \"$(ls -A /data/vmaps)\"",
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	return cmd.Run() == nil
}

func downloadAndExtractViaContainer(ctx context.Context, url, volumeName string) error {
	// Create temp file for download
	tmpFile, err := os.CreateTemp("", "client-data-*.zip")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Download with progress
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	// Write to temp file with progress
	fmt.Printf("  Downloading (%.1f MB)... ", float64(resp.ContentLength)/1024/1024)
	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		return fmt.Errorf("write download: %w", err)
	}
	fmt.Println("done")

	// Extract using Docker container with volume mounted
	fmt.Println("  Extracting via Docker container...")
	return extractViaContainer(ctx, tmpPath, volumeName)
}

func extractViaContainer(ctx context.Context, zipPath, volumeName string) error {
	// Use alpine with unzip to extract to the Docker volume
	// Mount the volume at /data and the zip at /data.zip
	absZipPath, err := filepath.Abs(zipPath)
	if err != nil {
		return fmt.Errorf("abs zip path: %w", err)
	}

	args := []string{
		"run", "--rm",
		"-v", volumeName + ":/data",
		"-v", absZipPath + ":/data.zip:ro",
		"alpine:latest",
		"sh", "-c",
		"cd /data && unzip -q -o /data.zip 'maps/*' 'vmaps/*' 'mmaps/*'",
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker extract: %w", err)
	}

	// Verify extraction
	if !isVolumePopulated(ctx, volumeName) {
		return fmt.Errorf("extraction verification failed: volume not populated")
	}

	return nil
}
