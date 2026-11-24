package layout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheRoot(t *testing.T) {
	// Reset the getter for test
	originalGetter := configGetCacheDir
	defer func() { configGetCacheDir = originalGetter }()

	configGetCacheDir = func() string { return "/test/cache" }

	root := CacheRoot()
	assert.Equal(t, "/test/cache", root)
}

func TestModuleCacheDir(t *testing.T) {
	originalGetter := configGetCacheDir
	defer func() { configGetCacheDir = originalGetter }()

	configGetCacheDir = func() string { return "/test/cache" }

	dir := ModuleCacheDir("mod-playerbots", "master")
	assert.Equal(t, "/test/cache/modules/mod-playerbots/master", dir)
}

func TestModuleSourceDir(t *testing.T) {
	originalGetter := configGetCacheDir
	defer func() { configGetCacheDir = originalGetter }()

	configGetCacheDir = func() string { return "/test/cache" }

	dir := ModuleSourceDir("mod-playerbots", "master")
	assert.Equal(t, "/test/cache/modules/mod-playerbots/master/master", dir)
}

func TestACoreCacheDir(t *testing.T) {
	originalGetter := configGetCacheDir
	defer func() { configGetCacheDir = originalGetter }()

	configGetCacheDir = func() string { return "/test/cache" }

	dir := ACoreCacheDir("a1b2c3d")
	assert.Equal(t, "/test/cache/acore/a1b2c3d", dir)
}

func TestIndexPath(t *testing.T) {
	originalGetter := configGetCacheDir
	defer func() { configGetCacheDir = originalGetter }()

	configGetCacheDir = func() string { return "/test/cache" }

	path := IndexPath()
	assert.Equal(t, "/test/cache/index.json", path)
}

func TestLockPath(t *testing.T) {
	originalGetter := configGetCacheDir
	defer func() { configGetCacheDir = originalGetter }()

	configGetCacheDir = func() string { return "/test/cache" }

	path := LockPath("mod-playerbots")
	assert.Equal(t, "/test/cache/locks/mod-playerbots.lock", path)
}

func TestEnsureDirs(t *testing.T) {
	tmpDir := t.TempDir()

	originalGetter := configGetCacheDir
	defer func() { configGetCacheDir = originalGetter }()

	configGetCacheDir = func() string { return tmpDir }

	err := EnsureDirs()
	require.NoError(t, err)

	// Check all expected directories exist
	expectedDirs := []string{
		filepath.Join(tmpDir, "modules"),
		filepath.Join(tmpDir, "acore"),
		filepath.Join(tmpDir, "locks"),
	}

	for _, dir := range expectedDirs {
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.True(t, info.IsDir(), "directory %s should exist", dir)
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct {
		input    string
		expected string
	}{
		{"~/cache", filepath.Join(home, "cache")},
		{"~/", home},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"", ""},
	}

	for _, tt := range tests {
		result := expandPath(tt.input)
		assert.Equal(t, tt.expected, result, "input: %s", tt.input)
	}
}
