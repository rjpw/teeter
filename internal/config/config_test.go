package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitConfigDefaults(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := &cobra.Command{}
	cmd.Flags().String("workdir", tmpDir, "")
	AddConfigFlags(cmd)
	_ = cmd.ParseFlags([]string{})

	initialized = false
	v = nil

	err := InitConfig(cmd)
	require.NoError(t, err)

	cfg := Get()
	assert.Equal(t, expandHome("~/.teeter/cache"), cfg.CacheDir)
	assert.Equal(t, "https://raw.githubusercontent.com/rjpw/teeter-registry/main/index.json", cfg.CatalogueURL)
	assert.Equal(t, "https://github.com/mod-playerbots/azerothcore-wotlk", cfg.DefaultACoreRepo)
	assert.Equal(t, "Playerbot", cfg.DefaultACoreBranch)
	assert.Equal(t, "v2", cfg.DockerComposeVersion)
	assert.Equal(t, "auto", cfg.LinkStrategy)
}

func TestInitConfigWithProjectConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create project config
	teeterDir := filepath.Join(tmpDir, ".teeter")
	require.NoError(t, os.MkdirAll(teeterDir, 0755))
	projectConfig := filepath.Join(teeterDir, "config.yaml")
	require.NoError(t, os.WriteFile(projectConfig, []byte(`
cache_dir: "/custom/cache"
catalogue_url: "https://custom.example.com/index.json"
link_strategy: "symlink"
`), 0644))

	cmd := &cobra.Command{}
	cmd.Flags().String("workdir", tmpDir, "")
	AddConfigFlags(cmd)
	_ = cmd.ParseFlags([]string{})

	initialized = false
	v = nil

	err := InitConfig(cmd)
	require.NoError(t, err)

	cfg := Get()
	assert.Equal(t, "/custom/cache", cfg.CacheDir)
	assert.Equal(t, "https://custom.example.com/index.json", cfg.CatalogueURL)
	assert.Equal(t, "symlink", cfg.LinkStrategy)
	// Defaults should still apply for unset values
	assert.Equal(t, "https://github.com/mod-playerbots/azerothcore-wotlk", cfg.DefaultACoreRepo)
}

func TestInitConfigWithFlags(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := &cobra.Command{}
	cmd.Flags().String("workdir", tmpDir, "")
	AddConfigFlags(cmd)
	_ = cmd.ParseFlags([]string{
		"--cache-dir", "/flag/cache",
		"--catalogue-url", "https://flag.example.com/index.json",
		"--link-strategy", "hardlink",
	})

	initialized = false
	v = nil

	err := InitConfig(cmd)
	require.NoError(t, err)

	cfg := Get()
	assert.Equal(t, "/flag/cache", cfg.CacheDir)
	assert.Equal(t, "https://flag.example.com/index.json", cfg.CatalogueURL)
	assert.Equal(t, "hardlink", cfg.LinkStrategy)
}

func TestExpandHome(t *testing.T) {
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
		result := expandHome(tt.input)
		assert.Equal(t, tt.expected, result, "input: %s", tt.input)
	}
}
