package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ThinRuntime manages the .teeter/runtime/ symlink farm.
type ThinRuntime struct {
	WorkDir string // Project working directory (where teeter.yaml lives)
}

// RuntimePaths returns the key paths for the thin runtime.
func (r *ThinRuntime) RuntimePaths() (runtimeDir, buildctxLink, currentHashFile string) {
	runtimeDir = filepath.Join(r.WorkDir, ".teeter", "runtime")
	buildctxLink = filepath.Join(runtimeDir, "buildctx")
	currentHashFile = filepath.Join(runtimeDir, ".current-hash")
	return
}

// EnsureRuntimeDir creates the runtime directory structure.
func (r *ThinRuntime) EnsureRuntimeDir() error {
	runtimeDir, _, _ := r.RuntimePaths()
	return os.MkdirAll(runtimeDir, 0755)
}

// Switch switches the thin runtime to point to the given build context hash.
// This creates a full symlink farm for all required files.
func (r *ThinRuntime) Switch(buildCtxHash string) error {
	_, buildctxLink, currentHashFile := r.RuntimePaths()

	if err := r.EnsureRuntimeDir(); err != nil {
		return fmt.Errorf("ensure runtime dir: %w", err)
	}

	// Verify target build context exists
	targetBuildCtx := filepath.Join(r.WorkDir, ".teeter", "buildctx", buildCtxHash)
	if _, err := os.Stat(targetBuildCtx); err != nil {
		return fmt.Errorf("build context %s does not exist: %w", buildCtxHash, err)
	}

	// Atomic symlink switch for buildctx
	newLink := buildctxLink + ".new"
	if err := os.Symlink(targetBuildCtx, newLink); err != nil {
		return fmt.Errorf("create new symlink: %w", err)
	}
	if err := os.Rename(newLink, buildctxLink); err != nil {
		os.Remove(newLink)
		return fmt.Errorf("atomic symlink switch: %w", err)
	}

	// Create symlink farm for all required files/directories
	runtimeDir := filepath.Join(r.WorkDir, ".teeter", "runtime")
	symlinks := map[string]string{
		"docker-compose.yml": filepath.Join(buildctxLink, "docker-compose.yml"),
		"sql":                filepath.Join(buildctxLink, "sql"),
		"env.ac":             filepath.Join(buildctxLink, "env.ac"),
		"azerothcore":        filepath.Join(buildctxLink, "azerothcore"),
	}
	for name, target := range symlinks {
		linkPath := filepath.Join(runtimeDir, name)
		// Remove existing symlink or file
		os.Remove(linkPath)
		if err := os.Symlink(target, linkPath); err != nil {
			return fmt.Errorf("create symlink %s: %w", name, err)
		}
	}

	// Copy .env (not symlink, since it contains secrets)
	srcEnv := filepath.Join(targetBuildCtx, ".env")
	dstEnv := filepath.Join(runtimeDir, ".env")
	if data, err := os.ReadFile(srcEnv); err == nil {
		if err := os.WriteFile(dstEnv, data, 0644); err != nil {
			return fmt.Errorf("copy .env: %w", err)
		}
	}

	// Write current hash file
	if err := os.WriteFile(currentHashFile, []byte(buildCtxHash+"\n"), 0644); err != nil {
		return fmt.Errorf("write current hash: %w", err)
	}

	return nil
}

// CurrentHash returns the currently active build context hash.
func (r *ThinRuntime) CurrentHash() (string, error) {
	_, _, currentHashFile := r.RuntimePaths()
	data, err := os.ReadFile(currentHashFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil // Not initialized
		}
		return "", fmt.Errorf("read current hash: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// GetComposeFile returns the path to the docker-compose.yml in the active build context.
func (r *ThinRuntime) GetComposeFile() (string, error) {
	hash, err := r.CurrentHash()
	if err != nil {
		return "", err
	}
	if hash == "" {
		return "", errors.New("no active build context (run 'teeter up' first)")
	}
	return filepath.Join(r.WorkDir, ".teeter", "buildctx", hash, "docker-compose.yml"), nil
}

// GetBuildCtxPath returns the path to the active build context.
func (r *ThinRuntime) GetBuildCtxPath() (string, error) {
	hash, err := r.CurrentHash()
	if err != nil {
		return "", err
	}
	if hash == "" {
		return "", errors.New("no active build context")
	}
	return filepath.Join(r.WorkDir, ".teeter", "buildctx", hash), nil
}

// ListBuildContexts returns all available build context hashes.
func (r *ThinRuntime) ListBuildContexts() ([]string, error) {
	buildctxDir := filepath.Join(r.WorkDir, ".teeter", "buildctx")
	entries, err := os.ReadDir(buildctxDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("read buildctx dir: %w", err)
	}
	var hashes []string
	for _, e := range entries {
		if e.IsDir() {
			hashes = append(hashes, e.Name())
		}
	}
	return hashes, nil
}

// ValidateBuildContext checks if a build context has all required files.
func (r *ThinRuntime) ValidateBuildContext(hash string) error {
	buildCtxPath := filepath.Join(r.WorkDir, ".teeter", "buildctx", hash)
	required := []string{
		"docker-compose.yml",
		"env.ac",
		".env",
		"sql/teeter-dbimport.conf",
		"sql/teeter-authserver.conf",
		"sql/teeter-worldserver.conf",
		"sql/teeter-combined.sql",
	}
	for _, f := range required {
		if _, err := os.Stat(filepath.Join(buildCtxPath, f)); err != nil {
			return fmt.Errorf("missing %s in build context %s: %w", f, hash, err)
		}
	}
	return nil
}
