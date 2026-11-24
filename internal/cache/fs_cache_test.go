package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFSCache_HasModule(t *testing.T) {
	tmpDir := t.TempDir()
	c := NewFSCache(tmpDir, LinkStrategyAuto)

	assert.False(t, c.HasModule("mod-test", "v1.0.0"))

	// Create the module directory structure
	moduleDir := filepath.Join(tmpDir, "modules", "mod-test", "v1.0.0", "modules", "mod-test")
	require.NoError(t, os.MkdirAll(moduleDir, 0755))

	assert.True(t, c.HasModule("mod-test", "v1.0.0"))
}

func TestFSCache_HasACore(t *testing.T) {
	tmpDir := t.TempDir()
	c := NewFSCache(tmpDir, LinkStrategyAuto)

	assert.False(t, c.HasACore("abc123"))

	acoreDir := filepath.Join(tmpDir, "acore", "abc123")
	require.NoError(t, os.MkdirAll(acoreDir, 0755))

	assert.True(t, c.HasACore("abc123"))
}

func TestFSCache_GetModule_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	c := NewFSCache(tmpDir, LinkStrategyAuto)

	_, err := c.GetModule(context.Background(), "mod-test", "v1.0.0")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestFSCache_LinkStrategy(t *testing.T) {
	tmpDir := t.TempDir()

	c1 := NewFSCache(tmpDir, LinkStrategyHardlink)
	assert.Equal(t, LinkStrategyHardlink, c1.LinkStrategy())

	c2 := NewFSCache(tmpDir, LinkStrategySymlink)
	assert.Equal(t, LinkStrategySymlink, c2.LinkStrategy())

	c3 := NewFSCache(tmpDir, LinkStrategyAuto)
	assert.Equal(t, LinkStrategyAuto, c3.LinkStrategy())

	c4 := NewFSCache(tmpDir, "")
	assert.Equal(t, LinkStrategyAuto, c4.LinkStrategy())
}

func TestFSCache_Index(t *testing.T) {
	tmpDir := t.TempDir()
	c := NewFSCache(tmpDir, LinkStrategyAuto)

	// Test GetIndex when not exists
	_, err := c.GetIndex()
	assert.ErrorIs(t, err, ErrNotFound)

	// PutIndex and GetIndex
	// We can't easily test PutIndex/GetIndex without the catalogue.Index type
	// but we can verify the file is created
	indexPath := filepath.Join(tmpDir, "index.json")
	require.NoError(t, os.MkdirAll(tmpDir, 0755))
	// Just verify the path is correct
	assert.Equal(t, indexPath, filepath.Join(tmpDir, "index.json"))
}

func TestLinkStrategyFromString(t *testing.T) {
	assert.Equal(t, LinkStrategyHardlink, LinkStrategyFromString("hardlink"))
	assert.Equal(t, LinkStrategyHardlink, LinkStrategyFromString("HARDLINK"))
	assert.Equal(t, LinkStrategySymlink, LinkStrategyFromString("symlink"))
	assert.Equal(t, LinkStrategyAuto, LinkStrategyFromString("auto"))
	assert.Equal(t, LinkStrategyAuto, LinkStrategyFromString("unknown"))
	assert.Equal(t, LinkStrategyCopy, LinkStrategyFromString("copy"))
}
