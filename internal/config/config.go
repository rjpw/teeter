package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rjpw/teeter/internal/cache/layout"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Config struct {
	CacheDir             string `mapstructure:"cache_dir"`
	CatalogueURL         string `mapstructure:"catalogue_url"`
	DefaultACoreRepo     string `mapstructure:"default_acore_repo"`
	DefaultACoreBranch   string `mapstructure:"default_acore_branch"`
	DockerComposeVersion string `mapstructure:"docker_compose_version"`
	LinkStrategy         string `mapstructure:"link_strategy"`
}

var (
	cfg         Config
	v           *viper.Viper
	initialized bool
)

// InitConfig initializes the viper configuration with precedence:
// 1. CLI flags (highest)
// 2. Project .teeter/config.yaml
// 3. Global ~/.teeter/config.yaml
// 4. Defaults (lowest)
func InitConfig(cmd *cobra.Command) error {
	if initialized {
		return nil
	}

	v = viper.New()

	// Set defaults
	setDefaults()

	// Global config file: ~/.teeter/config.yaml
	homeDir, _ := os.UserHomeDir()
	globalConfigPath := filepath.Join(homeDir, ".teeter", "config.yaml")
	v.SetConfigFile(globalConfigPath)
	v.SetConfigType("yaml")
	_ = v.ReadInConfig() // Ignore error if not found

	// Project config file: .teeter/config.yaml (in workdir)
	workDir, _ := cmd.Flags().GetString("workdir")
	if workDir == "" {
		workDir = "."
	}
	v.AddConfigPath(filepath.Join(workDir, ".teeter"))
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	_ = v.MergeInConfig() // Ignore error if not found

	// Bind CLI flags
	bindFlags(cmd)

	// Unmarshal into struct
	if err := v.Unmarshal(&cfg); err != nil {
		return err
	}

	// Expand ~ in cache dir
	cfg.CacheDir = expandHome(cfg.CacheDir)

	layout.SetCacheDirGetter(func() string { return cfg.CacheDir })

	initialized = true

	return nil
}

func setDefaults() {
	v.SetDefault("cache_dir", "~/.teeter/cache")
	v.SetDefault("catalogue_url", "https://raw.githubusercontent.com/rjpw/teeter-registry/main/index.json")
	v.SetDefault("default_acore_repo", "https://github.com/mod-playerbots/azerothcore-wotlk")
	v.SetDefault("default_acore_branch", "Playerbot")
	v.SetDefault("docker_compose_version", "v2")
	v.SetDefault("link_strategy", "auto")
}

func bindFlags(cmd *cobra.Command) {
	v.BindPFlag("cache_dir", cmd.Flags().Lookup("cache-dir"))
	v.BindPFlag("catalogue_url", cmd.Flags().Lookup("catalogue-url"))
	v.BindPFlag("default_acore_repo", cmd.Flags().Lookup("default-acore-repo"))
	v.BindPFlag("default_acore_branch", cmd.Flags().Lookup("default-acore-branch"))
	v.BindPFlag("docker_compose_version", cmd.Flags().Lookup("docker-compose-version"))
	v.BindPFlag("link_strategy", cmd.Flags().Lookup("link-strategy"))
}

// AddConfigFlags adds the config-related flags to a command
func AddConfigFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("cache-dir", "", "Cache directory (default: ~/.teeter/cache)")
	cmd.PersistentFlags().String("catalogue-url", "", "Catalogue index URL")
	cmd.PersistentFlags().String("default-acore-repo", "", "Default AC repo URL")
	cmd.PersistentFlags().String("default-acore-branch", "", "Default AC branch")
	cmd.PersistentFlags().String("docker-compose-version", "", "Docker compose version (v1|v2)")
	cmd.PersistentFlags().String("link-strategy", "", "Link strategy (hardlink|symlink|auto)")
}

// Get returns the loaded configuration
func Get() Config {
	return cfg
}

// GetCacheDir returns the expanded cache directory
func GetCacheDir() string {
	return cfg.CacheDir
}

// GetCatalogueURL returns the catalogue URL
func GetCatalogueURL() string {
	return cfg.CatalogueURL
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, path[2:])
	}
	return path
}
