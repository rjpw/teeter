package compose

import (
	"fmt"
	"path/filepath"

	"github.com/rjpw/teeter/internal/buildctx"
	"github.com/rjpw/teeter/internal/manifest"
)

// ServiceGenerator generates docker-compose services from build context and manifest.
type ServiceGenerator struct {
	BuildContext *buildctx.BuildContext
	Manifest     *manifest.Manifest
}

// GenerateServices generates all services for the compose config.
// New architecture: only worldserver is custom-built; other services use upstream AC images.
func (g *ServiceGenerator) GenerateServices() (map[string]Service, error) {
	buildCtxPath := g.BuildContext.Path
	hash := g.BuildContext.Hash
	if len(hash) > 8 {
		hash = hash[:8]
	}
	// Volume names use project name (stable across build context changes)
	projectName := g.Manifest.Project.Name
	volName := func(base string) string {
		return fmt.Sprintf("teeter-%s-%s", projectName, base)
	}
	// Default to AC's master tag; manifest can override via compose.build_strategy or env
	dockerImageTag := g.Manifest.Compose.BuildStrategy
	if dockerImageTag == "" {
		dockerImageTag = "master"
	}
	// If build_strategy is "prebuilt" or "auto", use tag; if "local", we build worldserver locally
	// The worldserver image tag is content-addressable
	worldserverImage := fmt.Sprintf("teeter/acore-worldserver:%s", hash)

	services := make(map[string]Service)

	// ac-database - MySQL (using root user per AC official pattern)
	services["ac-database"] = Service{
		Image: "mysql:8.4",
		Command: []string{"mysqld", "--bind-address=0.0.0.0"},
		Environment: []string{
			"MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD}",
		},
		Volumes: []string{
			fmt.Sprintf("%s:/var/lib/mysql", volName("mysql-data")),
		},
		HealthCheck: &HealthCheck{
			Test:        []string{"CMD", "mysqladmin", "ping", "-h", "127.0.0.1", "-u", "root", "-p${MYSQL_ROOT_PASSWORD}"},
			Interval:    "10s",
			Timeout:     "5s",
			Retries:     10,
			StartPeriod: "30s",
		},
		Restart:  "unless-stopped",
		Networks: []string{"ac-network"},
		Profiles: []string{"core"},
	}

	// ac-db-import - oneshot database initialization (upstream image)
	services["ac-db-import"] = Service{
		Image:   fmt.Sprintf("acore/ac-wotlk-db-import:%s", dockerImageTag),
		Command: []string{"dbimport"},
		EnvFile: []string{".env", "env.ac"},
		Volumes: []string{
			fmt.Sprintf("%s:/var/lib/mysql", volName("mysql-data")),
			fmt.Sprintf("%s:/azerothcore/env/dist/data", volName("extracted-data")),
			"./sql/teeter-dbimport.conf:/azerothcore/env/dist/etc/dbimport.conf:ro",
			"./sql/teeter-combined.sql:/azerothcore/data/custom.sql:ro",
		},
		DependsOn: map[string]DependsOnConfig{
			"ac-database": {Condition: "service_healthy"},
		},
		Networks: []string{"ac-network"},
		Profiles: []string{"core"},
		Restart:  "no",
	}

	// ac-authserver (upstream image)
	services["ac-authserver"] = Service{
		Image:   fmt.Sprintf("acore/ac-wotlk-authserver:%s", dockerImageTag),
		Command: []string{"authserver"},
		EnvFile: []string{".env", "env.ac"},
		Ports:   []string{"3724:3724"},
		Volumes: []string{
			"./sql/teeter-authserver.conf:/azerothcore/env/dist/etc/authserver.conf:ro",
			fmt.Sprintf("%s:/azerothcore/env/dist/data:ro", volName("extracted-data")),
		},
		DependsOn: map[string]DependsOnConfig{
			"ac-database":  {Condition: "service_healthy"},
			"ac-db-import": {Condition: "service_completed_successfully"},
		},
		Restart:  "unless-stopped",
		Networks: []string{"ac-network"},
		Profiles: []string{"core"},
	}

	// ac-worldserver - CUSTOM BUILT from build context
	services["ac-worldserver"] = Service{
		Image: worldserverImage,
		Build: &BuildConfig{
			Context:    filepath.Join(buildCtxPath, "azerothcore"),
			Dockerfile: "apps/docker/Dockerfile",
			Target:     "worldserver",
		},
		Command: []string{"worldserver"},
		EnvFile: []string{".env", "env.ac"},
		Ports:   []string{"8085:8085"},
		Volumes: []string{
			"./sql/teeter-worldserver.conf:/azerothcore/env/dist/etc/worldserver.conf:ro",
			fmt.Sprintf("%s:/azerothcore/env/dist/data:ro", volName("extracted-data")),
		},
		DependsOn: map[string]DependsOnConfig{
			"ac-database":         {Condition: "service_healthy"},
			"ac-db-import":        {Condition: "service_completed_successfully"},
			"ac-authserver":       {Condition: "service_started"},
			"ac-client-data-init": {Condition: "service_completed_successfully"},
		},
		Restart:  "unless-stopped",
		Networks: []string{"ac-network"},
		Profiles: []string{"core"},
	}

	// ac-client-data-init - oneshot client data extraction (upstream image)
	services["ac-client-data-init"] = Service{
		Image:   fmt.Sprintf("acore/ac-wotlk-client-data:%s", dockerImageTag),
		Command: []string{"bash", "-c", "source /azerothcore/apps/installer/includes/functions.sh && inst_download_client_data"},
		EnvFile: []string{".env", "env.ac"},
		Volumes: []string{
			fmt.Sprintf("%s:/wow-client:ro", volName("client-data")),
			fmt.Sprintf("%s:/azerothcore/env/dist/data", volName("extracted-data")),
		},
		DependsOn: map[string]DependsOnConfig{
			"ac-db-import": {Condition: "service_completed_successfully"},
		},
		Networks: []string{"ac-network"},
		Profiles: []string{"core"},
		Restart:  "no",
	}

	// ac-tools - development tools (upstream image)
	services["ac-tools"] = Service{
		Image:   fmt.Sprintf("acore/ac-wotlk-tools:%s", dockerImageTag),
		Command: []string{"sleep", "infinity"},
		EnvFile: []string{".env", "env.ac"},
		Volumes: []string{
			fmt.Sprintf("%s:/azerothcore/env/dist/etc", volName("config")),
			fmt.Sprintf("%s:/azerothcore/env/dist/data:ro", volName("extracted-data")),
		},
		Networks: []string{"ac-network"},
		Profiles: []string{"tools"},
		Restart:  "no",
	}

	// ac-dev-server - NOT included (Phase 9 uses bind mounts + Dockerfile.dev-server)

	// Apply manifest service extensions
	if g.Manifest != nil && g.Manifest.Compose.Services != nil {
		for name, ext := range g.Manifest.Compose.Services {
			if svc, ok := services[name]; ok {
				services[name] = g.applyServiceExtension(svc, ext)
			}
		}
	}

	return services, nil
}

// applyServiceExtension applies a ServiceExtension to a base Service.
func (g *ServiceGenerator) applyServiceExtension(base Service, ext manifest.ServiceExtension) Service {
	if ext.Image != "" {
		base.Image = ext.Image
	}
	if ext.Build != nil {
		base.Build = &BuildConfig{
			Context:    ext.Build.Context,
			Dockerfile: ext.Build.Dockerfile,
			Args:       ext.Build.Args,
		}
	}
	if ext.Environment != nil {
		if base.Environment == nil {
			base.Environment = []string{}
		}
		for k, v := range ext.Environment {
			base.Environment = append(base.Environment, fmt.Sprintf("%s=%s", k, v))
		}
	}
	if ext.Volumes != nil {
		base.Volumes = append(base.Volumes, ext.Volumes...)
	}
	if ext.Ports != nil {
		base.Ports = append(base.Ports, ext.Ports...)
	}
	if ext.DependsOn != nil {
		if base.DependsOn == nil {
			base.DependsOn = map[string]DependsOnConfig{}
		}
		for _, dep := range ext.DependsOn {
			base.DependsOn[dep] = DependsOnConfig{Condition: "service_healthy"}
		}
	}
	if ext.Profiles != nil {
		base.Profiles = append(base.Profiles, ext.Profiles...)
	}
	return base
}

// GenerateNetworks generates the networks configuration.
func (g *ServiceGenerator) GenerateNetworks() map[string]Network {
	return map[string]Network{
		"ac-network": {
			Driver: "bridge",
		},
	}
}

// GenerateVolumes generates the volumes configuration.
func (g *ServiceGenerator) GenerateVolumes() map[string]Volume {
	projectName := g.Manifest.Project.Name
	volName := func(base string) string {
		return fmt.Sprintf("teeter-%s-%s", projectName, base)
	}
	return map[string]Volume{
		volName("mysql-data"):     {Driver: "local"},
		volName("extracted-data"): {Driver: "local"},
		volName("config"):         {Driver: "local"},
		volName("client-data"):    {Driver: "local"},
	}
}
