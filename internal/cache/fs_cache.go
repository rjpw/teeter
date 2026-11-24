package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/rjpw/teeter/internal/cache/layout"
	"github.com/rjpw/teeter/internal/catalogue"
)

var (
	ErrLockTimeout = errors.New("lock timeout")
	ErrNotFound    = errors.New("not found")
)

type FSCache struct {
	cacheDir     string
	linkStrategy LinkStrategy
	locks        map[string]*sync.Mutex
	locksMu      sync.Mutex
}

func NewFSCache(cacheDir string, linkStrategy LinkStrategy) *FSCache {
	if linkStrategy == "" {
		linkStrategy = LinkStrategyAuto
	}
	expanded := layout.ExpandPath(cacheDir)
	return &FSCache{
		cacheDir:     expanded,
		linkStrategy: linkStrategy,
		locks:        make(map[string]*sync.Mutex),
	}
}

func (c *FSCache) getLock(name string) *sync.Mutex {
	c.locksMu.Lock()
	defer c.locksMu.Unlock()
	if _, ok := c.locks[name]; !ok {
		c.locks[name] = &sync.Mutex{}
	}
	return c.locks[name]
}

func (c *FSCache) Lock(ctx context.Context, name string) (UnlockFunc, error) {
	lockPath := filepath.Join(c.cacheDir, "locks", name+".lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	// Try to acquire flock with timeout
	acquired := make(chan error, 1)
	go func() {
		acquired <- flock(f)
	}()

	select {
	case err := <-acquired:
		if err != nil {
			f.Close()
			return nil, err
		}
		return func() error {
			funlock(f)
			f.Close()
			return nil
		}, nil
	case <-ctx.Done():
		f.Close()
		return nil, ctx.Err()
	case <-time.After(30 * time.Second):
		f.Close()
		return nil, ErrLockTimeout
	}
}

func flock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func funlock(f *os.File) {
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

func (c *FSCache) moduleCacheDir(name, version string) string {
	return filepath.Join(c.cacheDir, "modules", name, version)
}

func (c *FSCache) moduleSourceDir(name, version string) string {
	return filepath.Join(c.moduleCacheDir(name, version), "modules", name)
}

func (c *FSCache) acoreCacheDir(commit string) string {
	return filepath.Join(c.cacheDir, "acore", commit)
}

func (c *FSCache) indexPath() string {
	return filepath.Join(c.cacheDir, "index.json")
}

func (c *FSCache) GetModule(ctx context.Context, name, version string) (string, error) {
	sourceDir := c.moduleSourceDir(name, version)
	if _, err := os.Stat(sourceDir); err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	return sourceDir, nil
}

func (c *FSCache) PutModule(ctx context.Context, name, version, repoURL, commit string) error {
	cacheDir := c.moduleCacheDir(name, version)
	gitDir := filepath.Join(cacheDir, ".git")

	if c.HasModule(name, version) {
		if err := c.VerifyModule(name, version); err == nil {
			return nil
		}
	}

	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			return fmt.Errorf("create cache dir: %w", err)
		}
		cmd := exec.CommandContext(ctx, "git", "clone", "--bare", repoURL, gitDir)
		cmd.Dir = cacheDir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git clone --bare: %w: %s", err, out)
		}
	}

	worktreeDir := filepath.Join(cacheDir, version)
	if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
		cmd := exec.CommandContext(ctx, "git", "-C", gitDir, "worktree", "add", worktreeDir, commit)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git worktree add: %w: %s", err, out)
		}
	} else {
		cmd := exec.CommandContext(ctx, "git", "-C", worktreeDir, "fetch", "origin", commit)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git fetch: %w: %s", err, out)
		}
		cmd = exec.CommandContext(ctx, "git", "-C", worktreeDir, "reset", "--hard", commit)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git reset: %w: %s", err, out)
		}
	}

	return nil
}

func (c *FSCache) HasModule(name, version string) bool {
	sourceDir := c.moduleSourceDir(name, version)
	_, err := os.Stat(sourceDir)
	return err == nil
}

func (c *FSCache) VerifyModule(name, version string) error {
	sourceDir := c.moduleSourceDir(name, version)
	cmd := exec.Command("git", "-C", sourceDir, "status", "--porcelain")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if len(out) > 0 {
		return errors.New("module has uncommitted changes")
	}
	return nil
}

func (c *FSCache) RemoveModule(name, version string) error {
	cacheDir := c.moduleCacheDir(name, version)
	return os.RemoveAll(cacheDir)
}

func (c *FSCache) GetACore(ctx context.Context, commit string) (string, error) {
	cacheDir := c.acoreCacheDir(commit)
	if _, err := os.Stat(cacheDir); err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	return cacheDir, nil
}

func (c *FSCache) PutACore(ctx context.Context, commit, repoURL string) error {
	cacheDir := c.acoreCacheDir(commit)
	gitDir := filepath.Join(cacheDir, ".git")

	if c.HasACore(commit) {
		return nil
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("create acore cache dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "clone", "--bare", repoURL, gitDir)
	cmd.Dir = cacheDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone --bare acore: %w: %s", err, out)
	}

	worktreeDir := filepath.Join(cacheDir, "worktree")
	cmd = exec.CommandContext(ctx, "git", "-C", gitDir, "worktree", "add", worktreeDir, commit)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add acore: %w: %s", err, out)
	}

	return nil
}

func (c *FSCache) HasACore(commit string) bool {
	cacheDir := c.acoreCacheDir(commit)
	_, err := os.Stat(cacheDir)
	return err == nil
}

func (c *FSCache) GetIndex() (*catalogue.Index, error) {
	data, err := os.ReadFile(c.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var cached struct {
		Index *catalogue.Index `json:"index"`
		ETag  string           `json:"etag"`
	}
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, err
	}
	return cached.Index, nil
}

func (c *FSCache) PutIndex(idx *catalogue.Index) error {
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		return err
	}

	cached := struct {
		Index *catalogue.Index `json:"index"`
		ETag  string           `json:"etag"`
	}{
		Index: idx,
		ETag:  "",
	}

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.indexPath(), data, 0644)
}

func (c *FSCache) LinkStrategy() LinkStrategy {
	return c.linkStrategy
}
