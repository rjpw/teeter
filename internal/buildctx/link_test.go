package buildctx

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rjpw/teeter/internal/cache"
	"github.com/rjpw/teeter/internal/catalogue"
	"github.com/rjpw/teeter/internal/manifest"
)

func TestLinkModules(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock cache with module source
	srcCacheDir := filepath.Join(tmpDir, "cache", "modules", "mod-test", "v1.0.0", "modules", "mod-test")
	os.MkdirAll(filepath.Join(srcCacheDir, "src"), 0755)
	os.WriteFile(filepath.Join(srcCacheDir, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.10)"), 0644)
	os.WriteFile(filepath.Join(srcCacheDir, "src", "test.cpp"), []byte("// test"), 0644)

	mockCache := &mockCacheWithModule{
		moduleSrc: filepath.Join(tmpDir, "cache", "modules", "mod-test", "v1.0.0"),
	}

	m := &manifest.Manifest{
		Version: "1",
		Project: manifest.Project{Name: "test"},
		ACore: manifest.ACoreSpec{
			Repo:   "https://github.com/example/acore",
			Branch: "main",
			Commit: "abc123",
		},
		Modules: []manifest.ModuleSpec{
			{Name: "mod-test", Version: "v1.0.0", Source: "catalogue"},
		},
	}

	res := &catalogue.Resolution{
		ACCommit:   "abc123",
		ACRepo:     "https://github.com/example/acore",
		BuildOrder: []string{"mod-test"},
		Modules: map[string]catalogue.ResolvedModule{
			"mod-test": {Version: "v1.0.0", Commit: "def456"},
		},
	}

	bc := &BuildContext{
		Path:        filepath.Join(tmpDir, "buildctx", "hash123"),
		ModulePaths: make(map[string]string),
		Manifest:    m,
		Resolution:  res,
		Hash:        "hash123",
	}

	// Create build context directories
	os.MkdirAll(filepath.Join(bc.Path, "modules"), 0755)

	err := LinkModules(context.Background(), bc, mockCache, cache.LinkStrategyAuto)
	if err != nil {
		t.Fatalf("LinkModules failed: %v", err)
	}

	// Verify module was linked
	linkedPath := bc.ModulePaths["mod-test"]
	if linkedPath == "" {
		t.Fatal("module path not recorded")
	}

	// Check structure
	for _, f := range []string{"CMakeLists.txt", filepath.Join("src", "test.cpp")} {
		fullPath := filepath.Join(linkedPath, f)
		if _, err := os.Stat(fullPath); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
}

func TestVerifyModuleStructure(t *testing.T) {
	tmpDir := t.TempDir()

	// Valid module
	validDir := filepath.Join(tmpDir, "valid")
	os.MkdirAll(filepath.Join(validDir, "src"), 0755)
	os.WriteFile(filepath.Join(validDir, "CMakeLists.txt"), []byte("cmake"), 0644)

	err := verifyModuleStructure(validDir)
	if err != nil {
		t.Errorf("valid module failed: %v", err)
	}

	// Missing CMakeLists.txt
	missingCMake := filepath.Join(tmpDir, "missing-cmake")
	os.MkdirAll(filepath.Join(missingCMake, "src"), 0755)
	err = verifyModuleStructure(missingCMake)
	if err == nil {
		t.Error("expected error for missing CMakeLists.txt")
	}

	// Missing src/
	missingSrc := filepath.Join(tmpDir, "missing-src")
	os.MkdirAll(missingSrc, 0755)
	os.WriteFile(filepath.Join(missingSrc, "CMakeLists.txt"), []byte("cmake"), 0644)
	err = verifyModuleStructure(missingSrc)
	if err == nil {
		t.Error("expected error for missing src/")
	}
}

type mockCacheWithModule struct {
	moduleSrc string
}

func (m *mockCacheWithModule) GetModule(ctx context.Context, name, version string) (string, error) {
	return m.moduleSrc, nil
}
func (m *mockCacheWithModule) PutModule(ctx context.Context, name, version, repoURL, commit string) error {
	return nil
}
func (m *mockCacheWithModule) HasModule(name, version string) bool     { return false }
func (m *mockCacheWithModule) VerifyModule(name, version string) error { return nil }
func (m *mockCacheWithModule) RemoveModule(name, version string) error { return nil }
func (m *mockCacheWithModule) GetACore(ctx context.Context, commit string) (string, error) {
	return "", cache.ErrNotFound
}
func (m *mockCacheWithModule) PutACore(ctx context.Context, commit, repoURL string) error { return nil }
func (m *mockCacheWithModule) HasACore(commit string) bool                                { return false }
func (m *mockCacheWithModule) GetIndex() (*catalogue.Index, error)                        { return nil, nil }
func (m *mockCacheWithModule) PutIndex(idx *catalogue.Index) error                        { return nil }
func (m *mockCacheWithModule) Lock(ctx context.Context, name string) (cache.UnlockFunc, error) {
	return func() error { return nil }, nil
}
func (m *mockCacheWithModule) LinkStrategy() cache.LinkStrategy { return cache.LinkStrategyAuto }
