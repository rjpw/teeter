# Design & Architecture

## Overview

`teeter` is a CLI tool that assembles and manages AzerothCore-WotLK mod setups from a manifest. It orchestrates:

1. **Fetching** AC source and modules from a catalogue
2. **Building** a content-addressable build context (AC + modules + patches + SQL)
3. **Generating** a Docker Compose stack with per-service images
4. **Operating** the server lifecycle (up/down/logs/exec/status)

The core insight: **each service gets its own Docker image built from a specific Dockerfile target**, not a shared runtime image.

---

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              TEETER CLI                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  teeter.yaml (manifest)                                                     │
│       │                                                                     │
│       ▼                                                                     │
│  ┌─────────────┐    ┌──────────────┐    ┌────────────────┐                  │
│  │  CATALOGUE  │───▶│   CACHE      │───▶│  BUILD CTX     │                  │
│  │  (index)    │    │ (~/.teeter/) │    │ (.teeter/      │                  │
│  └─────────────┘    └──────────────┘    │  buildctx/<h>) │                  │
│       ▲                    ▲            └───────┬────────┘                  │
│       │                    │                    │                           │
│       ▼                    ▼                    ▼                           │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                      DOCKER COMPOSE BUILD                            │   │
│  │  ┌────────────┐ ┌────────────┐ ┌─────────────┐ ┌────────────┐        │   │
│  │  │  build     │ │  authserver│ │  worldserver│ │  db-import │  ...   │   │
│  │  │  target    │ │  target    │ │  target     │ │  target    │        │   │
│  │  └────────────┘ └────────────┘ └─────────────┘ └────────────┘        │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                      DOCKER COMPOSE UP (Runtime)                      │  │
│  │  ┌─────────┐ ┌──────────┐ ┌─────────┐ ┌─────────┐ ┌─────────────────┐ │  │
│  │  │ac-db    │ │ac-db-imp │ │ac-auth  │ │ac-world │ │ac-client-data   │ │  │
│  │  │(mysql)  │ │(dbimport)│ │(auth)   │ │(world)  │ │(extractor)      │ │  │
│  │  └─────────┘ └──────────┘ └─────────┘ └─────────┘ └─────────────────┘ │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Core Concepts

### 1. Manifest (`teeter.yaml`)

Single source of truth defining:
- AC repository + commit/branch
- Module list (name, version, source, dependencies, patches)
- Database customizations (migrations, seeds)
- Client maps data source (static download)
- Environment variables, compose overrides, build strategy

### 2. Catalogue Index

Static JSON index (default: `azerothcore.github.io/data/catalogue.json`) containing:
- Module metadata (repo URL, default branch, releases)
- Per-module dependencies, conflicts, AC compatibility
- 292+ modules, no auth required, no rate limits

### 3. Cache (`~/.teeter/cache/`)

```
~/.teeter/cache/
├── modules/
│   └── <name>/<version>/          # Bare clone + worktree
│       ├── .git/                  # Bare repo
│       └── <version>/             # Worktree at specific commit
├── acore/
│   └── <commit>/                  # Bare clone + worktree
│       ├── .git/
│       └── worktree/
├── index.json                     # Cached catalogue index
├── index.etag                     # ETag for conditional fetch
└── locks/                         # Flock files for concurrent access
```

### 4. Build Context (`.teeter/buildctx/<hash>/`)

Content-addressable, assembled from:
- AC source (checked out at commit)
- Module sources (linked via hardlink/symlink/copy)
- Patches applied
- SQL files combined (migrations + seeds + module SQL)
- Config templates

**Hash** = SHA256(AC commit + module commits + patches + manifest) → determines image tags.

---

## Docker Image Strategy

### Core Principle: Decoupled Runtime

**Only the worldserver image is custom-built.** All other services use upstream AC images directly.

The `teeter` command acts like an **anti-corruption layer**: given community mods with different conventions, it structures build inputs for worldserver, and provides configuration/data at runtime via volumes/env. The other AC images (authserver, db-import, client-data, tools) are stable upstream artifacts — we don't rebuild them.

### Upstream AC Images (Used As-Is)

| Service | Upstream Image | What teeter Provides at Runtime |
|---------|----------------|--------------------------------|
| `ac-database` | `mysql:8.4` | DB passwords via `.env` |
| `ac-authserver` | `acore/ac-wotlk-authserver:<tag>` | Config via `env.ac` + volumes |
| `ac-db-import` | `acore/ac-wotlk-db-import:<tag>` | Custom SQL via volume, config via `env.ac` |
| `ac-client-data-init` | `acore/ac-wotlk-client-data:<tag>` | WoW client path, download config |
| `ac-tools` | `acore/ac-wotlk-tools:<tag>` | Volumes for extracted data |

**Tag**: Use AC's `DOCKER_IMAGE_TAG` (e.g., `master`, `latest`, or specific version).

### Custom Image: Worldserver (Primary Artifact)

| Service | Image | Dockerfile Target | Built From |
|---------|-------|-------------------|------------|
| `ac-worldserver` | `teeter/acore-worldserver:<hash-8>` | `worldserver` | `.teeter/buildctx/<hash>/` |

**Build context includes**:
- AC source (patched at commit)
- Module sources (linked)
- Custom SQL (for db-import volume, not baked into image)
- Config templates (for runtime volumes, not baked)

**Hash** = SHA256(AC commit + module commits + patches + manifest) → content-addressable tag.

### Why This Decoupling Matters

1. **Build brittleness isolated** — Only worldserver compilation can fail; other services just work
2. **Mod SQL/config at runtime** — Module SQL goes to db-import volume; module config goes to config volume. No image rebuild needed for config changes.
3. **Upstream AC updates** — Pull new `acore/ac-wotlk-authserver:master` without rebuilding worldserver
4. **Cache efficiency** — Worldserver image only rebuilds when AC/modules/patches change
5. **Admin flexibility** — Modify `env.ac`, `.env`, or volumes without `teeter build`

---

## Compose Services

### Service Definitions

```yaml
services:
  ac-database:
    image: mysql:8.4
    healthcheck: mysqladmin ping
    volumes: [mysql-data:/var/lib/mysql]
    profiles: [core]

  ac-db-import:
    image: acore/ac-wotlk-db-import:${DOCKER_IMAGE_TAG:-master}
    command: ["/entrypoint.sh", "dbimport"]
    depends_on:
      ac-database: {condition: service_healthy}
    volumes:
      - mysql-data:/var/lib/mysql
      - extracted-data:/azerothcore/env/dist/data
      - config:/azerothcore/env/dist/etc
      - ./combined.sql:/azerothcore/data/custom.sql:ro
    env_file: [.env, env.ac]
    profiles: [core]
    restart: "no"

  ac-authserver:
    image: acore/ac-wotlk-authserver:${DOCKER_IMAGE_TAG:-master}
    command: ["/entrypoint.sh", "authserver"]
    depends_on:
      ac-database: {condition: service_healthy}
      ac-db-import: {condition: service_completed_successfully}
    healthcheck:
      test: ["CMD", "nc", "-z", "localhost", "3724"]
      interval: "10s"
      retries: 10
    volumes:
      - config:/azerothcore/env/dist/etc
      - extracted-data:/azerothcore/env/dist/data:ro
    env_file: [.env, env.ac]
    profiles: [core]

  ac-worldserver:
    image: teeter/acore-worldserver:<hash>
    command: ["/entrypoint.sh", "worldserver"]
    depends_on:
      ac-database: {condition: service_healthy}
      ac-db-import: {condition: service_completed_successfully}
      ac-authserver: {condition: service_healthy}
      ac-client-data-init: {condition: service_completed_successfully}
    healthcheck:
      test: ["CMD", "nc", "-z", "localhost", "8085"]
      interval: "10s"
      retries: 10
    volumes:
      - config:/azerothcore/env/dist/etc
      - extracted-data:/azerothcore/env/dist/data:ro
    env_file: [.env, env.ac]
    profiles: [core]

  ac-client-data-init:
    image: acore/ac-wotlk-client-data:${DOCKER_IMAGE_TAG:-master}
    command: ["/entrypoint.sh", "client-data"]
    depends_on:
      ac-db-import: {condition: service_completed_successfully}
    volumes:
      - client-data:/wow-client:ro
      - extracted-data:/azerothcore/env/dist/data
    env_file: [.env, env.ac]
    profiles: [core]
    restart: "no"

  ac-tools:
    image: acore/ac-wotlk-tools:${DOCKER_IMAGE_TAG:-master}
    command: ["sleep", "infinity"]
    volumes:
      - config:/azerothcore/env/dist/etc
      - extracted-data:/azerothcore/env/dist/data:ro
    env_file: [.env, env.ac]
    profiles: [tools]

  # ac-dev-server: NOT included (Phase 9 uses bind mounts + Dockerfile.dev-server)
```

### Startup Sequence

```
1. ac-database          (mysql:8.4)              ──▶ healthy
       │
       ├── 2. ac-db-import        (dbimport)     ──▶ completed
       │       │
       │       ├── 3. ac-client-data-init (client-data) ──▶ completed
       │       │
       │       └── 4. ac-authserver      (authserver)     ──▶ healthy
       │
       └── 5. ac-worldserver      (worldserver)    ──▶ healthy
               (waits for 2, 3, 4)
```

---

## Entrypoint (Upstream AC)

We use AC's existing `apps/docker/entrypoint.sh`:

```bash
#!/usr/bin/env bash
# 1. Copy default configs from /env/ref/etc → /env/dist/etc (no clobber)
# 2. Set up component-specific config ($ACORE_COMPONENT.conf)
# 3. exec "$@"  # Runs the CMD: authserver, worldserver, dbimport, etc.
```

Each service's `CMD` in the Dockerfile target is the binary name. The entrypoint just prepares config and executes it.

---

## Build Strategies

| Strategy | Behavior | Use Case |
|----------|----------|----------|
| `prebuilt` (default) | Pull images from registry, no local build | CI, limited resources |
| `auto` | Build only dirty layers (cache-aware) | Development |
| `local` | Force rebuild all targets locally | Full control |

**Layer caching**: Docker build cache + BuildKit cache mounts (`--mount=type=cache,target=/ccache`) preserve ccache across builds.

---

## Resource Requirements (Validated by `teeter doctor`)

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| RAM | 4 GB | 8+ GB |
| Swap | 2 GB | 4+ GB |
| CPU | 2 cores | 4+ cores |
| Disk | 10 GB free | 50+ GB |
| Docker | Daemon running | BuildKit enabled |

---

## Client Data Pipeline

### Source: `local` (default)
- User provides WoW client path (`wow_client_maps_path`)
- `ac-client-data-init` runs extractors (map_extractor, vmap4_extractor, mmaps_generator)
- Output to `extracted-data` volume

### Source: `static` (optional)
- Downloads pre-extracted `Data.zip` from `wowgaming/client-data` releases (1.11 GB)
- No extraction needed, just unzip to `extracted-data`
- `version` field specifies release tag (e.g., `v20.0`)

---

## Project Locking

`.teeter/lock` file prevents concurrent `teeter up`:
- Contains PID + timestamp
- Stale if PID dead OR >1 hour old
- Auto-cleaned on `teeter down`

---

## Configuration Files

| File | Generated By | Purpose |
|------|--------------|---------|
| `docker-compose.yml` | `teeter compose` | Full stack definition |
| `.env` | `teeter compose` | Secrets + overrides (DB passwords) |
| `env.ac` | `teeter compose` | AC-specific env vars (from manifest + modules) |
| `combined.sql` | `teeter buildctx` / `compose` | All SQL concatenated for db-import |

---

## Commands Reference

| Command | Phase | Description |
|---------|-------|-------------|
| `teeter init` | 0 | Create manifest template |
| `teeter validate` | 1 | Validate manifest |
| `teeter catalogue refresh` | 2 | Fetch/update catalogue index |
| `teeter pull` | 2 | Resolve + cache AC + modules |
| `teeter buildctx` | 3 | Assemble build context |
| `teeter compose` | 4 | Generate docker-compose.yml |
| `teeter build` | 4 | Explicit image builds (`--target`, `--all`) |
| `teeter doctor` | 4 | Validate system resources |
| `teeter up` | 5 | Full startup (pull→buildctx→compose→up→wait) |
| `teeter down` | 5 | Shutdown (`--volumes`, `--rmi`) |
| `teeter logs` | 5 | Stream service logs |
| `teeter exec` | 5 | Execute in container |
| `teeter shell` | 5 | Interactive shell |
| `teeter status` | 5 | Show service health |
| `teeter client-data` | 6 | Manage client data |
| `teeter cache` | 7 | Cache management |
| `teeter list` | 7 | List available modules |
| `teeter backup` | 8 | Backup DB + volumes |
| `teeter restore` | 8 | Restore from backup |
| `teeter maint` | 8 | Maintenance tasks |

---

## Non-Goals

- **No `worldserver-dev` service** — Dev mode (Phase 9) uses separate `Dockerfile.dev-server` with host bind mounts
- **No single shared runtime image** — Each service target produces its own minimal image
- **No custom entrypoint** — Use upstream AC entrypoint
- **No database schema management** — Handled by AC's `dbimport` tool
- **No WoW client extraction logic** — Handled by AC's `client-data` target

---

## Key Design Decisions

1. **Per-service images** — Matches upstream AC Dockerfile targets; worldserver is the custom artifact
2. **Content-addressable build context** — Hash determines image tags; only dirty layers rebuild
3. **Static catalogue default** — No GitHub token needed; 292 modules, no rate limits
4. **Manifest-driven client data** — `wow_client_maps_path` only required when extraction enabled
5. **Layered build strategy** — `prebuilt` default safe for constrained machines
6. **Doctor before build** — Fail fast on resource constraints
7. **Project locking** — Prevents corrupted state from concurrent `up`