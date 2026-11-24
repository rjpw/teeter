package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rjpw/teeter/internal/buildctx"
	"github.com/rjpw/teeter/internal/cache"
	"github.com/rjpw/teeter/internal/catalogue"
	"github.com/rjpw/teeter/internal/compose"
	"github.com/rjpw/teeter/internal/config"
	"github.com/rjpw/teeter/internal/manifest"
	"github.com/spf13/cobra"
)

func NewComposeCmd() *cobra.Command {
	var (
		manifestPath  string
		outputFile    string
		profile       string
		buildStrategy string
		buildMemory   string
		buildCPUs     string
		buildJobs     int
		ccacheMaxSize string
		registry      string
		dryRun        bool
	)

	cmd := &cobra.Command{
		Use:   "compose",
		Short: "Generate docker-compose.yml from manifest and build context",
		Long:  `Generate docker-compose.yml using the layered build strategy.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

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

			// Apply CLI flags over manifest/config
			if buildStrategy == "" {
				buildStrategy = m.Compose.BuildStrategy
			}
			if buildStrategy == "" {
				buildStrategy = "prebuilt"
			}

			if buildMemory == "" {
				buildMemory = m.Compose.BuildMemory
			}
			if buildMemory == "" {
				buildMemory = "4G"
			}

			if buildCPUs == "" {
				buildCPUs = m.Compose.BuildCPUs
			}
			if buildCPUs == "" {
				buildCPUs = "2"
			}

			if buildJobs == 0 {
				buildJobs = m.Compose.BuildJobs
			}
			if buildJobs == 0 {
				buildJobs = 2
			}

			if ccacheMaxSize == "" {
				ccacheMaxSize = m.Compose.CCacheMaxSize
			}
			if ccacheMaxSize == "" {
				ccacheMaxSize = "1G"
			}

			if registry == "" {
				registry = m.Compose.Registry
			}
			if registry == "" {
				registry = "teeter"
			}

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

			// Write env files to build context
			if err := writeEnvFiles(bc.Path, envFiles); err != nil {
				return fmt.Errorf("write env files: %w", err)
			}

			// Generate SQL files
			sqlFiles, err := compose.GenerateSQLFiles(ctx, bc, c, m)
			if err != nil {
				return fmt.Errorf("generate SQL files: %w", err)
			}

			// Write SQL files to build context
			if err := writeSQLFiles(bc.Path, sqlFiles); err != nil {
				return fmt.Errorf("write SQL files: %w", err)
			}

			// Generate Dockerfile
			dockerfileData := compose.DockerfileData{
				BuildJobs:     buildJobs,
				CCacheMaxSize: ccacheMaxSize,
				BuildMemory:   buildMemory,
				BuildCPUs:     buildCPUs,
				Registry:      registry,
				ACCommit:      res.ACCommit,
				ModulesHash:   bc.Hash,
				FullHash:      bc.Hash,
			}
			dockerfile, err := compose.GenerateDockerfile(dockerfileData)
			if err != nil {
				return fmt.Errorf("generate Dockerfile: %w", err)
			}

			// Write Dockerfile to build context
			if err := writeDockerfile(bc.Path, dockerfile); err != nil {
				return fmt.Errorf("write Dockerfile: %w", err)
			}

			// Generate services
			svcGen := compose.ServiceGenerator{
				BuildContext:  bc,
				Manifest:      m,
				Registry:      registry,
				BuildStrategy: buildStrategy,
			}

			services, err := svcGen.GenerateServices()
			if err != nil {
				return fmt.Errorf("generate services: %w", err)
			}

			networks := svcGen.GenerateNetworks()
			volumes := svcGen.GenerateVolumes()

			// Create compose config
			composeConfig := compose.ComposeConfig{
				Version:  "3.8",
				Services: services,
				Networks: networks,
				Volumes:  volumes,
			}

			// Marshal to YAML
			yamlData, err := marshalCompose(composeConfig)
			if err != nil {
				return fmt.Errorf("marshal compose: %w", err)
			}

			// Determine output file
			if outputFile == "" {
				outputFile = filepath.Join(bc.Path, "docker-compose.yml")
			}

			// Dry run: print to stdout
			if dryRun {
				fmt.Print(string(yamlData))
				return nil
			}

			// Write to file
			if err := os.WriteFile(outputFile, yamlData, 0644); err != nil {
				return fmt.Errorf("write compose file: %w", err)
			}

			// Print summary
			tags := svcGen.ComputeImageTags()
			fmt.Printf("Generating docker-compose.yml (%s)...\n", buildStrategy)
			fmt.Printf("  Builder:     %s\n", tags.Builder)
			fmt.Printf("  Modules:     %s\n", tags.Modules)
			fmt.Printf("  Runtime:     %s\n", tags.Runtime)
			fmt.Printf("  Services:    %d\n", len(services))
			fmt.Printf("  Profiles:    %v\n", getProfiles(services))
			fmt.Printf("  Volumes:     %d\n", len(volumes))
			fmt.Printf("  Networks:    %d\n", len(networks))
			fmt.Printf("Written to %s\n", outputFile)

			return nil
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to manifest file (default: teeter.yaml or .teeter/manifest.yaml)")
	cmd.Flags().StringVar(&outputFile, "output", "", "Output file (default: .teeter/buildctx/<hash>/docker-compose.yml)")
	cmd.Flags().StringVar(&profile, "profile", "core", "Comma-separated profiles (default: core)")
	cmd.Flags().StringVar(&buildStrategy, "build-strategy", "", "Build strategy: prebuilt|auto|local (default: prebuilt)")
	cmd.Flags().StringVar(&buildMemory, "build-memory", "", "Memory limit for builds (default: 4G)")
	cmd.Flags().StringVar(&buildCPUs, "build-cpus", "", "CPU limit for builds (default: 2)")
	cmd.Flags().IntVar(&buildJobs, "build-jobs", 0, "Parallel link jobs (default: 2)")
	cmd.Flags().StringVar(&ccacheMaxSize, "ccache-maxsize", "", "CCache max size (default: 1G)")
	cmd.Flags().StringVar(&registry, "registry", "", "Image registry prefix (default: teeter)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print compose to stdout without writing file")

	return cmd
}
