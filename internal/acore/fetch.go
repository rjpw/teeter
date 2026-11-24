package acore

import (
	"context"

	"github.com/rjpw/teeter/internal/cache"
	"github.com/rjpw/teeter/internal/config"
)

// FetchACore fetches the AC source to cache and returns the path to the worktree.
// The worktree is at cache/acore/<commit>/worktree/.
func FetchACore(ctx context.Context, cfg config.Config, commit, repo string, c cache.Cache) (string, error) {
	if err := c.PutACore(ctx, commit, repo); err != nil {
		return "", err
	}
	return c.GetACore(ctx, commit)
}
