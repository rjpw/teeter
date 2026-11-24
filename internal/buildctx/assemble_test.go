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

func TestComputeHash(t *testing.T) {
	m := &manifest.Manifest{
		Project: manifest.Project{Name: "test-project"},
		ACore: manifest.ACoreSpec{
			Repo:   "https://github.com/example/acore",
			Branch: "main",
		},
		Modules: []manifest.ModuleSpec{
			{Name: "mod-a", Patches: []manifest.PatchSpec{{Path: "patch1.patch"}}},
			{Name: "mod-b"},
		},
		Database: manifest.DatabaseSpec{
			Migrations: []string{"sql/mig1.sql"},
			Seed:       []string{"sql/seed1.sql"},
		},
	}

	res := &catalogue.Resolution{
		ACCommit:   "abc123",
		ACRepo:     "https://github.com/example/acore",
		BuildOrder: []string{"mod-a", "mod-b"},
		Modules: map[string]catalogue.ResolvedModule{
			"mod-a": {Commit: "def456"},
			"mod-b": {Commit: "ghi789"},
		},
	}

	hash1, err := computeHash(m, res)
	if err != nil {
		t.Fatalf("computeHash failed: %v", err)
	}

	// Same inputs should produce same hash
	hash2, err := computeHash(m, res)
	if err != nil {
		t.Fatalf("computeHash failed: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("hash not deterministic: %s != %s", hash1, hash2)
	}

	// Different AC commit should produce different hash
	res2 := *res
	res2.ACCommit = "different"
	hash3, _ := computeHash(m, &res2)
	if hash1 == hash3 {
		t.Errorf("hash should differ with different AC commit")
	}

	// Different module commit should produce different hash
	res3 := *res
	res3.Modules = map[string]catalogue.ResolvedModule{
		"mod-a": {Commit: "different"},
		"mod-b": {Commit: "ghi789"},
	}
	hash4, _ := computeHash(m, &res3)
	if hash1 == hash4 {
		t.Errorf("hash should differ with different module commit")
	}
}

func TestCreateDirs(t *testing.T) {
	tmpDir := t.TempDir()
	root := filepath.Join(tmpDir, "buildctx")

	err := createDirs(root)
	if err != nil {
		t.Fatalf("createDirs failed: %v", err)
	}

	expectedDirs := []string{
		"azerothcore",
		"modules",
		"patches",
		filepath.Join("sql", "custom"),
		filepath.Join("sql", "modules"),
		".teeter",
	}

	for _, dir := range expectedDirs {
		fullPath := filepath.Join(root, dir)
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Errorf("missing directory %s: %v", dir, err)
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
	}
}

func TestLinkDirHardlink(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src")
	dst := filepath.Join(tmpDir, "dst")

	// Create source structure
	os.MkdirAll(filepath.Join(src, "subdir"), 0755)
	os.WriteFile(filepath.Join(src, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(src, "subdir", "file2.txt"), []byte("content2"), 0644)

	err := linkDir(src, dst, cache.LinkStrategyHardlink)
	if err != nil {
		t.Fatalf("linkDir hardlink failed: %v", err)
	}

	// Verify structure - dst should be a directory with hardlinked files
	for _, f := range []string{"file1.txt", filepath.Join("subdir", "file2.txt")} {
		dstPath := filepath.Join(dst, f)
		info, err := os.Stat(dstPath)
		if err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
		if info.IsDir() {
			t.Errorf("%s should be a file", f)
		}

		// Verify content
		content, err := os.ReadFile(dstPath)
		if err != nil {
			t.Errorf("read %s: %v", f, err)
		}
		expected := "content1"
		if f == "subdir/file2.txt" {
			expected = "content2"
		}
		if string(content) != expected {
			t.Errorf("content mismatch for %s: got %q, want %q", f, string(content), expected)
		}
	}
}

func TestLinkDirSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src")
	dst := filepath.Join(tmpDir, "dst")

	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	err := linkDir(src, dst, cache.LinkStrategySymlink)
	if err != nil {
		t.Fatalf("linkDir symlink failed: %v", err)
	}

	info, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("dst is not a symlink")
	}

	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != src {
		t.Errorf("symlink target mismatch: got %s, want %s", target, src)
	}
}

func TestLinkDirAuto(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src")
	dst := filepath.Join(tmpDir, "dst")

	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	// Auto should try hardlink first, fall back to symlink
	err := linkDir(src, dst, cache.LinkStrategyAuto)
	if err != nil {
		t.Fatalf("linkDir auto failed: %v", err)
	}

	// Should have created something
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	if info.IsDir() {
		// Hardlink tree
		dstFile := filepath.Join(dst, "file.txt")
		if _, err := os.Stat(dstFile); err != nil {
			t.Errorf("hardlink file missing: %v", err)
		}
	} else if info.Mode()&os.ModeSymlink != 0 {
		// Symlink
	} else {
		t.Errorf("dst is neither dir nor symlink")
	}
}

func TestAssemble_ValidBuildContext(t *testing.T) {
	// This is an integration-style test that requires a cache
	// For now, test that Assemble returns an error when cache is not populated
	tmpDir := t.TempDir()
	m := &manifest.Manifest{
		Version: "1",
		Project: manifest.Project{Name: "test"},
		ACore: manifest.ACoreSpec{
			Repo:   "https://github.com/example/acore",
			Branch: "main",
			Commit: "abc123",
		},
		Modules: []manifest.ModuleSpec{},
	}

	// Create a mock cache that will fail
	mockCache := &mockCacheForTest{}
	res := &catalogue.Resolution{
		ACCommit:   "abc123",
		ACRepo:     "https://github.com/example/acore",
		BuildOrder: []string{},
		Modules:    map[string]catalogue.ResolvedModule{},
	}

	_, err := Assemble(context.Background(), m, res, mockCache, tmpDir)
	if err == nil {
		t.Errorf("expected error when cache not populated")
	}
}

// mockCacheForTest implements cache.Cache for testing
type mockCacheForTest struct{}

func (m *mockCacheForTest) GetModule(ctx context.Context, name, version string) (string, error) {
	return "", cache.ErrNotFound
}
func (m *mockCacheForTest) PutModule(ctx context.Context, name, version, repoURL, commit string) error {
	return nil
}
func (m *mockCacheForTest) HasModule(name, version string) bool     { return false }
func (m *mockCacheForTest) VerifyModule(name, version string) error { return nil }
func (m *mockCacheForTest) RemoveModule(name, version string) error { return nil }
func (m *mockCacheForTest) GetACore(ctx context.Context, commit string) (string, error) {
	return "", cache.ErrNotFound
}
func (m *mockCacheForTest) PutACore(ctx context.Context, commit, repoURL string) error { return nil }
func (m *mockCacheForTest) HasACore(commit string) bool                                { return false }
func (m *mockCacheForTest) GetIndex() (*catalogue.Index, error)                        { return nil, nil }
func (m *mockCacheForTest) PutIndex(idx *catalogue.Index) error                        { return nil }
func (m *mockCacheForTest) Lock(ctx context.Context, name string) (cache.UnlockFunc, error) {
	return func() error { return nil }, nil
}
func (m *mockCacheForTest) LinkStrategy() cache.LinkStrategy { return cache.LinkStrategyAuto }
