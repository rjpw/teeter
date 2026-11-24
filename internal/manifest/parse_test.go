package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testdataDir = "../../testdata/manifests"

func TestParseFile_ValidMinimal(t *testing.T) {
	m, err := ParseFile(filepath.Join(testdataDir, "valid_minimal.yaml"))
	require.NoError(t, err)

	assert.Equal(t, "1", m.Version)
	assert.Equal(t, "my-solo-server", m.Project.Name)
	assert.Equal(t, "https://github.com/mod-playerbots/azerothcore-wotlk", m.ACore.Repo)
	assert.Equal(t, "Playerbot", m.ACore.Branch)
	assert.Len(t, m.Modules, 0)
	assert.Equal(t, "local", m.ClientData.Source)
	assert.Equal(t, "~/wow-client", m.ClientData.Path)
}

func TestParseFile_ValidFull(t *testing.T) {
	m, err := ParseFile(filepath.Join(testdataDir, "valid_full.yaml"))
	require.NoError(t, err)

	assert.Equal(t, "1", m.Version)
	assert.Equal(t, "my-full-server", m.Project.Name)
	assert.Equal(t, "Full featured server with multiple modules", m.Project.Description)
	assert.Equal(t, "a1b2c3d4e5f6", m.ACore.Commit)
	assert.False(t, m.ACore.Shallow)
	assert.Len(t, m.Modules, 3)
	assert.Equal(t, "mod-playerbots", m.Modules[0].Name)
	assert.Equal(t, "catalogue", m.Modules[0].Source)
	assert.Equal(t, "master", m.Modules[0].Version)
	assert.Len(t, m.Modules[0].Patches, 1)
	assert.Equal(t, "patches/playerbots-fix.patch", m.Modules[0].Patches[0].Path)
	assert.Equal(t, "src", m.Modules[0].Patches[0].Target)
	assert.Equal(t, "mod-dungeon-clear", m.Modules[1].Name)
	assert.Equal(t, "mod-playerbots", m.Modules[1].DependsOn[0])
	assert.Equal(t, "mod-ah-bot", m.Modules[2].Name)
	assert.Equal(t, "git", m.Modules[2].Source)
	assert.Equal(t, "main", m.Modules[2].Version)
	assert.Equal(t, "https://github.com/someuser/ah-bot", m.Modules[2].Path)
	assert.Len(t, m.Database.Migrations, 2)
	assert.Len(t, m.Database.Seed, 1)
	assert.True(t, m.ClientData.ExtractMaps)
	assert.True(t, m.ClientData.ExtractVmaps)
	assert.True(t, m.ClientData.ExtractMmaps)
	assert.Equal(t, "custom_value", m.Environment.Vars["CUSTOM_VAR"])
	assert.Contains(t, m.Environment.Secrets, "MYSQL_ROOT_PASSWORD")
	assert.Equal(t, "ac-network", m.Compose.Network)
	// Compose.Volumes: key="/host/logs" value="/azerothcore/env/dist/logs"
	assert.Equal(t, "/azerothcore/env/dist/logs", m.Compose.Volumes["/host/logs"])
	assert.Contains(t, m.Compose.Profiles, "core")
	assert.Contains(t, m.Compose.Profiles, "tools")
	assert.NotNil(t, m.Compose.Services["ac-worldserver"])
}

func TestParseFile_ValidNoClient(t *testing.T) {
	m, err := ParseFile(filepath.Join(testdataDir, "valid_no_client.yaml"))
	require.NoError(t, err)

	assert.Equal(t, "1", m.Version)
	assert.Equal(t, "ci-server", m.Project.Name)
	assert.Equal(t, "local", m.ClientData.Source) // default applied
	assert.Empty(t, m.ClientData.Path)
}

func TestParseFile_NotFound(t *testing.T) {
	_, err := ParseFile(filepath.Join(testdataDir, "nonexistent.yaml"))
	require.Error(t, err)
	var pe *ParseError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, filepath.Join(testdataDir, "nonexistent.yaml"), pe.Path)
}

func TestParseFile_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	invalidYAML := filepath.Join(tmpDir, "invalid.yaml")
	require.NoError(t, os.WriteFile(invalidYAML, []byte("invalid: yaml: ["), 0644))

	_, err := ParseFile(invalidYAML)
	require.Error(t, err)
	var pe *ParseError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, invalidYAML, pe.Path)
}

func TestParseBytes_DefaultsApplied(t *testing.T) {
	yamlData := []byte(`
version: "1"
project:
  name: "test"
acore:
  repo: "https://github.com/test/repo"
  branch: "main"
modules:
  - name: "mod-a"
client_data:
  source: "local"
  path: "/path"
`)
	m, err := ParseBytes(yamlData)
	require.NoError(t, err)

	assert.Equal(t, "catalogue", m.Modules[0].Source) // default applied
	assert.Equal(t, "local", m.ClientData.Source)
}

func TestParseBytes_InvalidVersion(t *testing.T) {
	yamlData := []byte(`
version: "2"
project:
  name: "test"
acore:
  repo: "https://github.com/test/repo"
  branch: "main"
modules: []
client_data:
  source: "local"
  path: "/path"
`)
	m, err := ParseBytes(yamlData)
	require.NoError(t, err) // Parser doesn't validate version
	assert.Equal(t, "2", m.Version)
	// Validation would catch this
	err = Validate(m, ".")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MANIFEST_VERSION")
}
