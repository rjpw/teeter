# Manifest Specification

## Overview

The manifest file (`teeter.yaml`) is the single source of truth for a teeter project. It defines the AC source, modules, client data, environment, and compose settings.

## Structure

```yaml
version: "1"                    # Required: schema version (must be "1")
project:                        # Required: project identity
acore:                          # Required: AC source specification
modules:                        # Optional: module list
client_data:                    # Optional: client data source
environment:                    # Optional: environment variables
compose:                        # Optional: compose overrides
```

## Fields

### `version` (string, required)

Schema version. Must be `"1"`.

### `project` (object, required)

```yaml
project:
  name: "my-server"             # Required: alphanumeric, hyphens, underscores
  description: "My solo server" # Optional
```

### `acore` (object, required)

```yaml
acore:
  repo: "https://github.com/mod-playerbots/azerothcore-wotlk"  # Required: Git URL
  branch: "Playerbot"             # Required unless commit specified
  commit: "abc123def"             # Optional: pin to specific commit
  shallow: false                  # Optional: shallow clone (faster, no history)
```

**Validation**: Either `branch` or `commit` must be specified.

### `modules` (array, optional)

Each module entry:

```yaml
modules:
  - name: "mod-playerbots"        # Required: unique name
    source: "catalogue"           # Required: catalogue | git | local
    version: "master"             # Required for catalogue/git
    path: ""                      # Required for git/local: repo URL or local path
    depends_on: []                # Optional: module names this depends on
    patches: []                   # Optional: list of patch files
    config:                       # Optional: module-specific config
      env: {}                     # Environment variables
      cmake_args: []              # CMake arguments
```

**Source types:**
- `catalogue` — From static catalogue (292+ modules)
- `git` — From Git repository
- `local` — From local filesystem path

### `client_data` (object, optional)

```yaml
client_data:
  source: "static"                # Only "static" supported
  version: "v20.0"                # Required: wowgaming/client-data tag
  url: ""                         # Optional: custom download URL
```

Downloads pre-extracted `Data.zip` from `wowgaming/client-data` releases (~1.1 GB).
No extraction needed, just unzip to `extracted-data` volume.
### `environment` (object, optional)

```yaml
environment:
  vars:                           # Injected into env.ac at runtime
    CUSTOM_VAR: "value"
  secrets:                        # Written to .env as empty placeholders
    - "MYSQL_ROOT_PASSWORD"
    - "DISCORD_WEBHOOK_URL"
```

### `compose` (object, optional)

```yaml
compose:
  network: "ac-network"           # Custom network name
  volumes:                        # Custom volume names (default: teeter-<project>-<type>)
    mysql-data: "custom-mysql"
    extracted-data: "custom-data"
  profiles: ["core"]              # Compose profiles to enable
  services:                       # Service extensions/overrides
    ac-authserver:
      image: "custom/authserver:tag"
      environment:
        EXTRA_VAR: "value"
  build_strategy: "prebuilt"      # prebuilt | auto | local
  build_memory: "4G"              # Build container memory limit
  build_cpus: "2"                 # Build container CPU limit
  build_jobs: 2                   # Parallel build jobs
  ccache_maxsize: "1G"            # ccache max size
  registry: ""                    # Custom Docker registry for worldserver image
```

## Complete Example

```yaml
version: "1"
project:
  name: "my-solo-server"
  description: "Solo server with playerbots and dungeonmaster"
acore:
  repo: "https://github.com/mod-playerbots/azerothcore-wotlk"
  branch: "Playerbot"
modules:
  - name: "mod-playerbots"
    source: "catalogue"
    version: "master"
  - name: "mod-dungeonmaster"
    source: "catalogue"
    version: "master"
  - name: "my-custom-module"
    source: "git"
    version: "main"
    path: "https://github.com/me/my-module"
    depends_on: ["mod-playerbots"]
    patches:
      - "patches/my-fix.patch"
client_data:
  source: "static"
  version: "v20.0"
environment:
  vars:
    LOG_LEVEL: "info"
  secrets:
    - "DISCORD_WEBHOOK_URL"
compose:
  build_strategy: "prebuilt"
  build_memory: "4G"
  build_cpus: "2"
  build_jobs: 2
```

## Validation Rules

Run `teeter validate` to check your manifest:

1. `version` must be `"1"`
2. `project.name` required, regex `^[a-zA-Z0-9][a-zA-Z0-9_-]*$`
3. `acore.repo` must be valid Git URL (http/https/git@)
4. `acore.branch` or `acore.commit` required
5. Module names must be unique
6. `depends_on` modules must exist in list
7. No circular dependencies
8. Local module paths must exist
9. Git URLs must be valid
10. `client_data.source` must be `static`
11. `client_data.version` required for `static` source