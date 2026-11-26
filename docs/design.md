# Design & Architecture

## Overview

`teeter` is a CLI tool that assembles and manages AzerothCore-WotLK mod setups from a manifest. It orchestrates:

1. **Fetching** AC source and modules from a catalogue
2. **Building** a content-addressable build context (AC + modules + patches + SQL)
3. **Generating** a Docker Compose stack with per-service images
4. **Operating** the server lifecycle (up/down/status) with safe migration

The core insight: **each service gets its own Docker image built from a specific Dockerfile target**, not a shared runtime image. **Build contexts are content-addressable and immutable; runtime data volumes are stable and persistent.**

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
│  │                      BUILD CONTEXT (Validation Harness)              │   │
│  │  ┌────────────┐ ┌────────────┐ ┌─────────────┐ ┌────────────┐        │   │
│  │  │  build     │ │  authserver│ │  worldserver│ │  db-import │  ...   │   │
│  │  │  target    │ │  target    │ │  target     │ │  target    │        │   │
│  │  └────────────┘ └────────────┘ └─────────────┘ └────────────┘        │   │
│  │  ┌────────────────────────────────────────────────────────────────┐   │   │
│  │  │ EPHEMERAL VALIDATION: spin up dbimport + server on tmp volume │   │   │
│  │  └────────────────────────────────────────────────────────────────┘   │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│               ┌────────────────────┴────────────────────┐                  │
│               ▼                                         ▼                  │
│  ┌─────────────────────────────────┐  ┌─────────────────────────────────┐  │
│  │      BUILD TIME (immutable)     │  │      RUNTIME (thin + stable)    │  │
│  ├─────────────────────────────────┤  ├─────────────────────────────────┤  │
│  │ .teeter/buildctx/<hash>/        │  │ .teeter/runtime/                │  │
│  │   ├── docker-compose.yml        │──▶│   ├── docker-compose.yml  ──▶ │  │
│  │   ├── sql/                      │   │   ├── sql/               ──▶  │  │
│  │   ├── env.ac / .env             │   │   ├── env.ac             ──▶  │  │
│  │   ├── azerothcore/ (linked)     │   │   ├── azerothcore/       ──▶  │  │
│  │   └── .build-hash               │   │   ├── .current-hash            │  │
│  └─────────────────────────────────┘   │   └── volumes/ (STABLE)        │  │
│                                        │       ├── mysql-data/          │  │
│                                        │       └── extracted-data/      │  │
│                                        └─────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Core Concepts

### 1. Manifest (`teeter.yaml`)

Single source of truth defining:
- Project identity: `project.name` (stable, used for volume naming)
- AC repository + commit/branch
- Module list (name, version, source, dependencies, patches)
- Database customizations (migrations, seeds)
- Client maps data source (static download or local extraction)
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

**Content-addressable, immutable, assembled from:**
- AC source (checked out at commit)
- Module sources (linked via hardlink/symlink/copy)
- Patches applied
- SQL files combined (migrations + seeds + module SQL)
- Config templates (dbimport.conf, authserver.conf, worldserver.conf)

**Hash** = SHA256(AC commit + module commits + patches + manifest modules/config) → determines image tags.

**Purpose**: Immutable validation harness. Each build context can be validated independently with ephemeral data.

### 5. Runtime (`.teeter/runtime/`)

**Thin symlink farm + stable volumes:**

```
.teeter/runtime/
├── docker-compose.yml     → ../buildctx/<currenthash>/docker-compose.yml
├── sql/                   → ../buildctx/<currenthash>/sql/
├── env.ac                 → ../buildctx/<currenthash>/env.ac
├── .env                   → ../buildctx/<currenthash>/.env (copied, not linked)
├── azerothcore/           → ../buildctx/<currenthash>/azerothcore/
├── .current-hash          # Records active build context hash
└── volumes/               # STABLE, project-named
    ├── mysql-data/        # Persistent MySQL (teeter-<project>-mysql)
    └── extracted-data/    # Persistent client data (teeter-<project>-extracted-data)
```

**Properties:**
- Zero duplication — everything links to build context
- Atomic switch — `ln -sfn ../buildctx/<newhash> .teeter/runtime/buildctx`
- Instant rollback — `ln -sfn ../buildctx/<oldhash> .teeter/runtime/buildctx`
- Single source of truth — build context immutable; runtime is thin pointers

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

**Build context includes:**
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
    healthcheck: mysqladmin ping -u root -p$${MYSQL_ROOT_PASSWORD}
    volumes:
      - teeter-${PROJECT_NAME}-mysql:/var/lib/mysql
    profiles: [core]

  ac-db-import:
    image: acore/ac-wotlk-db-import:${DOCKER_IMAGE_TAG:-master}
    command: ["dbimport"]
    depends_on:
      ac-database: {condition: service_healthy}
    volumes:
      - teeter-${PROJECT_NAME}-mysql:/var/lib/mysql
      - teeter-${PROJECT_NAME}-extracted-data:/azerothcore/env/dist/data
      - ./sql/teeter-dbimport.conf:/azerothcore/env/dist/etc/dbimport.conf:ro
      - ./sql/teeter-combined.sql:/azerothcore/data/custom.sql:ro
    env_file: [.env, env.ac]
    profiles: [core]
    restart: "no"

  ac-authserver:
    image: acore/ac-wotlk-authserver:${DOCKER_IMAGE_TAG:-master}
    command: ["authserver"]
    depends_on:
      ac-database: {condition: service_healthy}
      ac-db-import: {condition: service_completed_successfully}
    volumes:
      - teeter-${PROJECT_NAME}-config:/azerothcore/env/dist/etc
      - teeter-${PROJECT_NAME}-extracted-data:/azerothcore/env/dist/data:ro
    env_file: [.env, env.ac]
    profiles: [core]

  ac-worldserver:
    image: teeter/acore-worldserver:<hash>
    command: ["worldserver"]
    depends_on:
      ac-database: {condition: service_healthy}
      ac-db-import: {condition: service_completed_successfully}
      ac-authserver: {condition: service_healthy}
      ac-client-data-init: {condition: service_completed_successfully}
    volumes:
      - teeter-${PROJECT_NAME}-config:/azerothcore/env/dist/etc
      - teeter-${PROJECT_NAME}-extracted-data:/azerothcore/env/dist/data:ro
    env_file: [.env, env.ac]
    profiles: [core]

  ac-client-data-init:
    image: acore/ac-wotlk-client-data:${DOCKER_IMAGE_TAG:-master}
    command: ["bash", "-c", "source /azerothcore/apps/installer/includes/functions.sh && inst_download_client_data"]
    depends_on:
      ac-db-import: {condition: service_completed_successfully}
    volumes:
      - client-data:/wow-client:ro
      - teeter-${PROJECT_NAME}-extracted-data:/azerothcore/env/dist/data
    env_file: [.env, env.ac]
    profiles: [core]
    restart: "no"

  ac-tools:
    image: acore/ac-wotlk-tools:${DOCKER_IMAGE_TAG:-master}
    command: ["sleep", "infinity"]
    volumes:
      - teeter-${PROJECT_NAME}-config:/azerothcore/env/dist/etc
      - teeter-${PROJECT_NAME}-extracted-data:/azerothcore/env/dist/data:ro
    env_file: [.env, env.ac]
    profiles: [tools]

  # ac-dev-server: NOT included (Phase 9 uses bind mounts + Dockerfile.dev-server)
```

### Volume Naming

| Volume | Docker Volume Name | Purpose |
|--------|-------------------|---------|
| `mysql-data` | `teeter-<project.name>-mysql` | All 3 DBs (auth, world, characters) |
| `extracted-data` | `teeter-<project.name>-extracted-data` | Maps, vmaps, mmaps |
| `config` | `teeter-<project.name>-config` | Runtime configs (optional) |
| `client-data` | `teeter-<project.name>-client-data` | WoW client for extractors |

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

**Note**: AC's official compose only health-checks the database. Auth/world servers depend on dbimport completion. We follow this pattern — no TCP port health checks for auth/world.

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

### Source: `static` (only supported source)
Downloads pre-extracted `Data.zip` from `wowgaming/client-data` releases (1.11 GB).
No extraction needed, just unzip to `extracted-data` volume.
`version` field specifies release tag (e.g., `v20.0`).

---

## Validation & Migration Lifecycle

### Build Context as Validation Harness

Each build context is a **self-contained test environment**:

```bash
# 1. Build worldserver image (content-addressable, no volumes)
teeter build

# 2. Validate with ephemeral DB (dry-run migration)
teeter validate --buildctx <hash>
# → Spins up ac-database + ac-db-import on EPHEMERAL volume
# → Runs dbimport → applies ALL SQL (new + existing updates)
# → Starts authserver + worldserver briefly
# → Health checks pass → validation succeeds
# → Tears down ephemeral volume
# → Output: "Would apply 47 new updates from mod-auctionhouse. Estimated 30s. No errors."

# 3. Backup persistent DB
teeter backup --output backup-$(date +%s).sql.gz

# 4. Migrate persistent DB (run dbimport against REAL volume)
teeter migrate --buildctx <hash>
# → Starts dbimport container pointing to PERSISTENT mysql volume
# → Applies only new updates (idempotent)
# → Exits

# 5. Switch runtime to new buildctx (atomic)
teeter switch --buildctx <hash>
# → ln -sfn ../buildctx/<newhash> .teeter/runtime/buildctx
# → docker compose -f .teeter/runtime/docker-compose.yml up -d

# 6. Rollback if issues
teeter switch --buildctx <oldhash>
```

### Key Properties

- **Validation is safe** — ephemeral volume, zero risk to persistent data
- **Migration is idempotent** — dbimport tracks applied updates; only new ones run
- **Switch is atomic** — symlink flip, then `docker compose up -d`
- **Rollback is instant** — flip symlink back, restart

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
| `teeter-*.conf` | `teeter compose` | dbimport/authserver/worldserver configs |

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
| `teeter build` | 4 | Build worldserver image |
| `teeter doctor` | 4 | Validate system resources |
| `teeter up` | 5 | Full startup (pull→buildctx→compose→up→wait) |
| `teeter down` | 5 | Shutdown (`--volumes`, `--rmi`) |
| `teeter status` | 5 | Show service health |
| `teeter validate --buildctx` | 5 | Validate build context with ephemeral DB |
| `teeter migrate` | 5 | Migrate persistent DB using build context |
| `teeter switch` | 5 | Atomically switch runtime to build context |
| `teeter backup` | 8 | Backup DB + volumes |
| `teeter restore` | 8 | Restore from backup |
| `teeter client-data` | 6 | Manage client data |
| `teeter cache` | 7 | Cache management |
| `teeter list` | 7 | List available modules |

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
8. **Stable data volumes** — Named by `project.name`, persist across build context changes
9. **Thin runtime** — Symlink farm to build context; atomic switch via `teeter switch`
10. **Validation before migration** — Ephemeral dry-run before touching persistent data