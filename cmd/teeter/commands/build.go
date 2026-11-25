package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/rjpw/teeter/internal/buildctx"
	"github.com/rjpw/teeter/internal/cache"
	"github.com/rjpw/teeter/internal/catalogue"
	"github.com/rjpw/teeter/internal/compose"
	"github.com/rjpw/teeter/internal/config"
	"github.com/rjpw/teeter/internal/manifest"
	"github.com/spf13/cobra"
)

func NewBuildCmd() *cobra.Command {
	var (
		manifestPath string
		force        bool
		noCache      bool
	)

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build the worldserver Docker image",
		Long:  `Build the custom worldserver image from the build context. Other services use upstream AC images.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

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

			// Initialize config
			cfg := config.Get()

			// Create cache
			c := cache.NewFSCache(cfg.CacheDir, cache.LinkStrategyFromString(cfg.LinkStrategy))

			// Load or fetch catalogue index
			idx, err := catalogue.LoadOrFetchIndex(ctx, cfg)
			if err != nil {
				return fmt.Errorf("load catalogue index: %w", err)
			}

			// Resolve modules
			res, err := catalogue.Resolve(ctx, m, idx, catalogue.DefaultResolveOptions())
			if err != nil {
				return fmt.Errorf("resolve modules: %w", err)
			}

			// Assemble build context
			workDir := filepath.Dir(manifestDir)
			bc, err := buildctx.Assemble(ctx, m, &res, c, workDir)
			if err != nil {
				return fmt.Errorf("assemble build context: %w", err)
			}

			// Generate env files
			envFiles, err := compose.GenerateEnvFiles(bc, m)
			if err != nil {
				return fmt.Errorf("generate env files: %w", err)
			}
			if err := writeEnvFiles(bc.Path, envFiles); err != nil {
				return fmt.Errorf("write env files: %w", err)
			}

			// Generate SQL files
			sqlFiles, err := compose.GenerateSQLFiles(ctx, bc, c, m)
			if err != nil {
				return fmt.Errorf("generate SQL files: %w", err)
			}
			if err := writeSQLFiles(bc.Path, sqlFiles); err != nil {
				return fmt.Errorf("write SQL files: %w", err)
			}

			// Build worldserver image
			worldserverImage := fmt.Sprintf("teeter/acore-worldserver:%s", bc.Hash[:8])
			fmt.Printf("Building worldserver image: %s\n", worldserverImage)
			fmt.Printf("Build context: %s\n", bc.Path)

			// Use docker compose build for the worldserver service
			// We need to generate a minimal compose file with just the worldserver service
			svcGen := compose.ServiceGenerator{
				BuildContext: bc,
				Manifest:     m,
			}

			services, err := svcGen.GenerateServices()
			if err != nil {
				return fmt.Errorf("generate services: %w", err)
			}

			// Filter to only worldserver
			worldserverSvc := services["ac-worldserver"]
			composeConfig := compose.ComposeConfig{
				Version:  "3.8",
				Services: map[string]compose.Service{"ac-worldserver": worldserverSvc},
				Networks: svcGen.GenerateNetworks(),
				Volumes:  svcGen.GenerateVolumes(),
			}

			yamlData, merr := marshalCompose(composeConfig)
			if merr != nil {
				return fmt.Errorf("marshal compose: %w", merr)
			}

			composeFile := filepath.Join(bc.Path, "docker-compose.build.yml")
			if err := os.WriteFile(composeFile, yamlData, 0644); err != nil {
				return fmt.Errorf("write compose file: %w", err)
			}

			// Build using docker compose
			dockerArgs := []string{"compose", "-f", composeFile, "build"}
			if force {
				dockerArgs = append(dockerArgs, "--no-cache")
			}
			if noCache {
				dockerArgs = append(dockerArgs, "--no-cache")
			}

			buildCmd := exec.CommandContext(ctx, "docker", dockerArgs...)
			buildCmd.Dir = bc.Path
			buildCmd.Stdout = os.Stdout
			buildCmd.Stderr = os.Stderr
			if err := buildCmd.Run(); err != nil {
				return fmt.Errorf("docker compose build: %w", err)
			}

			fmt.Printf("Build complete: %s\n", worldserverImage)
			return nil
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to manifest file")
	cmd.Flags().BoolVar(&force, "force", false, "Rebuild even if image exists (implies --no-cache)")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Don't use Docker layer cache")

	return cmd
}
