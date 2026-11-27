package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rjpw/teeter/internal/acore"
	"github.com/rjpw/teeter/internal/buildctx"
	"github.com/rjpw/teeter/internal/cache"
	"github.com/rjpw/teeter/internal/catalogue"
	"github.com/rjpw/teeter/internal/config"
	"github.com/rjpw/teeter/internal/manifest"
	"github.com/spf13/cobra"
)

func NewBuildCtxCmd() *cobra.Command {
	var (
		manifestPath string
		outputDir    string
		noPatches    bool
		noSQL        bool
		dryRun       bool
	)

	cmd := &cobra.Command{
		Use:   "buildctx",
		Short: "Assemble build context from manifest and cache",
		Long:  `Assemble build context by linking AC source and modules from cache, applying patches, and assembling SQL files.`,
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

			// Validate manifest
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

			// Fetch AC source
			fmt.Printf("Fetching AC source...\n")
			acPath, err := acore.FetchACore(ctx, cfg, res.ACCommit, res.ACRepo, c)
			if err != nil {
				return fmt.Errorf("fetch AC source: %w", err)
			}
			fmt.Printf("  AC: %s (at %s)\n", res.ACCommit[:7], acPath)

			// Determine work directory
			workDir := filepath.Dir(filepath.Dir(manifestPath)) // .teeter is in workDir
			if outputDir != "" {
				workDir = filepath.Dir(outputDir)
			}

			// Assemble build context
			fmt.Printf("Assembling build context...\n")
			bc, err := buildctx.Assemble(ctx, m, &res, c, workDir)
			if err != nil {
				return fmt.Errorf("assemble build context: %w", err)
			}

			// Apply patches
			if !noPatches {
				fmt.Printf("Applying patches...\n")
				if err := buildctx.ApplyPatches(ctx, bc); err != nil {
					return fmt.Errorf("apply patches: %w", err)
				}
			} else {
				fmt.Printf("Skipping patches (--no-patches)\n")
			}

			// Assemble SQL
			if !noSQL {
				fmt.Printf("Assembling SQL...\n")
				if err := buildctx.AssembleSQL(ctx, bc, c); err != nil {
					return fmt.Errorf("assemble SQL: %w", err)
				}
			} else {
				fmt.Printf("Skipping SQL assembly (--no-sql)\n")
			}

			fmt.Printf("Build context ready at %s\n", bc.Path)
			return nil
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to manifest file (default: teeter.yaml or .teeter/manifest.yaml)")
	cmd.Flags().StringVar(&outputDir, "output", "", "Output directory (default: .teeter/buildctx/<hash>/)")
	cmd.Flags().BoolVar(&noPatches, "no-patches", false, "Skip patch application")
	cmd.Flags().BoolVar(&noSQL, "no-sql", false, "Skip SQL assembly")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without executing")

	return cmd
}
