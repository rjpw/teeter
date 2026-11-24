package cache

import (
	"context"
	"strings"

	"github.com/rjpw/teeter/internal/catalogue"
)

// UnlockFunc is a function that releases a lock.
type UnlockFunc func() error

// Cache defines the interface for cache operations.
type Cache interface {
	// Module operations
	GetModule(ctx context.Context, name, version string) (string, error) // returns path to modules/<name>/
	PutModule(ctx context.Context, name, version, repoURL, commit string) error
	HasModule(name, version string) bool
	VerifyModule(name, version string) error
	RemoveModule(name, version string) error

	// AC operations
	GetACore(ctx context.Context, commit string) (string, error)
	PutACore(ctx context.Context, commit, repoURL string) error
	HasACore(commit string) bool

	// Index
	GetIndex() (*catalogue.Index, error)
	PutIndex(idx *catalogue.Index) error

	// Locking
	Lock(ctx context.Context, name string) (UnlockFunc, error)

	// Link strategy
	LinkStrategy() LinkStrategy
}

// LinkStrategy defines how to link files from cache to build context.
type LinkStrategy string

const (
	LinkStrategyHardlink LinkStrategy = "hardlink"
	LinkStrategySymlink  LinkStrategy = "symlink"
	LinkStrategyAuto     LinkStrategy = "auto"
	LinkStrategyCopy     LinkStrategy = "copy"
)

// LinkStrategyFromString parses a link strategy string.
func LinkStrategyFromString(s string) LinkStrategy {
	switch strings.ToLower(s) {
	case "hardlink":
		return LinkStrategyHardlink
	case "symlink":
		return LinkStrategySymlink
	case "auto":
		return LinkStrategyAuto
	case "copy":
		return LinkStrategyCopy
	default:
		return LinkStrategyAuto
	}
}
