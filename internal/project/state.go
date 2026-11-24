package project

import (
	"os"
	"path/filepath"
)

// ProjectDir finds the .teeter directory by walking up from cwd
func ProjectDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		teeterDir := filepath.Join(cwd, ".teeter")
		if info, err := os.Stat(teeterDir); err == nil && info.IsDir() {
			return teeterDir, nil
		}

		parent := filepath.Dir(cwd)
		if parent == cwd {
			break // reached root
		}
		cwd = parent
	}

	return "", os.ErrNotExist
}

// EnsureDirs creates all necessary .teeter subdirectories
func EnsureDirs(teeterDir string) error {
	dirs := []string{
		teeterDir,
		filepath.Join(teeterDir, "logs"),
		filepath.Join(teeterDir, "backups"),
		filepath.Join(teeterDir, "buildctx"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// ConfigPath returns the path to project config
func ConfigPath(teeterDir string) string {
	return filepath.Join(teeterDir, "config.yaml")
}

// LockPath returns the path to lock.json
func LockPath(teeterDir string) string {
	return filepath.Join(teeterDir, "lock.json")
}

// ComposePath returns the path to compose.yml
func ComposePath(teeterDir string) string {
	return filepath.Join(teeterDir, "compose.yml")
}

// LinksPath returns the path to links.json
func LinksPath(teeterDir string) string {
	return filepath.Join(teeterDir, "links.json")
}

// LogsDir returns the path to logs directory
func LogsDir(teeterDir string) string {
	return filepath.Join(teeterDir, "logs")
}

// BackupsDir returns the path to backups directory
func BackupsDir(teeterDir string) string {
	return filepath.Join(teeterDir, "backups")
}

// BuildCtxDir returns the base buildctx directory
func BuildCtxDir(teeterDir string) string {
	return filepath.Join(teeterDir, "buildctx")
}

// BuildCtxHashDir returns the buildctx directory for a specific hash
func BuildCtxHashDir(teeterDir, hash string) string {
	return filepath.Join(BuildCtxDir(teeterDir), hash)
}
