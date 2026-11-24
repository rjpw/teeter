# teeter
![](docs/images/teeter.webp)

A [narrow-gauge](https://en.wikipedia.org/wiki/Narrow-gauge_railway) mod manager for AzerothCore. Some assembly required.

> Up and awa ... oh!

## Motivation

If you've ever modded Skyrim (for me it was "Oldrim") you'll know there are excellent tools to customize your virtual world. Such is not the case for a lesser-known open world solo RPG, namely `AzerothCore-WotLK`, played solo with `mod-playerbots`.

I know. A table for one, and a `CLOSED` sign on the door of this massively multiplayer online role-playing game? When I play with other people, I prefer the official [World of Warcraft](https://worldofwarcraft.blizzard.com/en-us/) experience. When I want to sharpen my DevOps skills, I build the Azeroth equivalent of a train set in the basement. When that falls down, no world firsts are delayed, and nobody's progress comes to a halt ...

... and I get to be a GM (when I lose patience).

## Assumptions

The `AzerothCore` team publishes a [catalogue](https://www.azerothcore.org/catalogue#/) of mods, tools, and scripts. The `teeter` CLI tool is meant to help smoosh those together into a teetering, tottering, but hopefully cooperating assemblage of open-source shinies.

I'm a DevOps guy, and I'm going to assume you're like me and you don't feel like cluttering up your systems with a dozen different database engines, build tools, templating kits, and assorted gnomish foot cannons.

For that reason, you're going to get `teeter` to help you compose a compact and repeatable Docker Compose file, you're going to feed that composition the most lovingly crafted manifest, and you're going to treat the resulting edifice like the fragile [Jenga](https://en.wikipedia.org/wiki/Jenga) tower that it is.

The good news is, `teeter` will hopefully help you stand the tower back up when your cat knocks it over.

## Installation

```sh
go install github.com/rjpw/teeter@latest
```

Verify installation:
```sh
teeter --help
```

## Quick Start (What Works Now)

### 1. Initialize a Project

```sh
mkdir my-server && cd my-server
teeter init
```

This creates `teeter.yaml` with sensible defaults.

### 2. Edit Your Manifest

```yaml
# teeter.yaml
version: "1"
project:
  name: "my-solo-server"
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
client_data:
  source: "local"
  path: "~/wow-client"  # Path to your WoW 3.3.5a client
  extract_maps: true
  extract_vmaps: true
  extract_mmaps: true
environment:
  vars: {}
compose: {}
```

### 3. Fetch the Static Catalogue (No Auth Required)

```sh
teeter catalogue refresh
# Output: 292 modules fetched from azerothcore.github.io
```

> **No GitHub token needed.** Uses the static catalogue from `azerothcore.github.io`.
> Use `teeter catalogue refresh --api` (requires `GITHUB_TOKEN`) for live GitHub data.

### 4. Resolve and Cache Dependencies

```sh
teeter pull --dry-run   # Show resolution plan
teeter pull             # Fetch AC source + modules to cache (~/.teeter/cache/)
```

### 5. Assemble Build Context

```sh
teeter buildctx
# Links AC source + modules from cache, applies patches, assembles SQL
# Output: .teeter/buildctx/<hash>/
```

### 6. Generate Docker Compose

```sh
teeter compose --dry-run   # Preview
teeter compose             # Write .teeter/buildctx/<hash>/docker-compose.yml
```

Uses **layered build strategy** by default (`prebuilt` = no local compilation):
- `teeter/acore-builder:<ac-commit>` — AC build environment
- `teeter/acore-modules:<modules-hash>` — Compiled modules
- `teeter/acore-runtime:<full-hash>` — Runtime image (config + SQL)

### 7. Check System Readiness

```sh
teeter doctor
# Checks: RAM, swap, CPU, disk, Docker daemon, buildx
```

### 8. Start the Server (Phase 5 - Not Yet Implemented)

```sh
# Coming in Phase 5:
teeter up          # Start all services (db → db-import → client-data → auth → world)
teeter logs -f ac-worldserver
teeter exec ac-worldserver bash
teeter down --volumes
```

## Current Capabilities (Phases 0-4 Complete)

| Command | Status |
|---------|--------|
| `teeter init` | ✅ Creates manifest |
| `teeter validate` | ✅ Schema + dependency validation |
| `teeter list` | ✅ Lists catalogue modules |
| `teeter catalogue refresh` | ✅ Static (default) / `--api` for GitHub |
| `teeter pull` | ✅ Resolves + caches AC + modules |
| `teeter buildctx` | ✅ Links sources, applies patches, assembles SQL |
| `teeter compose` | ✅ Generates layered docker-compose.yml |
| `teeter build` | ✅ Builds specific Docker layers |
| `teeter doctor` | ✅ System resource validation |
| `teeter up/down/logs/exec` | 🚧 Phase 5 |

## Manifest Reference (Minimal)

```yaml
version: "1"
project:
  name: "my-server"
acore:
  repo: "https://github.com/mod-playerbots/azerothcore-wotlk"
  branch: "Playerbot"
  # commit: "abc123"  # Optional: pin to specific commit
modules:
  - name: "mod-playerbots"      # From catalogue
    source: "catalogue"
    version: "master"
  - name: "my-custom-module"    # From Git
    source: "git"
    version: "main"
    path: "https://github.com/me/my-module"
  - name: "local-module"        # Local path
    source: "local"
    path: "./my-module"
client_data:
  source: "local"
  path: "~/wow-client"
  extract_maps: true
  extract_vmaps: true
  extract_mmaps: true
environment:
  vars:
    CUSTOM_VAR: "value"
  secrets:
    - "MYSQL_ROOT_PASSWORD"
    - "DISCORD_WEBHOOK_URL"
compose:
  build_strategy: "prebuilt"  # prebuilt | auto | local
  build_memory: "4G"
  build_cpus: "2"
  build_jobs: 2
  ccache_maxsize: "1G"
```

## Layered Build Strategy

```
teeter/acore-runtime:<full-hash>    ← Config/SQL changes (fast, copy-only)
teeter/acore-modules:<modules-hash>  ← Module changes (ccache incremental)
teeter/acore-builder:<ac-commit>     ← AC version bump (rare)
ubuntu:22.04                          ← Base (never rebuilds)
```

**Default: `prebuilt`** — no local compilation. Images pulled from registry.
Use `--build-strategy local` (or `build_strategy: local` in manifest) to build locally.

## Configuration

Global config: `~/.teeter/config.yaml`
```yaml
cache_dir: "~/.teeter/cache"
catalogue_url: "https://raw.githubusercontent.com/azerothcore/azerothcore.github.io/master/data/catalogue.json"
link_strategy: "auto"  # hardlink | symlink | auto | copy
```

## Documentation

* [Design & Architecture](docs/design.md)
* [Manifest Specification](docs/manifest.md) *(coming soon)*
* [Caveat Emptor](docs/caveats.md)

## License

This project is licensed under the [MIT License](LICENSE.md).