package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validateTestdataDir = "../../testdata/manifests"

func TestValidate_ValidMinimal(t *testing.T) {
	m, err := ParseFile(filepath.Join(validateTestdataDir, "valid_minimal.yaml"))
	require.NoError(t, err)

	// Need to create a temp wow-client dir for the test
	tmpDir := t.TempDir()
	wowClient := filepath.Join(tmpDir, "wow-client")
	require.NoError(t, os.MkdirAll(wowClient, 0755))

	// Update path to point to temp dir
	m.ClientData.Path = wowClient

	err = Validate(m, tmpDir)
	assert.NoError(t, err)
}

func TestValidate_ValidFull(t *testing.T) {
	m, err := ParseFile(filepath.Join(validateTestdataDir, "valid_full.yaml"))
	require.NoError(t, err)

	// Create temp directories for paths that need to exist
	tmpDir := t.TempDir()
	wowClient := filepath.Join(tmpDir, "wow-client")
	require.NoError(t, os.MkdirAll(wowClient, 0755))
	patchDir := filepath.Join(tmpDir, "patches")
	require.NoError(t, os.MkdirAll(patchDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(patchDir, "playerbots-fix.patch"), []byte(""), 0644))
	sqlDir := filepath.Join(tmpDir, "sql", "custom")
	require.NoError(t, os.MkdirAll(sqlDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sqlDir, "migration_001.sql"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(sqlDir, "migration_002.sql"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(sqlDir, "seed_data.sql"), []byte(""), 0644))
	hostLogs := filepath.Join(tmpDir, "host", "logs")
	require.NoError(t, os.MkdirAll(hostLogs, 0755))

	// Update paths
	m.ClientData.Path = wowClient
	m.Modules[0].Patches[0].Path = filepath.Join(patchDir, "playerbots-fix.patch")
	m.Database.Migrations[0] = filepath.Join(sqlDir, "migration_001.sql")
	m.Database.Migrations[1] = filepath.Join(sqlDir, "migration_002.sql")
	m.Database.Seed[0] = filepath.Join(sqlDir, "seed_data.sql")
	m.Compose.Volumes["/host/logs"] = "/azerothcore/env/dist/logs"

	err = Validate(m, tmpDir)
	assert.NoError(t, err)
}

func TestValidate_InvalidVersion(t *testing.T) {
	m, err := ParseFile(filepath.Join(validateTestdataDir, "invalid_version.yaml"))
	require.NoError(t, err)

	err = Validate(m, validateTestdataDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MANIFEST_VERSION")
	assert.Contains(t, err.Error(), "unsupported version")
}

func TestValidate_InvalidCycle(t *testing.T) {
	m, err := ParseFile(filepath.Join(validateTestdataDir, "invalid_cycle.yaml"))
	require.NoError(t, err)

	err = Validate(m, validateTestdataDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MANIFEST_CYCLE")
	// Cycle detection may output "mod-a -> mod-b" or "mod-a -> mod-b -> mod-a"
	assert.Contains(t, err.Error(), "mod-a")
	assert.Contains(t, err.Error(), "mod-b")
}

func TestValidate_InvalidMissingRef(t *testing.T) {
	m, err := ParseFile(filepath.Join(validateTestdataDir, "invalid_missing_ref.yaml"))
	require.NoError(t, err)

	err = Validate(m, validateTestdataDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MANIFEST_REF")
	assert.Contains(t, err.Error(), "unknown module reference")
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestValidate_InvalidBadACore(t *testing.T) {
	m, err := ParseFile(filepath.Join(validateTestdataDir, "invalid_bad_acore.yaml"))
	require.NoError(t, err)

	err = Validate(m, validateTestdataDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MANIFEST_INVALID")
	assert.Contains(t, err.Error(), "acore.repo")
	assert.Contains(t, err.Error(), "valid Git URL")
}

func TestValidate_InvalidMissingMigration(t *testing.T) {
	m, err := ParseFile(filepath.Join(validateTestdataDir, "invalid_missing_migration.yaml"))
	require.NoError(t, err)

	err = Validate(m, validateTestdataDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MANIFEST_IO")
	assert.Contains(t, err.Error(), "file not found")
}

func TestValidate_InvalidDuplicateModule(t *testing.T) {
	m, err := ParseFile(filepath.Join(validateTestdataDir, "invalid_duplicate_module.yaml"))
	require.NoError(t, err)

	err = Validate(m, validateTestdataDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MANIFEST_INVALID")
	assert.Contains(t, err.Error(), "duplicate module name")
}

func TestValidate_MissingProjectName(t *testing.T) {
	yamlData := []byte(`
version: "1"
project: {}
acore:
  repo: "https://github.com/test/repo"
  branch: "main"
modules: []
client_data:
  source: "local"
  path: "/path"
`)
	m, err := ParseBytes(yamlData)
	require.NoError(t, err)

	err = Validate(m, ".")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project.name")
	assert.Contains(t, err.Error(), "required")
}

func TestValidate_InvalidProjectName(t *testing.T) {
	yamlData := []byte(`
version: "1"
project:
  name: "invalid name!"
acore:
  repo: "https://github.com/test/repo"
  branch: "main"
modules: []
client_data:
  source: "local"
  path: "/path"
`)
	m, err := ParseBytes(yamlData)
	require.NoError(t, err)

	err = Validate(m, ".")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project.name")
	assert.Contains(t, err.Error(), "must match")
}

func TestValidate_MissingACoreBranchOrCommit(t *testing.T) {
	yamlData := []byte(`
version: "1"
project:
  name: "test"
acore:
  repo: "https://github.com/test/repo"
modules: []
client_data:
  source: "local"
  path: "/path"
`)
	m, err := ParseBytes(yamlData)
	require.NoError(t, err)

	err = Validate(m, ".")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "acore")
	assert.Contains(t, err.Error(), "branch or commit must be specified")
}

func TestValidate_InvalidModuleSource(t *testing.T) {
	yamlData := []byte(`
version: "1"
project:
  name: "test"
acore:
  repo: "https://github.com/test/repo"
  branch: "main"
modules:
  - name: "mod-a"
    source: "invalid"
client_data:
  source: "local"
  path: "/path"
`)
	m, err := ParseBytes(yamlData)
	require.NoError(t, err)

	err = Validate(m, ".")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "modules[0].source")
	assert.Contains(t, err.Error(), "invalid source")
}

func TestValidate_ClientDataLocalMissingPath(t *testing.T) {
	yamlData := []byte(`
version: "1"
project:
  name: "test"
acore:
  repo: "https://github.com/test/repo"
  branch: "main"
modules: []
client_data:
  source: "local"
  path: ""
`)
	m, err := ParseBytes(yamlData)
	require.NoError(t, err)

	err = Validate(m, ".")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client_data.path")
	assert.Contains(t, err.Error(), "required when source is local")
}

func TestValidate_ClientDataLocalPathNotFound(t *testing.T) {
	yamlData := []byte(`
version: "1"
project:
  name: "test"
acore:
  repo: "https://github.com/test/repo"
  branch: "main"
modules: []
client_data:
  source: "local"
  path: "/nonexistent/path"
`)
	m, err := ParseBytes(yamlData)
	require.NoError(t, err)

	err = Validate(m, ".")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MANIFEST_IO")
	assert.Contains(t, err.Error(), "path not found")
}

func TestValidate_InvalidClientDataSource(t *testing.T) {
	yamlData := []byte(`
version: "1"
project:
  name: "test"
acore:
  repo: "https://github.com/test/repo"
  branch: "main"
modules: []
client_data:
  source: "invalid"
`)
	m, err := ParseBytes(yamlData)
	require.NoError(t, err)

	err = Validate(m, ".")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client_data.source")
	assert.Contains(t, err.Error(), "invalid source")
}
