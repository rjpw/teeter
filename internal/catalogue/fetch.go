package catalogue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/rjpw/teeter/internal/cache/layout"
	"github.com/rjpw/teeter/internal/config"
)

var (
	ErrIndexNotModified = errors.New("index not modified")
	ErrIndexNotFound    = errors.New("index not found")
)

// FetchIndex fetches the catalogue index from the configured URL with ETag caching.
func FetchIndex(ctx context.Context, cfg config.Config) (*Index, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	indexPath := layout.IndexPath()
	var etag string

	// Load existing index to get ETag
	if existing, err := LoadIndex(indexPath); err == nil {
		etag = existing.ETag
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.CatalogueURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := client.Do(req)
	if err != nil {
		// On network error, try to return cached index
		if cached, err := LoadIndex(indexPath); err == nil {
			return cached.Index, nil
		}
		return nil, fmt.Errorf("fetch index: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		// Return cached index
		cached, err := LoadIndex(indexPath)
		if err != nil {
			return nil, fmt.Errorf("load cached index: %w", err)
		}
		return cached.Index, nil

	case http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}

		var idx Index
		if err := json.Unmarshal(body, &idx); err != nil {
			return nil, fmt.Errorf("parse index json: %w", err)
		}

		// Save to cache with ETag
		newEtag := resp.Header.Get("ETag")
		if err := SaveIndex(&idx, indexPath, newEtag); err != nil {
			return nil, fmt.Errorf("save index: %w", err)
		}

		return &idx, nil

	default:
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
}

// CachedIndex holds the index and its ETag for caching.
type CachedIndex struct {
	Index *Index
	ETag  string
}

// LoadIndex loads the cached index from disk.
func LoadIndex(path string) (*CachedIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrIndexNotFound
		}
		return nil, fmt.Errorf("read index file: %w", err)
	}

	var cached CachedIndex
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("parse cached index: %w", err)
	}

	return &cached, nil
}

// SaveIndex saves the index to disk with ETag.
func SaveIndex(idx *Index, path, etag string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	cached := CachedIndex{
		Index: idx,
		ETag:  etag,
	}

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// LoadOrFetchIndex loads cached index or fetches fresh with retries.
func LoadOrFetchIndex(ctx context.Context, cfg config.Config) (*Index, error) {
	const maxRetries = 3

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		idx, err := FetchIndex(ctx, cfg)
		if err == nil {
			return idx, nil
		}

		// Don't retry on 304 (not modified) - that's success
		if errors.Is(err, ErrIndexNotModified) {
			cached, err := LoadIndex(layout.IndexPath())
			if err != nil {
				return nil, err
			}
			return cached.Index, nil
		}

		lastErr = err
		if attempt < maxRetries {
			backoff := time.Duration(1<<attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	return nil, fmt.Errorf("fetch index after %d retries: %w", maxRetries, lastErr)
}
