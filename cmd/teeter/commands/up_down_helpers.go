package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rjpw/teeter/internal/buildctx"
	"github.com/rjpw/teeter/internal/cache"
	"github.com/rjpw/teeter/internal/catalogue"
	"github.com/rjpw/teeter/internal/config"
	"github.com/rjpw/teeter/internal/manifest"
)

// BuildContextFinder finds the build context for a project
type BuildContextFinder struct {
	ManifestDir string
	Manifest    *manifest.Manifest
}

// Find finds the build context, assembling if necessary
func (f *BuildContextFinder) Find() (*buildctx.BuildContext, error) {
	// Look for .teeter/buildctx/<hash> directories
	buildctxDir := filepath.Join(f.ManifestDir, ".teeter", "buildctx")
	entries, err := os.ReadDir(buildctxDir)
	if err != nil {
		// Try to assemble build context to get the hash
		return f.assemble()
	}

	// Find the most recent valid build context
	var latest *buildctx.BuildContext
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		hash := entry.Name()
		bcPath := filepath.Join(buildctxDir, hash)
		composeFile := filepath.Join(bcPath, "docker-compose.yml")
		if _, err := os.Stat(composeFile); err == nil {
			bc := &buildctx.BuildContext{
				Path: bcPath,
				Hash: hash,
			}
			if latest == nil {
				latest = bc
			}
		}
	}

	if latest != nil {
		return latest, nil
	}

	return f.assemble()
}

func (f *BuildContextFinder) assemble() (*buildctx.BuildContext, error) {
	ctx := context.Background()
	cfg := config.Get()
	c := cache.NewFSCache(cfg.CacheDir, cache.LinkStrategyFromString(cfg.LinkStrategy))
	idx, err := catalogue.LoadOrFetchIndex(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("load catalogue index: %w", err)
	}
	res, err := catalogue.Resolve(ctx, f.Manifest, idx, catalogue.DefaultResolveOptions())
	if err != nil {
		return nil, fmt.Errorf("resolve modules: %w", err)
	}
	workDir := filepath.Dir(f.ManifestDir)
	bc, err := buildctx.Assemble(ctx, f.Manifest, &res, c, workDir)
	if err != nil {
		return nil, fmt.Errorf("assemble build context: %w", err)
	}
	return bc, nil
}

// GetProfileArgs returns the profile arguments for docker compose
func GetProfileArgs(m *manifest.Manifest) []string {
	profiles := strings.Join(m.Compose.Profiles, ",")
	if profiles == "" {
		profiles = "core"
	}
	profileArgs := []string{}
	for _, p := range strings.Split(profiles, ",") {
		profileArgs = append(profileArgs, "--profile", strings.TrimSpace(p))
	}
	return profileArgs
}

// GetComposeFile returns the path to the docker-compose.yml file
func GetComposeFile(bc *buildctx.BuildContext) string {
	return filepath.Join(bc.Path, "docker-compose.yml")
}

// FindManifest locates the manifest file in the working directory
func FindManifest(workDir string) string {
	candidates := []string{
		"teeter.yaml",
		".teeter/manifest.yaml",
	}
	for _, c := range candidates {
		path := c
		if workDir != "" {
			path = filepath.Join(workDir, c)
		}
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return "teeter.yaml"
}

// newDockerCommand creates a docker command with proper environment
func newDockerCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = dir
	return cmd
}
