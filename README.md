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
go install github.com/rjpw/teeter/cmd/teeter@latest
```

If the Go module proxy hasn't indexed the latest version yet (404 on `sum.golang.org`):
```sh
GOSUMDB=off go install github.com/rjpw/teeter/cmd/teeter@latest
```

Verify installation (ensure `~/go/bin` is in your PATH):
```sh
export PATH=$PATH:~/go/bin
teeter --help
```

## Quick Start

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
  source: "static"
  version: "v20.0"

> **Client data**: Uses pre-extracted data from `wowgaming/client-data` releases (no WoW client needed).
### 3. Fetch the Static Catalogue (No Auth Required)

```sh
teeter catalogue refresh
# Output: 292 modules fetched from azerothcore.github.io
```

> **No GitHub token needed.** Uses the static catalogue from `azerothcore.github.io`.
> Use `teeter catalogue refresh --api` (requires `GITHUB_TOKEN`) for live GitHub data.

### 4. Download Client Data (Pre-extracted Maps/VMaps/MMaps)

```sh
teeter client-data pull
# Downloads Data.zip from wowgaming/client-data v20.0, extracts to Docker volume
```

### 5. Resolve and Cache Dependencies

```sh
teeter pull --dry-run   # Show resolution plan
teeter pull             # Fetch AC source + modules to cache (~/.teeter/cache/)
```

### 6. Assemble Build Context

```sh
teeter buildctx
# Links AC source + modules from cache, applies patches, assembles SQL
# Output: .teeter/buildctx/<hash>/
```

### 7. Generate Docker Compose

```sh
teeter compose --dry-run   # Preview
teeter compose             # Write .teeter/buildctx/<hash>/docker-compose.yml
```

Uses **per-service images** (default `prebuilt` = no local compilation):
- `ac-database`: `mysql:8.4`
- `ac-db-import`: `acore/ac-wotlk-db-import:master`
- `ac-authserver`: `acore/ac-wotlk-authserver:master`
- `ac-worldserver`: `teeter/acore-worldserver:<hash>` (custom-built from build context)
- `ac-client-data-init`: `acore/ac-wotlk-client-data:master`
- `ac-tools`: `acore/ac-wotlk-tools:master`

### 8. Check System Readiness

```sh
teeter doctor
# Checks: RAM, swap, CPU, disk, Docker daemon, buildx
```

### 9. Start the Server

```sh
teeter up          # Full orchestration: lock → pull → buildctx → compose → up → health wait
teeter status      # Formatted health table
# Use docker compose logs directly for log streaming
teeter down --volumes
```

## Safe Migration Workflow

When you want to add modules or change configuration:

```sh
# 1. Build new worldserver image
teeter build

# 2. Validate with ephemeral DB (safe dry-run, zero risk to your data)
teeter validate buildctx --buildctx <newhash> --ephemeral
# → "Would apply 47 new updates from mod-auctionhouse. Estimated 30s. No errors."

# 3. Backup persistent DB
teeter backup --output backup.sql.gz

# 4. Migrate persistent DB (apply new SQL, idempotent)
teeter migrate --buildctx <newhash>
# → "Applied 47 new updates... in 30s"

# 5. Switch runtime to new buildctx (atomic)
teeter switch --buildctx <newhash>

# 6. Start stack
teeter up

# Rollback if needed:
teeter switch --rollback
```

## Current Capabilities

| Command | Status |
|---------|--------|
| `teeter init` | ✅ Creates manifest |
| `teeter validate` | ✅ Schema + dependency validation |
| `teeter catalogue refresh` | ✅ Static (default) / `--api` for GitHub |
| `teeter pull` | ✅ Resolves + caches AC + modules |
| `teeter buildctx` | ✅ Links sources, applies patches, assembles SQL |
| `teeter compose` | ✅ Generates docker-compose.yml (per-service images) |
| `teeter build` | ✅ Builds worldserver image |
| `teeter doctor` | ✅ System resource validation |
| `teeter up` | ✅ Full orchestration + health wait |
| `teeter down` | ✅ Graceful shutdown + lock release |
| `teeter status` | ✅ Formatted health table |
| `teeter client-data pull` | ✅ Download pre-extracted client data |
| `teeter switch` | ✅ Atomic runtime cutover / rollback |
| `teeter validate buildctx --ephemeral` | ✅ Dry-run validation |
| `teeter migrate` | ✅ Apply SQL to persistent DB |
| `teeter backup` / `teeter restore` | ✅ Disaster recovery |

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
  source: "static"
  version: "v20.0"
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

## Per-Service Docker Images

| Service | Image | Source |
|---------|-------|--------|
| `ac-database` | `mysql:8.4` | External |
| `ac-db-import` | `acore/ac-wotlk-db-import:master` | Upstream AC |
| `ac-authserver` | `acore/ac-wotlk-authserver:master` | Upstream AC |
| `ac-worldserver` | `teeter/acore-worldserver:<hash>` | **Custom (build context)** |
| `ac-client-data-init` | `acore/ac-wotlk-client-data:master` | Upstream AC |
| `ac-tools` | `acore/ac-wotlk-tools:master` | Upstream AC |

**Only worldserver is custom-built** — content-addressable hash from AC commit + modules + patches.
All other services use stable upstream AC images.

**Build strategies** (for worldserver only):
- `prebuilt` (default) — pull from registry, no local build
- `auto` — build only dirty layers (cache-aware)
- `local` — force rebuild locally

## Configuration

Global config: `~/.teeter/config.yaml`

```yaml
cache_dir: "~/.teeter/cache"
catalogue_url: "https://raw.githubusercontent.com/azerothcore/azerothcore.github.io/master/data/catalogue.json"
link_strategy: "auto"  # hardlink | symlink | auto | copy
```

## Documentation

- [Design & Architecture](docs/design.md)
- [Manifest Specification](docs/manifest.md)
- [Caveats](docs/caveats.md)

## License

This project is licensed under the [MIT License](LICENSE.md).