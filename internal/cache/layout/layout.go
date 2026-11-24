package layout

import (
	"os"
	"path/filepath"
)

// CacheRoot returns the expanded cache directory path
func CacheRoot() string {
	return expandPath(configGetCacheDir())
}

// ModuleCacheDir returns the cache directory for a specific module version
func ModuleCacheDir(name, version string) string {
	return filepath.Join(CacheRoot(), "modules", name, version)
}

// ModuleSourceDir returns the path to the module source within the cache
// (i.e., the modules/<name>/ subdirectory inside the version dir)
func ModuleSourceDir(name, version string) string {
	return filepath.Join(ModuleCacheDir(name, version), "modules", name)
}

// ACoreCacheDir returns the cache directory for a specific AC commit
func ACoreCacheDir(commit string) string {
	return filepath.Join(CacheRoot(), "acore", commit)
}

// IndexPath returns the path to the cached catalogue index
func IndexPath() string {
	return filepath.Join(CacheRoot(), "index.json")
}

// LockPath returns the path to a lock file for the given name
func LockPath(name string) string {
	return filepath.Join(CacheRoot(), "locks", name+".lock")
}

// EnsureDirs creates all necessary cache subdirectories
func EnsureDirs() error {
	dirs := []string{
		filepath.Join(CacheRoot(), "modules"),
		filepath.Join(CacheRoot(), "acore"),
		filepath.Join(CacheRoot(), "locks"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// configGetCacheDir is a function variable to allow config package to provide the cache dir
// This avoids import cycles; it's set by the config package during init
var configGetCacheDir func() string

func init() {
	// This will be overridden by config.SetCacheDirGetter in main
	configGetCacheDir = func() string { return "~/.teeter/cache" }
}

func expandPath(path string) string {
	if path == "" {
		return ""
	}
	if path[0] == '~' {
		home, _ := os.UserHomeDir()
		if len(path) > 1 && path[1] == '/' {
			return filepath.Join(home, path[2:])
		}
		return home
	}
	return path
}

// ExpandPath expands a path, resolving ~ to home directory
func ExpandPath(path string) string {
	return expandPath(path)
}

// SetCacheDirGetter sets the function to get the cache dir from config
// Called from config package to avoid import cycle
func SetCacheDirGetter(f func() string) {
	configGetCacheDir = f
}
