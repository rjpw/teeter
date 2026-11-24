package manifest

type Manifest struct {
	Version     string         `yaml:"version" json:"version"`
	Project     Project        `yaml:"project" json:"project"`
	ACore       ACoreSpec      `yaml:"acore" json:"acore"`
	Modules     []ModuleSpec   `yaml:"modules" json:"modules"`
	Database    DatabaseSpec   `yaml:"database" json:"database"`
	ClientData  ClientDataSpec `yaml:"client_data" json:"client_data"`
	Environment EnvSpec        `yaml:"environment" json:"environment"`
	Compose     ComposeSpec    `yaml:"compose" json:"compose"`
}

type Project struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

type ACoreSpec struct {
	Repo    string `yaml:"repo" json:"repo"`
	Branch  string `yaml:"branch" json:"branch"`
	Commit  string `yaml:"commit,omitempty" json:"commit,omitempty"`
	Shallow bool   `yaml:"shallow,omitempty" json:"shallow,omitempty"`
}

type ModuleSpec struct {
	Name      string       `yaml:"name" json:"name"`
	Source    string       `yaml:"source,omitempty" json:"source,omitempty"`
	Version   string       `yaml:"version,omitempty" json:"version,omitempty"`
	Path      string       `yaml:"path,omitempty" json:"path,omitempty"`
	Patches   []PatchSpec  `yaml:"patches,omitempty" json:"patches,omitempty"`
	Config    ModuleConfig `yaml:"config,omitempty" json:"config,omitempty"`
	DependsOn []string     `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
}

type PatchSpec struct {
	Path   string `yaml:"path" json:"path"`
	Target string `yaml:"target,omitempty" json:"target,omitempty"`
}

type ModuleConfig struct {
	Env       map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	CMakeArgs []string          `yaml:"cmake_args,omitempty" json:"cmake_args,omitempty"`
}

type DatabaseSpec struct {
	Migrations []string `yaml:"migrations,omitempty" json:"migrations,omitempty"`
	Seed       []string `yaml:"seed,omitempty" json:"seed,omitempty"`
}

type ClientDataSpec struct {
	Source       string `yaml:"source" json:"source"`
	Path         string `yaml:"path,omitempty" json:"path,omitempty"`
	URL          string `yaml:"url,omitempty" json:"url,omitempty"`
	ExtractMaps  bool   `yaml:"extract_maps,omitempty" json:"extract_maps,omitempty"`
	ExtractVmaps bool   `yaml:"extract_vmaps,omitempty" json:"extract_vmaps,omitempty"`
	ExtractMmaps bool   `yaml:"extract_mmaps,omitempty" json:"extract_mmaps,omitempty"`
}

type EnvSpec struct {
	Vars    map[string]string `yaml:"vars,omitempty" json:"vars,omitempty"`
	Secrets []string          `yaml:"secrets,omitempty" json:"secrets,omitempty"`
}

type ComposeSpec struct {
	Network  string                      `yaml:"network,omitempty" json:"network,omitempty"`
	Volumes  map[string]string           `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	Profiles []string                    `yaml:"profiles,omitempty" json:"profiles,omitempty"`
	Services map[string]ServiceExtension `yaml:"services,omitempty" json:"services,omitempty"`
}

type ServiceExtension struct {
	Image       string            `yaml:"image,omitempty" json:"image,omitempty"`
	Build       *BuildExtension   `yaml:"build,omitempty" json:"build,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty" json:"environment,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	Ports       []string          `yaml:"ports,omitempty" json:"ports,omitempty"`
	DependsOn   []string          `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Profiles    []string          `yaml:"profiles,omitempty" json:"profiles,omitempty"`
}

type BuildExtension struct {
	Context    string            `yaml:"context,omitempty" json:"context,omitempty"`
	Dockerfile string            `yaml:"dockerfile,omitempty" json:"dockerfile,omitempty"`
	Args       map[string]string `yaml:"args,omitempty" json:"args,omitempty"`
}
