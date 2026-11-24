package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rjpw/teeter/internal/cache"
	"github.com/rjpw/teeter/internal/catalogue"
	"github.com/rjpw/teeter/internal/config"
	"github.com/rjpw/teeter/internal/manifest"
	"github.com/spf13/cobra"
)

func NewPullCmd() *cobra.Command {
	var (
		dryRun       bool
		force        bool
		manifestPath string
	)

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Fetch modules and AC source to cache",
		Long:  `Resolve manifest modules and clone them to the local cache.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cfg := config.Get()
			workDir, _ := cmd.Flags().GetString("workdir")

			// Determine manifest path
			if manifestPath == "" {
				manifestPath = findManifest(workDir)
			}

			m, err := manifest.ParseFile(manifestPath)
			if err != nil {
				return fmt.Errorf("parse manifest: %w", err)
			}

			// Validate manifest
			manifestDir := filepath.Dir(manifestPath)
			if manifestDir == "" {
				manifestDir = "."
			}
			if err := manifest.Validate(m, manifestDir); err != nil {
				return fmt.Errorf("validate manifest: %w", err)
			}

			// Load or fetch catalogue index (only if needed)
			var idx *catalogue.Index
			needsIndex := false
			for _, spec := range m.Modules {
				if spec.Source == "catalogue" {
					needsIndex = true
					break
				}
			}

			if needsIndex {
				var err error
				idx, err = catalogue.LoadOrFetchIndex(ctx, cfg)
				if err != nil {
					if dryRun {
						fmt.Printf("Warning: could not fetch catalogue index: %v\n", err)
						idx = &catalogue.Index{Modules: make(map[string]catalogue.ModuleEntry)}
					} else {
						return fmt.Errorf("fetch catalogue index: %w", err)
					}
				}
			} else {
				idx = &catalogue.Index{Modules: make(map[string]catalogue.ModuleEntry)}
			}

			// Create cache
			c := cache.NewFSCache(cfg.CacheDir, cache.LinkStrategyFromString(cfg.LinkStrategy))

			// Resolve modules
			res, err := catalogue.Resolve(ctx, m, idx, catalogue.DefaultResolveOptions())
			if err != nil {
				if dryRun {
					fmt.Printf("Warning: resolution incomplete: %v\n", err)
					// Create partial resolution for dry-run display
					res = catalogue.Resolution{
						Modules:    make(map[string]catalogue.ResolvedModule),
						BuildOrder: []string{},
						ACCommit:   m.ACore.Branch,
						ACRepo:     m.ACore.Repo,
					}
				} else {
					return fmt.Errorf("resolve modules: %w", err)
				}
			}

			// Print resolution plan
			printResolutionPlan(m, res, idx)

			if dryRun {
				printCacheStatus(c, res)
				return nil
			}

			// Fetch to cache
			fmt.Println("Fetching to cache...")
			for i, name := range res.BuildOrder {
				rm := res.Modules[name]
				commitDisplay := rm.Commit
				if len(commitDisplay) > 12 {
					commitDisplay = commitDisplay[:12]
				}
				fmt.Printf("  [%d/%d] %s@%s ", i+1, len(res.BuildOrder), name, commitDisplay)
				if err := c.PutModule(ctx, name, rm.Version, rm.Repository, rm.Commit); err != nil {
					fmt.Printf("✗\n")
					return fmt.Errorf("fetch module %s: %w", name, err)
				}
				fmt.Printf("✓\n")
			}

			// Fetch AC if not in cache
			if !c.HasACore(res.ACCommit) {
				commitDisplay := res.ACCommit
				if len(commitDisplay) > 12 {
					commitDisplay = commitDisplay[:12]
				}
				fmt.Printf("  AC: %s@%s ", m.ACore.Repo, commitDisplay)
				if err := c.PutACore(ctx, res.ACCommit, m.ACore.Repo); err != nil {
					fmt.Printf("✗\n")
					return fmt.Errorf("fetch AC: %w", err)
				}
				fmt.Printf("✓\n")
			}

			fmt.Println("Done. Run 'teeter up' to start.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show resolution plan without cloning")
	cmd.Flags().BoolVar(&force, "force", false, "Re-clone even if cached")
	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", "", "Path to manifest file")

	return cmd
}

func findManifest(workDir string) string {
	candidates := []string{
		"teeter.yaml",
		".teeter/manifest.yaml",
	}
	for _, c := range candidates {
		path := c
		if workDir != "" {
			path = workDir + "/" + c
		}
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return "teeter.yaml"
}

func printResolutionPlan(m *manifest.Manifest, res catalogue.Resolution, idx *catalogue.Index) {
	fmt.Println("Resolving modules...")

	// AC
	acDisplay := m.ACore.Repo
	if m.ACore.Commit != "" {
		commitDisplay := m.ACore.Commit
		if len(commitDisplay) > 12 {
			commitDisplay = commitDisplay[:12]
		}
		acDisplay += "@" + commitDisplay
	} else {
		acDisplay += "@" + m.ACore.Branch
	}
	if res.ACCommit != m.ACore.Branch && res.ACCommit != m.ACore.Commit {
		commitDisplay := res.ACCommit
		if len(commitDisplay) > 12 {
			commitDisplay = commitDisplay[:12]
		}
		acDisplay += " → " + commitDisplay
	}
	fmt.Printf("  acore: %s\n", acDisplay)

	// Modules
	for _, name := range res.BuildOrder {
		rm := res.Modules[name]
		spec := rm.Spec

		var display string
		switch spec.Source {
		case "catalogue":
			entry := idx.Modules[name]
			var latest string
			if entry.LatestRelease != nil {
				latest = entry.LatestRelease.TagName
			} else {
				latest = entry.DefaultBranch
			}
			if spec.Version == "" {
				display = fmt.Sprintf("%s: %s (latest: %s)", name, latest, latest)
			} else {
				commitDisplay := rm.Commit
				if len(commitDisplay) > 12 {
					commitDisplay = commitDisplay[:12]
				}
				display = fmt.Sprintf("%s: %s → %s", name, spec.Version, commitDisplay)
			}
		case "git":
			commitDisplay := rm.Commit
			if len(commitDisplay) > 12 {
				commitDisplay = commitDisplay[:12]
			}
			display = fmt.Sprintf("%s: %s → %s", name, spec.Version, commitDisplay)
		case "local":
			display = fmt.Sprintf("%s: local (%s)", name, spec.Path)
		}
		fmt.Printf("  %s\n", display)
	}

	fmt.Printf("Build order: %s\n", formatSlice(res.BuildOrder))
}

func printCacheStatus(c *cache.FSCache, res catalogue.Resolution) {
	fmt.Println("Cache status:")
	for _, name := range res.BuildOrder {
		rm := res.Modules[name]
		if c.HasModule(name, rm.Version) {
			fmt.Printf("  %s@%s: present\n", name, rm.Version)
		} else {
			fmt.Printf("  %s@%s: missing\n", name, rm.Version)
		}
	}
	if c.HasACore(res.ACCommit) {
		acDisplay := res.ACCommit
		if len(acDisplay) > 12 {
			acDisplay = acDisplay[:12]
		}
		fmt.Printf("  acore@%s: present\n", acDisplay)
	} else {
		acDisplay := res.ACCommit
		if len(acDisplay) > 12 {
			acDisplay = acDisplay[:12]
		}
		fmt.Printf("  acore@%s: missing\n", acDisplay)
	}
}

func formatSlice(s []string) string {
	if len(s) == 0 {
		return "[]"
	}
	return "[" + strings.Join(s, ", ") + "]"
}
