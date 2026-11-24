package buildctx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rjpw/teeter/internal/cache"
	"github.com/rjpw/teeter/internal/catalogue"
	"github.com/rjpw/teeter/internal/manifest"
)

// BuildContext represents the assembled build context for a teeter project.
type BuildContext struct {
	Path        string            // .teeter/buildctx/<hash>/
	ACPath      string            // .../azerothcore/
	ModulePaths map[string]string // module name -> linked path
	Manifest    *manifest.Manifest
	Resolution  *catalogue.Resolution
	Hash        string // content hash for caching
}

// Assemble creates a build context from a manifest and resolution.
func Assemble(ctx context.Context, m *manifest.Manifest, res *catalogue.Resolution, c cache.Cache, workDir string) (*BuildContext, error) {
	// Compute hash
	hash, err := computeHash(m, res)
	if err != nil {
		return nil, fmt.Errorf("compute hash: %w", err)
	}

	// Build context path
	buildCtxPath := filepath.Join(workDir, ".teeter", "buildctx", hash)

	// Check if already exists and is valid
	if isValidBuildContext(buildCtxPath, hash) {
		// Load existing
		return loadBuildContext(buildCtxPath, m, res, hash)
	}

	// Create fresh build context
	bc := &BuildContext{
		Path:        buildCtxPath,
		ModulePaths: make(map[string]string),
		Manifest:    m,
		Resolution:  res,
		Hash:        hash,
	}

	// Create directory structure
	if err := createDirs(bc.Path); err != nil {
		return nil, fmt.Errorf("create dirs: %w", err)
	}

	// Link AC source
	acPath, err := linkACSource(bc, c)
	if err != nil {
		return nil, fmt.Errorf("link AC source: %w", err)
	}
	bc.ACPath = acPath

	// Link modules
	if err := LinkModules(ctx, bc, c, c.LinkStrategy()); err != nil {
		return nil, fmt.Errorf("link modules: %w", err)
	}

	// Copy patches
	if err := copyPatches(bc, m); err != nil {
		return nil, fmt.Errorf("copy patches: %w", err)
	}

	// Assemble SQL
	if err := AssembleSQL(ctx, bc, c); err != nil {
		return nil, fmt.Errorf("assemble SQL: %w", err)
	}

	// Write resolved manifest
	if err := writeResolvedManifest(bc); err != nil {
		return nil, fmt.Errorf("write resolved manifest: %w", err)
	}

	return bc, nil
}

// computeHash computes a content hash for the build context.
func computeHash(m *manifest.Manifest, res *catalogue.Resolution) (string, error) {
	h := sha256.New()

	// AC commit
	h.Write([]byte(res.ACCommit + "\n"))

	// Module commits in build order
	for _, name := range res.BuildOrder {
		if mod, ok := res.Modules[name]; ok {
			h.Write([]byte(mod.Commit + "\n"))
		}
	}

	// Patch contents
	for _, modSpec := range m.Modules {
		for _, patch := range modSpec.Patches {
			content, err := os.ReadFile(patch.Path)
			if err != nil {
				// If patch file doesn't exist, include path in hash anyway
				h.Write([]byte(patch.Path + "\n"))
			} else {
				h.Write(content)
			}
			h.Write([]byte("\n"))
		}
	}

	// Manifest content (for any other relevant fields)
	// We include project name, AC repo, database specs
	h.Write([]byte(m.Project.Name + "\n"))
	h.Write([]byte(m.ACore.Repo + "\n"))
	h.Write([]byte(m.ACore.Branch + "\n"))
	for _, mig := range m.Database.Migrations {
		h.Write([]byte(mig + "\n"))
	}
	for _, seed := range m.Database.Seed {
		h.Write([]byte(seed + "\n"))
	}

	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// isValidBuildContext checks if the build context exists and matches the hash.
func isValidBuildContext(path, expectedHash string) bool {
	hashFile := filepath.Join(path, ".teeter", "hash")
	data, err := os.ReadFile(hashFile)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == expectedHash
}

// loadBuildContext loads an existing valid build context.
func loadBuildContext(path string, m *manifest.Manifest, res *catalogue.Resolution, hash string) (*BuildContext, error) {
	bc := &BuildContext{
		Path:        path,
		ModulePaths: make(map[string]string),
		Manifest:    m,
		Resolution:  res,
		Hash:        hash,
	}

	bc.ACPath = filepath.Join(path, "azerothcore")

	// Populate module paths
	for _, name := range res.BuildOrder {
		bc.ModulePaths[name] = filepath.Join(path, "modules", name)
	}

	return bc, nil
}

// createDirs creates the build context directory structure.
func createDirs(root string) error {
	dirs := []string{
		filepath.Join(root, "azerothcore"),
		filepath.Join(root, "modules"),
		filepath.Join(root, "patches"),
		filepath.Join(root, "sql", "custom"),
		filepath.Join(root, "sql", "modules"),
		filepath.Join(root, ".teeter"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// linkACSource links the AC source from cache to build context.
func linkACSource(bc *BuildContext, c cache.Cache) (string, error) {
	acCachePath, err := c.GetACore(nil, bc.Resolution.ACCommit)
	if err != nil {
		return "", err
	}

	// The worktree is at cacheDir/worktree
	worktreePath := filepath.Join(acCachePath, "worktree")
	targetPath := filepath.Join(bc.Path, "azerothcore")

	// Use the cache's link strategy
	return targetPath, linkDir(worktreePath, targetPath, c.LinkStrategy())
}

// linkDir links a directory using the specified strategy.
func linkDir(src, dst string, strategy cache.LinkStrategy) error {
	// Remove existing destination if it exists
	if err := os.RemoveAll(dst); err != nil {
		return err
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	switch strategy {
	case cache.LinkStrategyHardlink:
		return linkDirHardlink(src, dst)
	case cache.LinkStrategySymlink:
		return os.Symlink(src, dst)
	case cache.LinkStrategyAuto:
		if err := linkDirHardlink(src, dst); err != nil {
			// Fall back to symlink on EXDEV or permission errors
			return os.Symlink(src, dst)
		}
		return nil
	case cache.LinkStrategyCopy:
		return copyDir(src, dst)
	default:
		return os.Symlink(src, dst)
	}
}

// linkDirHardlink creates a hardlink tree (directory structure with hardlinked files).
func linkDirHardlink(src, dst string) error {
	// Create destination root directory first
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Hardlink the file
		return os.Link(path, dstPath)
	})
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}

// copyPatches copies patch files from manifest to build context.
func copyPatches(bc *BuildContext, m *manifest.Manifest) error {
	patchesDir := filepath.Join(bc.Path, "patches")
	for _, modSpec := range m.Modules {
		for _, patch := range modSpec.Patches {
			src := patch.Path
			// If relative, resolve relative to manifest directory
			if !filepath.IsAbs(src) {
				// We need the manifest directory - for now assume it's in workDir
				// This will be improved when we have the manifest path
				src = filepath.Join(filepath.Dir(bc.Path), "..", src)
			}

			dst := filepath.Join(patchesDir, filepath.Base(src))
			data, err := os.ReadFile(src)
			if err != nil {
				return fmt.Errorf("read patch %s: %w", src, err)
			}
			if err := os.WriteFile(dst, data, 0644); err != nil {
				return fmt.Errorf("write patch %s: %w", dst, err)
			}
		}
	}
	return nil
}

// writeResolvedManifest writes the resolved manifest to the build context.
func writeResolvedManifest(bc *BuildContext) error {
	// For now, write the original manifest with resolved commits
	// In the future, we could write a fully resolved manifest
	dst := filepath.Join(bc.Path, "teeter.yaml")
	// This is a placeholder - we'd need yaml marshaling
	return os.WriteFile(dst, []byte("# Resolved manifest\n# TODO: write full resolved manifest\n"), 0644)
}
