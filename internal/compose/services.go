package compose

import (
	"fmt"

	"github.com/rjpw/teeter/internal/buildctx"
	"github.com/rjpw/teeter/internal/manifest"
)

// ServiceGenerator generates docker-compose services from build context and manifest.
type ServiceGenerator struct {
	BuildContext  *buildctx.BuildContext
	Manifest      *manifest.Manifest
	Registry      string
	BuildStrategy string
}

// ImageTags holds the computed image tags for each layer.
type ImageTags struct {
	Builder string
	Modules string
	Runtime string
}

// ComputeImageTags computes the image tags for the layered build strategy.
func (g *ServiceGenerator) ComputeImageTags() ImageTags {
	acCommit := g.BuildContext.Resolution.ACCommit
	if len(acCommit) > 7 {
		acCommit = acCommit[:7]
	}
	modulesHash := g.BuildContext.Hash
	if len(modulesHash) > 8 {
		modulesHash = modulesHash[:8]
	}
	fullHash := g.BuildContext.Hash
	if len(fullHash) > 12 {
		fullHash = fullHash[:12]
	}

	registry := g.Registry
	if registry == "" {
		registry = "teeter"
	}

	return ImageTags{
		Builder: fmt.Sprintf("%s/acore-builder:%s", registry, acCommit),
		Modules: fmt.Sprintf("%s/acore-modules:%s", registry, modulesHash),
		Runtime: fmt.Sprintf("%s/acore-runtime:%s", registry, fullHash),
	}
}

// GenerateServices generates all services for the compose config.
func (g *ServiceGenerator) GenerateServices() (map[string]Service, error) {
	tags := g.ComputeImageTags()
	runtimeImage := tags.Runtime

	services := make(map[string]Service)

	// ac-database - MySQL
	services["ac-database"] = Service{
		Image: "mysql:8.4",
		Environment: []string{
			"MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD}",
			"MYSQL_DATABASE=acore_world",
			"MYSQL_USER=acore",
			"MYSQL_PASSWORD=${MYSQL_PASSWORD}",
		},
		Volumes: []string{
			"mysql-data:/var/lib/mysql",
		},
		HealthCheck: &HealthCheck{
			Test:        []string{"CMD", "mysqladmin", "ping", "-h", "localhost", "-u", "acore", "-p${MYSQL_PASSWORD}"},
			Interval:    "10s",
			Timeout:     "5s",
			Retries:     10,
			StartPeriod: "30s",
		},
		Restart:  "unless-stopped",
		Networks: []string{"ac-network"},
		Profiles: []string{"core"},
	}

	// ac-db-import - oneshot database initialization
	services["ac-db-import"] = Service{
		Image:   runtimeImage,
		Command: []string{"/entrypoint.sh", "db-import"},
		EnvFile: []string{".env", "env.ac"},
		Volumes: []string{
			"mysql-data:/var/lib/mysql",
			"extracted-data:/azerothcore/env/dist/data",
		},
		DependsOn: map[string]DependsOnConfig{
			"ac-database": {Condition: "service_healthy"},
		},
		Networks: []string{"ac-network"},
		Profiles: []string{"core"},
		Restart:  "no",
	}

	// ac-authserver
	services["ac-authserver"] = Service{
		Image:   runtimeImage,
		Command: []string{"/entrypoint.sh", "authserver"},
		EnvFile: []string{".env", "env.ac"},
		Ports:   []string{"3724:3724"},
		Volumes: []string{
			"config:/azerothcore/env/dist/etc",
			"extracted-data:/azerothcore/env/dist/data:ro",
		},
		DependsOn: map[string]DependsOnConfig{
			"ac-database":  {Condition: "service_healthy"},
			"ac-db-import": {Condition: "service_completed_successfully"},
		},
		HealthCheck: &HealthCheck{
			Test:     []string{"CMD", "nc", "-z", "localhost", "3724"},
			Interval: "10s",
			Timeout:  "5s",
			Retries:  10,
		},
		Restart:  "unless-stopped",
		Networks: []string{"ac-network"},
		Profiles: []string{"core"},
	}

	// ac-worldserver
	services["ac-worldserver"] = Service{
		Image:   runtimeImage,
		Command: []string{"/entrypoint.sh", "worldserver"},
		EnvFile: []string{".env", "env.ac"},
		Ports:   []string{"8085:8085"},
		Volumes: []string{
			"config:/azerothcore/env/dist/etc",
			"extracted-data:/azerothcore/env/dist/data:ro",
		},
		DependsOn: map[string]DependsOnConfig{
			"ac-database":   {Condition: "service_healthy"},
			"ac-db-import":  {Condition: "service_completed_successfully"},
			"ac-authserver": {Condition: "service_healthy"},
		},
		HealthCheck: &HealthCheck{
			Test:     []string{"CMD", "nc", "-z", "localhost", "8085"},
			Interval: "10s",
			Timeout:  "5s",
			Retries:  10,
		},
		Restart:  "unless-stopped",
		Networks: []string{"ac-network"},
		Profiles: []string{"core"},
	}

	// ac-client-data-init - oneshot client data extraction
	services["ac-client-data-init"] = Service{
		Image:   runtimeImage,
		Command: []string{"/entrypoint.sh", "client-data"},
		EnvFile: []string{".env", "env.ac"},
		Volumes: []string{
			"client-data:/wow-client:ro",
			"extracted-data:/azerothcore/env/dist/data",
		},
		DependsOn: map[string]DependsOnConfig{
			"ac-db-import": {Condition: "service_completed_successfully"},
		},
		Networks: []string{"ac-network"},
		Profiles: []string{"core"},
		Restart:  "no",
	}

	// ac-tools - development tools
	services["ac-tools"] = Service{
		Image:   runtimeImage,
		Command: []string{"sleep", "infinity"},
		EnvFile: []string{".env", "env.ac"},
		Volumes: []string{
			"config:/azerothcore/env/dist/etc",
			"extracted-data:/azerothcore/env/dist/data:ro",
		},
		Networks: []string{"ac-network"},
		Profiles: []string{"tools"},
		Restart:  "no",
	}

	// ac-dev-server - dev profile with live reload
	services["ac-dev-server"] = Service{
		Image:   runtimeImage,
		Command: []string{"/entrypoint.sh", "worldserver-dev"},
		EnvFile: []string{".env", "env.ac"},
		Ports:   []string{"8086:8085"},
		Volumes: []string{
			"config:/azerothcore/env/dist/etc",
			"extracted-data:/azerothcore/env/dist/data:ro",
			// Host module sources mounted for live reload
		},
		DependsOn: map[string]DependsOnConfig{
			"ac-database":   {Condition: "service_healthy"},
			"ac-db-import":  {Condition: "service_completed_successfully"},
			"ac-authserver": {Condition: "service_healthy"},
		},
		HealthCheck: &HealthCheck{
			Test:     []string{"CMD", "nc", "-z", "localhost", "8085"},
			Interval: "10s",
			Timeout:  "5s",
			Retries:  10,
		},
		Restart:  "unless-stopped",
		Networks: []string{"ac-network"},
		Profiles: []string{"dev"},
	}

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
			base.DependsOn = make(map[string]DependsOnConfig)
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
	networkName := "ac-network"
	if g.Manifest != nil && g.Manifest.Compose.Network != "" {
		networkName = g.Manifest.Compose.Network
	}
	return map[string]Network{
		networkName: {Driver: "bridge", Name: networkName},
	}
}

// GenerateVolumes generates the volumes configuration.
func (g *ServiceGenerator) GenerateVolumes() map[string]Volume {
	volumes := map[string]Volume{
		"mysql-data":     {Driver: "local"},
		"extracted-data": {Driver: "local"},
		"config":         {Driver: "local"},
		"acore-build":    {Driver: "local"},
		"ccache-volume":  {Driver: "local"},
	}

	// Add host volumes from manifest
	if g.Manifest != nil && g.Manifest.Compose.Volumes != nil {
		for hostPath, containerPath := range g.Manifest.Compose.Volumes {
			volName := fmt.Sprintf("host-volume-%s", hostPath)
			// Host volumes are handled as bind mounts in service volumes, not named volumes
			_ = volName
			_ = containerPath
		}
	}

	// Add client data volume if local
	if g.Manifest != nil && g.Manifest.ClientData.Source == "local" && g.Manifest.ClientData.Path != "" {
		volumes["client-data"] = Volume{Driver: "local"}
	}

	return volumes
}
