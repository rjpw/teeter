package compose

// ComposeConfig represents the docker-compose.yml structure.
type ComposeConfig struct {
	Version  string             `yaml:"version,omitempty" json:"version,omitempty"`
	Services map[string]Service `yaml:"services" json:"services"`
	Networks map[string]Network `yaml:"networks,omitempty" json:"networks,omitempty"`
	Volumes  map[string]Volume  `yaml:"volumes,omitempty" json:"volumes,omitempty"`
}

// Service represents a docker-compose service.
type Service struct {
	Image       string                     `yaml:"image,omitempty" json:"image,omitempty"`
	Build       *BuildConfig               `yaml:"build,omitempty" json:"build,omitempty"`
	Command     []string                   `yaml:"command,omitempty" json:"command,omitempty"`
	Entrypoint  []string                   `yaml:"entrypoint,omitempty" json:"entrypoint,omitempty"`
	Environment []string                   `yaml:"environment,omitempty" json:"environment,omitempty"`
	EnvFile     []string                   `yaml:"env_file,omitempty" json:"env_file,omitempty"`
	Volumes     []string                   `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	Ports       []string                   `yaml:"ports,omitempty" json:"ports,omitempty"`
	DependsOn   map[string]DependsOnConfig `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	HealthCheck *HealthCheck               `yaml:"healthcheck,omitempty" json:"healthcheck,omitempty"`
	Profiles    []string                   `yaml:"profiles,omitempty" json:"profiles,omitempty"`
	Restart     string                     `yaml:"restart,omitempty" json:"restart,omitempty"`
	User        string                     `yaml:"user,omitempty" json:"user,omitempty"`
	WorkingDir  string                     `yaml:"working_dir,omitempty" json:"working_dir,omitempty"`
	Networks    []string                   `yaml:"networks,omitempty" json:"networks,omitempty"`
	Deploy      *DeployConfig              `yaml:"deploy,omitempty" json:"deploy,omitempty"`
}

// BuildConfig represents the build configuration for a service.
type BuildConfig struct {
	Context    string            `yaml:"context,omitempty" json:"context,omitempty"`
	Dockerfile string            `yaml:"dockerfile,omitempty" json:"dockerfile,omitempty"`
	Args       map[string]string `yaml:"args,omitempty" json:"args,omitempty"`
	Target     string            `yaml:"target,omitempty" json:"target,omitempty"`
	CacheFrom  []string          `yaml:"cache_from,omitempty" json:"cache_from,omitempty"`
}

// DeployConfig represents the deploy configuration (for Swarm).
type DeployConfig struct {
	Resources *Resources `yaml:"resources,omitempty" json:"resources,omitempty"`
}

// Resources represents resource limits.
type Resources struct {
	Limits       *ResourceLimit `yaml:"limits,omitempty" json:"limits,omitempty"`
	Reservations *ResourceLimit `yaml:"reservations,omitempty" json:"reservations,omitempty"`
}

// ResourceLimit represents CPU/memory limits.
type ResourceLimit struct {
	CPUs   string `yaml:"cpus,omitempty" json:"cpus,omitempty"`
	Memory string `yaml:"memory,omitempty" json:"memory,omitempty"`
}

// HealthCheck represents a health check configuration.
type HealthCheck struct {
	Test        []string `yaml:"test,omitempty" json:"test,omitempty"`
	Interval    string   `yaml:"interval,omitempty" json:"interval,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Retries     int      `yaml:"retries,omitempty" json:"retries,omitempty"`
	StartPeriod string   `yaml:"start_period,omitempty" json:"start_period,omitempty"`
}

// DependsOnConfig represents dependency configuration.
type DependsOnConfig struct {
	Condition string `yaml:"condition,omitempty" json:"condition,omitempty"`
}

// Network represents a network configuration.
type Network struct {
	Driver string `yaml:"driver,omitempty" json:"driver,omitempty"`
	Name   string `yaml:"name,omitempty" json:"name,omitempty"`
}

// Volume represents a volume configuration.
type Volume struct {
	Driver string `yaml:"driver,omitempty" json:"driver,omitempty"`
	Name   string `yaml:"name,omitempty" json:"name,omitempty"`
}
