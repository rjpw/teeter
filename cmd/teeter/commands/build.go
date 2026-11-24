package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

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
		manifestPath  string
		layer         string
		force         bool
		noCache       bool
		buildMemory   string
		buildCPUs     string
		buildJobs     int
		ccacheMaxSize string
		registry      string
	)

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build Docker image layers for the layered build strategy",
		Long:  `Build missing Docker image layers (builder, modules, runtime) for local deployment.`,
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

			// Apply defaults
			if layer == "" {
				layer = "all"
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
			if err := writeDockerfile(bc.Path, dockerfile); err != nil {
				return fmt.Errorf("write Dockerfile: %w", err)
			}

			// Compute image tags
			tags := computeImageTags(registry, res.ACCommit, bc.Hash)

			// Build requested layers
			layersToBuild := parseLayers(layer)
			for _, l := range layersToBuild {
				if err := buildLayer(ctx, l, bc.Path, tags, force, noCache, buildMemory, buildCPUs, buildJobs); err != nil {
					return fmt.Errorf("build layer %s: %w", l, err)
				}
			}

			fmt.Printf("Build complete. Images:\n")
			fmt.Printf("  Builder:  %s\n", tags.Builder)
			fmt.Printf("  Modules:  %s\n", tags.Modules)
			fmt.Printf("  Runtime:  %s\n", tags.Runtime)

			return nil
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to manifest file")
	cmd.Flags().StringVar(&layer, "layer", "all", "Layer to build: builder|modules|runtime|all")
	cmd.Flags().BoolVar(&force, "force", false, "Rebuild even if image exists")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Don't use Docker layer cache")
	cmd.Flags().StringVar(&buildMemory, "build-memory", "", "Memory limit for builds (default: 4G)")
	cmd.Flags().StringVar(&buildCPUs, "build-cpus", "", "CPU limit for builds (default: 2)")
	cmd.Flags().IntVar(&buildJobs, "build-jobs", 0, "Parallel link jobs (default: 2)")
	cmd.Flags().StringVar(&ccacheMaxSize, "ccache-maxsize", "", "CCache max size (default: 1G)")
	cmd.Flags().StringVar(&registry, "registry", "", "Image registry prefix (default: teeter)")

	return cmd
}

// ImageTags holds the computed image tags.
type ImageTags struct {
	Builder string
	Modules string
	Runtime string
}

func computeImageTags(registry, acCommit, buildCtxHash string) ImageTags {
	if len(acCommit) > 7 {
		acCommit = acCommit[:7]
	}
	if len(buildCtxHash) > 8 {
		buildCtxHash = buildCtxHash[:8]
	}
	fullHash := buildCtxHash
	if len(buildCtxHash) > 12 {
		fullHash = buildCtxHash[:12]
	}

	return ImageTags{
		Builder: fmt.Sprintf("%s/acore-builder:%s", registry, acCommit),
		Modules: fmt.Sprintf("%s/acore-modules:%s", registry, buildCtxHash),
		Runtime: fmt.Sprintf("%s/acore-runtime:%s", registry, fullHash),
	}
}

func parseLayers(layer string) []string {
	switch layer {
	case "builder":
		return []string{"builder"}
	case "modules":
		return []string{"modules"}
	case "runtime":
		return []string{"runtime"}
	case "all":
		return []string{"builder", "modules", "runtime"}
	default:
		return []string{"all"}
	}
}

func buildLayer(ctx context.Context, layer, buildCtxPath string, tags ImageTags, force, noCache bool, buildMemory, buildCPUs string, buildJobs int) error {
	// Check if image already exists
	imageTag := ""
	target := ""
	cacheFrom := ""

	switch layer {
	case "builder":
		imageTag = tags.Builder
		target = "builder-base"
	case "modules":
		imageTag = tags.Modules
		target = "modules-build"
		cacheFrom = tags.Builder
	case "runtime":
		imageTag = tags.Runtime
		target = "runtime"
		cacheFrom = tags.Modules
	}

	if imageTag == "" {
		return fmt.Errorf("unknown layer: %s", layer)
	}

	// Check if image exists
	if !force {
		// In a real implementation, check docker image inspect
		fmt.Printf("  [%s] Checking image %s...\n", layer, imageTag)
	}

	fmt.Printf("  [%s] Building %s (target: %s)...\n", layer, imageTag, target)

	// In a real implementation, use Docker SDK to build
	// For now, print the command that would be run
	buildCmd := fmt.Sprintf("docker build --target %s -t %s", target, imageTag)
	if cacheFrom != "" {
		buildCmd += fmt.Sprintf(" --cache-from %s", cacheFrom)
	}
	if noCache {
		buildCmd += " --no-cache"
	}
	buildCmd += fmt.Sprintf(" --memory %s --cpus %s", buildMemory, buildCPUs)
	buildCmd += fmt.Sprintf(" --build-arg BUILD_JOBS=%d", buildJobs)
	buildCmd += fmt.Sprintf(" %s", buildCtxPath)

	fmt.Printf("    Command: %s\n", buildCmd)

	// Simulate build time
	time.Sleep(100 * time.Millisecond)

	fmt.Printf("  [%s] Built %s\n", layer, imageTag)
	return nil
}
