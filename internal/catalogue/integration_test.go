package catalogue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rjpw/teeter/internal/cache/layout"
	"github.com/rjpw/teeter/internal/config"
	"github.com/rjpw/teeter/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPullIntegration tests the full pull flow with a mock catalogue server
func TestPullIntegration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test server with catalogue index
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == "\"test-etag\"" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", "\"test-etag\"")
		w.WriteHeader(http.StatusOK)
		index := Index{
			FetchedAt: time.Now(),
			Modules: map[string]ModuleEntry{
				"mod-playerbots": {
					Name:          "mod-playerbots",
					RepositoryURL: "https://github.com/test/mod-playerbots",
					DefaultBranch: "master",
					Releases: []Release{
						{TagName: "v1.0.0", TargetSHA: "abc123def456", PublishedAt: "2024-01-01", Prerelease: false},
					},
					LatestRelease: &Release{TagName: "v1.0.0", TargetSHA: "abc123def456"},
					Dependencies:  []DependencySpec{},
				},
				"mod-dungeon-clear": {
					Name:          "mod-dungeon-clear",
					RepositoryURL: "https://github.com/test/mod-dungeon-clear",
					DefaultBranch: "main",
					Releases: []Release{
						{TagName: "v2.0.0", TargetSHA: "fed654cba321", PublishedAt: "2024-02-01", Prerelease: false},
					},
					LatestRelease: &Release{TagName: "v2.0.0", TargetSHA: "fed654cba321"},
					Dependencies:  []DependencySpec{{Name: "mod-playerbots", Version: "*", Optional: false, Category: "module"}},
				},
			},
		}
		json.NewEncoder(w).Encode(index)
	}))
	defer server.Close()

	// Create manifest with catalogue modules
	manifestPath := filepath.Join(tmpDir, "teeter.yaml")
	manifestContent := `version: "1"
project:
  name: "test-server"
acore:
  repo: "https://github.com/test/acore"
  branch: "main"
modules:
  - name: "mod-playerbots"
    source: "catalogue"
    version: "v1.0.0"
  - name: "mod-dungeon-clear"
    source: "catalogue"
    version: "v2.0.0"
    depends_on:
      - "mod-playerbots"
client_data:
  source: "local"
  path: "/tmp/wow-client"
environment:
  vars: {}
compose: {}
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0644))

	cfg := config.Config{
		CatalogueURL: server.URL,
		CacheDir:     filepath.Join(tmpDir, "cache"),
		LinkStrategy: "auto",
	}

	layout.SetCacheDirGetter(func() string { return cfg.CacheDir })
	defer layout.SetCacheDirGetter(func() string { return "~/.teeter/cache" })

	// Fetch index
	idx, err := FetchIndex(context.Background(), cfg)
	require.NoError(t, err)
	assert.Len(t, idx.Modules, 2)

	// Resolve
	m, err := manifest.ParseFile(manifestPath)
	require.NoError(t, err)

	res, err := Resolve(context.Background(), m, idx, DefaultResolveOptions())
	require.NoError(t, err)

	// Verify resolution
	assert.Len(t, res.Modules, 2)
	assert.Equal(t, "abc123def456", res.Modules["mod-playerbots"].Commit)
	assert.Equal(t, "v1.0.0", res.Modules["mod-playerbots"].Version)
	assert.Equal(t, "fed654cba321", res.Modules["mod-dungeon-clear"].Commit)
	assert.Equal(t, "v2.0.0", res.Modules["mod-dungeon-clear"].Version)

	// Verify build order (mod-playerbots before mod-dungeon-clear due to dependency)
	assert.Equal(t, []string{"mod-playerbots", "mod-dungeon-clear"}, res.BuildOrder)
	assert.Equal(t, "main", res.ACCommit)
}

func TestPullIntegration_CachedIndex(t *testing.T) {
	tmpDir := t.TempDir()

	// Create cached index
	cacheDir := filepath.Join(tmpDir, "cache")
	indexPath := filepath.Join(cacheDir, "index.json")
	require.NoError(t, os.MkdirAll(cacheDir, 0755))

	cached := CachedIndex{
		Index: &Index{
			FetchedAt: time.Now(),
			Modules: map[string]ModuleEntry{
				"mod-cached": {
					Name:          "mod-cached",
					RepositoryURL: "https://github.com/test/mod-cached",
					DefaultBranch: "main",
					LatestRelease: &Release{TagName: "v1.0.0", TargetSHA: "cached123"},
				},
			},
		},
		ETag: "\"cached-etag\"",
	}
	data, err := json.MarshalIndent(cached, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(indexPath, data, 0644))

	// Server that returns 304
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "\"cached-etag\"", r.Header.Get("If-None-Match"))
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	cfg := config.Config{
		CatalogueURL: server.URL,
		CacheDir:     cacheDir,
	}

	layout.SetCacheDirGetter(func() string { return cacheDir })
	defer layout.SetCacheDirGetter(func() string { return "~/.teeter/cache" })

	idx, err := FetchIndex(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "cached123", idx.Modules["mod-cached"].LatestRelease.TargetSHA)
}

func TestPullIntegration_DependencyResolution(t *testing.T) {
	tmpDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "\"dep-etag\"")
		w.WriteHeader(http.StatusOK)
		index := Index{
			FetchedAt: time.Now(),
			Modules: map[string]ModuleEntry{
				"mod-a": {Name: "mod-a", RepositoryURL: "https://github.com/test/mod-a", DefaultBranch: "main", LatestRelease: &Release{TagName: "v1.0.0", TargetSHA: "sha-a"}},
				"mod-b": {Name: "mod-b", RepositoryURL: "https://github.com/test/mod-b", DefaultBranch: "main", LatestRelease: &Release{TagName: "v1.0.0", TargetSHA: "sha-b"}, Dependencies: []DependencySpec{{Name: "mod-a", Version: "*", Optional: false, Category: "module"}}},
				"mod-c": {Name: "mod-c", RepositoryURL: "https://github.com/test/mod-c", DefaultBranch: "main", LatestRelease: &Release{TagName: "v1.0.0", TargetSHA: "sha-c"}, Dependencies: []DependencySpec{{Name: "mod-b", Version: "*", Optional: false, Category: "module"}}},
			},
		}
		json.NewEncoder(w).Encode(index)
	}))
	defer server.Close()

	manifestPath := filepath.Join(tmpDir, "teeter.yaml")
	manifestContent := `version: "1"
project:
  name: "test"
acore:
  repo: "https://github.com/test/acore"
  branch: "main"
modules:
  - name: "mod-a"
    source: "catalogue"
  - name: "mod-b"
    source: "catalogue"
  - name: "mod-c"
    source: "catalogue"
    depends_on:
      - "mod-a"
client_data:
  source: "local"
  path: "/tmp/test"
environment:
  vars: {}
compose: {}
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0644))

	cfg := config.Config{
		CatalogueURL: server.URL,
		CacheDir:     filepath.Join(tmpDir, "cache"),
	}

	layout.SetCacheDirGetter(func() string { return cfg.CacheDir })
	defer layout.SetCacheDirGetter(func() string { return "~/.teeter/cache" })

	idx, err := FetchIndex(context.Background(), cfg)
	require.NoError(t, err)

	m, err := manifest.ParseFile(manifestPath)
	require.NoError(t, err)

	res, err := Resolve(context.Background(), m, idx, DefaultResolveOptions())
	require.NoError(t, err)

	// Build order should be mod-a, mod-b, mod-c
	// mod-a and mod-b have catalogue deps; mod-c has manifest dep on mod-a
	// Combined: mod-a (no deps), mod-b (dep on mod-a), mod-c (deps on mod-a, mod-b via catalogue)
	assert.Equal(t, []string{"mod-a", "mod-b", "mod-c"}, res.BuildOrder)
}

func TestPullIntegration_ConflictDetection(t *testing.T) {
	tmpDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "\"conflict-etag\"")
		w.WriteHeader(http.StatusOK)
		index := Index{
			FetchedAt: time.Now(),
			Modules: map[string]ModuleEntry{
				"mod-x": {Name: "mod-x", RepositoryURL: "https://github.com/test/mod-x", DefaultBranch: "main", LatestRelease: &Release{TagName: "v1.0.0", TargetSHA: "sha-x"}, Conflicts: []string{"mod-y"}},
				"mod-y": {Name: "mod-y", RepositoryURL: "https://github.com/test/mod-y", DefaultBranch: "main", LatestRelease: &Release{TagName: "v1.0.0", TargetSHA: "sha-y"}},
			},
		}
		json.NewEncoder(w).Encode(index)
	}))
	defer server.Close()

	manifestPath := filepath.Join(tmpDir, "teeter.yaml")
	manifestContent := `version: "1"
project:
  name: "test"
acore:
  repo: "https://github.com/test/acore"
  branch: "main"
modules:
  - name: "mod-x"
    source: "catalogue"
  - name: "mod-y"
    source: "catalogue"
client_data:
  source: "local"
  path: "/tmp/test"
environment:
  vars: {}
compose: {}
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0644))

	cfg := config.Config{
		CatalogueURL: server.URL,
		CacheDir:     filepath.Join(tmpDir, "cache"),
	}

	layout.SetCacheDirGetter(func() string { return cfg.CacheDir })
	defer layout.SetCacheDirGetter(func() string { return "~/.teeter/cache" })

	idx, err := FetchIndex(context.Background(), cfg)
	require.NoError(t, err)

	m, err := manifest.ParseFile(manifestPath)
	require.NoError(t, err)

	_, err = Resolve(context.Background(), m, idx, DefaultResolveOptions())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicts with")
}
