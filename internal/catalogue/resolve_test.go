package catalogue

import (
	"context"
	"testing"

	"github.com/rjpw/teeter/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_EmptyModules(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1",
		Project: manifest.Project{Name: "test"},
		ACore: manifest.ACoreSpec{
			Repo:   "https://github.com/test/acore",
			Branch: "main",
		},
		Modules: []manifest.ModuleSpec{},
		ClientData: manifest.ClientDataSpec{
			Source: "local",
			Path:   "/tmp/test",
		},
	}

	idx := &Index{
		Modules: make(map[string]ModuleEntry),
	}

	res, err := Resolve(context.Background(), m, idx, DefaultResolveOptions())
	require.NoError(t, err)
	assert.Equal(t, "main", res.ACCommit)
	assert.Equal(t, "https://github.com/test/acore", res.ACRepo)
	assert.Len(t, res.Modules, 0)
	assert.Len(t, res.BuildOrder, 0)
}

func TestResolve_LocalModule(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1",
		Project: manifest.Project{Name: "test"},
		ACore: manifest.ACoreSpec{
			Repo:   "https://github.com/test/acore",
			Branch: "main",
		},
		Modules: []manifest.ModuleSpec{
			{Name: "mod-local", Source: "local", Path: "/path/to/mod"},
		},
		ClientData: manifest.ClientDataSpec{
			Source: "local",
			Path:   "/tmp/test",
		},
	}

	idx := &Index{Modules: make(map[string]ModuleEntry)}

	res, err := Resolve(context.Background(), m, idx, DefaultResolveOptions())
	require.NoError(t, err)
	assert.Len(t, res.Modules, 1)
	rm := res.Modules["mod-local"]
	assert.Equal(t, "local", rm.Version)
	assert.Equal(t, "local", rm.Repository)
	assert.Equal(t, "/path/to/mod", rm.CachePath)
	assert.Equal(t, []string{}, rm.Dependencies)
	assert.Equal(t, []string{}, rm.Conflicts)
	assert.Equal(t, []string{"mod-local"}, res.BuildOrder)
}

func TestResolve_CatalogueModule(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1",
		Project: manifest.Project{Name: "test"},
		ACore: manifest.ACoreSpec{
			Repo:   "https://github.com/test/acore",
			Branch: "main",
		},
		Modules: []manifest.ModuleSpec{
			{Name: "mod-cat", Source: "catalogue", Version: "v1.0.0"},
		},
		ClientData: manifest.ClientDataSpec{
			Source: "local",
			Path:   "/tmp/test",
		},
	}

	idx := &Index{
		Modules: map[string]ModuleEntry{
			"mod-cat": {
				Name:          "mod-cat",
				RepositoryURL: "https://github.com/test/mod-cat",
				DefaultBranch: "main",
				Releases: []Release{
					{TagName: "v1.0.0", TargetSHA: "abc123def456", PublishedAt: "2024-01-01", Prerelease: false},
				},
				LatestRelease: &Release{TagName: "v1.0.0", TargetSHA: "abc123def456"},
				Dependencies:  []string{"mod-dep"},
			},
		},
	}

	res, err := Resolve(context.Background(), m, idx, DefaultResolveOptions())
	require.NoError(t, err)
	assert.Len(t, res.Modules, 1)
	rm := res.Modules["mod-cat"]
	assert.Equal(t, "v1.0.0", rm.Version)
	assert.Equal(t, "abc123def456", rm.Commit)
	assert.Equal(t, "https://github.com/test/mod-cat", rm.Repository)
	assert.Equal(t, []string{"mod-dep"}, rm.Dependencies)
}

func TestResolve_DependencyGraph(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1",
		Project: manifest.Project{Name: "test"},
		ACore: manifest.ACoreSpec{
			Repo:   "https://github.com/test/acore",
			Branch: "main",
		},
		Modules: []manifest.ModuleSpec{
			{Name: "mod-a", Source: "local", Path: "/a", DependsOn: []string{"mod-b"}},
			{Name: "mod-b", Source: "local", Path: "/b", DependsOn: []string{"mod-c"}},
			{Name: "mod-c", Source: "local", Path: "/c"},
		},
		ClientData: manifest.ClientDataSpec{
			Source: "local",
			Path:   "/tmp/test",
		},
	}

	idx := &Index{Modules: make(map[string]ModuleEntry)}

	res, err := Resolve(context.Background(), m, idx, DefaultResolveOptions())
	require.NoError(t, err)
	// Build order should be mod-c, mod-b, mod-a (topological sort)
	assert.Equal(t, []string{"mod-c", "mod-b", "mod-a"}, res.BuildOrder)
}

func TestResolve_CycleDetection(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1",
		Project: manifest.Project{Name: "test"},
		ACore: manifest.ACoreSpec{
			Repo:   "https://github.com/test/acore",
			Branch: "main",
		},
		Modules: []manifest.ModuleSpec{
			{Name: "mod-a", Source: "local", Path: "/a", DependsOn: []string{"mod-b"}},
			{Name: "mod-b", Source: "local", Path: "/b", DependsOn: []string{"mod-a"}},
		},
		ClientData: manifest.ClientDataSpec{
			Source: "local",
			Path:   "/tmp/test",
		},
	}

	idx := &Index{Modules: make(map[string]ModuleEntry)}

	_, err := Resolve(context.Background(), m, idx, DefaultResolveOptions())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependency cycle detected")
	assert.Contains(t, err.Error(), "mod-a")
	assert.Contains(t, err.Error(), "mod-b")
}

func TestResolve_ConflictDetection(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1",
		Project: manifest.Project{Name: "test"},
		ACore: manifest.ACoreSpec{
			Repo:   "https://github.com/test/acore",
			Branch: "main",
		},
		Modules: []manifest.ModuleSpec{
			{Name: "mod-a", Source: "catalogue"},
			{Name: "mod-b", Source: "catalogue"},
		},
		ClientData: manifest.ClientDataSpec{
			Source: "local",
			Path:   "/tmp/test",
		},
	}

	idx := &Index{
		Modules: map[string]ModuleEntry{
			"mod-a": {Name: "mod-a", RepositoryURL: "https://github.com/test/mod-a", Conflicts: []string{"mod-b"}},
			"mod-b": {Name: "mod-b", RepositoryURL: "https://github.com/test/mod-b"},
		},
	}

	_, err := Resolve(context.Background(), m, idx, DefaultResolveOptions())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicts with")
}
