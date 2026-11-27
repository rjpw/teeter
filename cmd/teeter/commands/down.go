package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rjpw/teeter/internal/manifest"
	"github.com/rjpw/teeter/internal/project"
	"github.com/spf13/cobra"
)

func NewDownCmd() *cobra.Command {
	var (
		manifestPath string
		volumes      bool
		rmi          string
		timeout      time.Duration
	)
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop the server stack",
		Long:  `Stop and remove all services.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// Resolve manifest path
			resolvedManifestPath, err := ResolveManifestPath(cmd, manifestPath)
			if err != nil {
				return err
			}
			manifestPath = resolvedManifestPath

			// Load manifest
			m, err := manifest.ParseFile(manifestPath)
			if err != nil {
				return err
			}

			manifestDir := filepath.Dir(manifestPath)
			if manifestDir == "." {
				manifestDir, _ = os.Getwd()
			}
			if err := manifest.Validate(m, manifestDir); err != nil {
				return err
			}

			// Find build context
			finder := &BuildContextFinder{
				ManifestDir: manifestDir,
				Manifest:    m,
			}
			bc, err := finder.Find()
			if err != nil {
				return fmt.Errorf("find build context: %w", err)
			}

			composeFile := GetComposeFile(bc)
			if _, err := os.Stat(composeFile); os.IsNotExist(err) {
				return fmt.Errorf("compose file not found: %s", composeFile)
			}

			if timeout == 0 {
				timeout = 30 * time.Second
			}

			// docker compose down
			fmt.Println("Stopping services...")
			downArgs := []string{"down", "--timeout", fmt.Sprintf("%d", int(timeout.Seconds()))}
			if volumes {
				downArgs = append(downArgs, "-v")
			}
			if rmi != "" {
				downArgs = append(downArgs, "--rmi", rmi)
			}
			downCmd := newDockerCommand(ctx, manifestDir, m.Project.Name, composeFile, downArgs...)
			downCmd.Stdout = os.Stdout
			downCmd.Stderr = os.Stderr
			if err := downCmd.Run(); err != nil {
				return fmt.Errorf("docker compose down: %w", err)
			}

			fmt.Println("Services stopped.")

			// Release project lock
			if unlock, err := project.AcquireLock(manifestDir); err == nil {
				_ = unlock()
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to manifest file (default: teeter.yaml or .teeter/manifest.yaml)")
	cmd.Flags().BoolVarP(&volumes, "volumes", "v", false, "Remove named volumes (mysql-data, extracted-data, config)")
	cmd.Flags().StringVar(&rmi, "rmi", "", "Remove images: 'local' or 'all'")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Stop timeout")

	return cmd
}
