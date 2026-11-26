package commands

import (
	"os"
	"path/filepath"

	"github.com/rjpw/teeter/internal/buildctx"
	"github.com/rjpw/teeter/internal/compose"
	"gopkg.in/yaml.v3"
)

func writeEnvFiles(buildCtxPath string, ef compose.EnvFiles) error {
	// Use the new WriteEnvFiles which also generates dbimport.conf, authserver.conf, worldserver.conf
	bc := &buildctx.BuildContext{
		Path: buildCtxPath,
	}
	return compose.WriteEnvFiles(bc, ef)
}

func writeSQLFiles(buildCtxPath string, sf compose.SQLFiles) error {
	sqlDir := filepath.Join(buildCtxPath, "sql")
	if err := os.MkdirAll(sqlDir, 0755); err != nil {
		return err
	}
	combinedPath := filepath.Join(sqlDir, "teeter-combined.sql")
	return os.WriteFile(combinedPath, []byte(sf.CombinedSQL), 0644)
}

func writeDockerfile(buildCtxPath string, dockerfile string) error {
	dockerfilePath := filepath.Join(buildCtxPath, "Dockerfile.teeter")
	return os.WriteFile(dockerfilePath, []byte(dockerfile), 0644)
}

func marshalCompose(config compose.ComposeConfig) ([]byte, error) {
	return yaml.Marshal(config)
}

func getProfiles(services map[string]compose.Service) []string {
	profileSet := make(map[string]bool)
	for _, svc := range services {
		for _, p := range svc.Profiles {
			profileSet[p] = true
		}
	}
	profiles := make([]string, 0, len(profileSet))
	for p := range profileSet {
		profiles = append(profiles, p)
	}
	return profiles
}
