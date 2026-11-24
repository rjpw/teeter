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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchIndex_NotModified(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.json")

	// Create a cached index with ETag
	cached := CachedIndex{
		Index: &Index{
			FetchedAt: time.Now(),
			Modules: map[string]ModuleEntry{
				"mod-test": {Name: "mod-test", RepositoryURL: "https://example.com/mod-test"},
			},
		},
		ETag: "\"abc123\"",
	}
	data, err := json.MarshalIndent(cached, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(indexPath, data, 0644))

	// Create test server that returns 304
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "\"abc123\"", r.Header.Get("If-None-Match"))
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	cfg := config.Config{
		CatalogueURL: server.URL,
		CacheDir:     tmpDir,
	}

	layout.SetCacheDirGetter(func() string { return tmpDir })
	defer layout.SetCacheDirGetter(func() string { return "~/.teeter/cache" })

	idx, err := FetchIndex(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "mod-test", idx.Modules["mod-test"].Name)
}

func TestFetchIndex_Fresh(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test server that returns 200 with new index
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "\"new-etag\"")
		w.WriteHeader(http.StatusOK)
		newIndex := Index{
			FetchedAt: time.Now(),
			Modules: map[string]ModuleEntry{
				"mod-new": {Name: "mod-new", RepositoryURL: "https://example.com/mod-new"},
			},
		}
		json.NewEncoder(w).Encode(newIndex)
	}))
	defer server.Close()

	cfg := config.Config{
		CatalogueURL: server.URL,
		CacheDir:     tmpDir,
	}

	layout.SetCacheDirGetter(func() string { return tmpDir })
	defer layout.SetCacheDirGetter(func() string { return "~/.teeter/cache" })

	idx, err := FetchIndex(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "mod-new", idx.Modules["mod-new"].Name)

	// Verify cached index was saved with new ETag
	savedData, err := os.ReadFile(layout.IndexPath())
	require.NoError(t, err)
	var saved CachedIndex
	require.NoError(t, json.Unmarshal(savedData, &saved))
	assert.Equal(t, "\"new-etag\"", saved.ETag)
	assert.Equal(t, "mod-new", saved.Index.Modules["mod-new"].Name)
}

func TestLoadIndex_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "nonexistent.json")

	_, err := LoadIndex(indexPath)
	assert.ErrorIs(t, err, ErrIndexNotFound)
}

func TestLoadIndex_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.json")
	require.NoError(t, os.WriteFile(indexPath, []byte("invalid json"), 0644))

	_, err := LoadIndex(indexPath)
	assert.Error(t, err)
}
