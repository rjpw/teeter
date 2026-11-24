package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested directory structure
	projectRoot := filepath.Join(tmpDir, "project", "subdir")
	require.NoError(t, os.MkdirAll(projectRoot, 0755))

	// Create .teeter dir at project root
	teeterDir := filepath.Join(tmpDir, "project", ".teeter")
	require.NoError(t, os.MkdirAll(teeterDir, 0755))

	// Change to subdir and test finding .teeter
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)

	require.NoError(t, os.Chdir(projectRoot))

	found, err := ProjectDir()
	require.NoError(t, err)
	assert.Equal(t, teeterDir, found)
}

func TestProjectDirNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)

	require.NoError(t, os.Chdir(tmpDir))

	_, err := ProjectDir()
	assert.Error(t, err)
	assert.Equal(t, os.ErrNotExist, err)
}

func TestEnsureDirs(t *testing.T) {
	tmpDir := t.TempDir()
	teeterDir := filepath.Join(tmpDir, ".teeter")

	err := EnsureDirs(teeterDir)
	require.NoError(t, err)

	expectedDirs := []string{
		teeterDir,
		filepath.Join(teeterDir, "logs"),
		filepath.Join(teeterDir, "backups"),
		filepath.Join(teeterDir, "buildctx"),
	}

	for _, dir := range expectedDirs {
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.True(t, info.IsDir(), "directory %s should exist", dir)
	}
}

func TestPaths(t *testing.T) {
	teeterDir := "/test/.teeter"

	tests := []struct {
		name     string
		fn       func(string) string
		expected string
	}{
		{"ConfigPath", ConfigPath, "/test/.teeter/config.yaml"},
		{"LockPath", LockPath, "/test/.teeter/lock.json"},
		{"ComposePath", ComposePath, "/test/.teeter/compose.yml"},
		{"LinksPath", LinksPath, "/test/.teeter/links.json"},
		{"LogsDir", LogsDir, "/test/.teeter/logs"},
		{"BackupsDir", BackupsDir, "/test/.teeter/backups"},
		{"BuildCtxDir", BuildCtxDir, "/test/.teeter/buildctx"},
		{"BuildCtxHashDir", func(d string) string { return BuildCtxHashDir(d, "abc123") }, "/test/.teeter/buildctx/abc123"},
	}

	for _, tt := range tests {
		result := tt.fn(teeterDir)
		assert.Equal(t, tt.expected, result, tt.name)
	}
}
