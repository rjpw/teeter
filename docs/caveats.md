# ⚠️ Caveats

## AI Disclosure

This project was developed with significant assistance from AI coding agents. Because I'm using a free model (which makes me feel less threatened with replacement than the frontier ones) all contributions have been reviewed by the human author, though they may contain residual AI-generated patterns, conventions, and definitely artwork.

## Personal Project Notice

This project is purely for **personal edification and entertainment**. It is not intended for production use, public servers, or any mission-critical deployments. Use at your own risk. Did I say it's called `teeter` for a reason?

## Legal Disclaimers

**Blizzard / Legal Notice**
This tool interacts with AzerothCore, which emulates [World of Warcraft](https://worldofwarcraft.blizzard.com/en-us/). World of Warcraft is a trademark of Blizzard Entertainment, Inc. This project is not affiliated with, endorsed by, or sponsored by Blizzard Entertainment. Use at your own legal risk.

**Non-Affiliation Statement**
The project's author is not affiliated with the AzerothCore team and makes no claims as to the suitability of this project with their much larger and more remarkable open-source achievement.

## Known Limitations

- **No dev mode** — Live module development with bind mounts not implemented
- **Single-server only** — No multi-server / clustering support
- **Static catalogue only** — Default uses static index; `--api` requires GitHub token
- **No automated module updates** — Version bumps require manual manifest edits
- **MySQL 8.4 only** — Hardcoded in docker-compose; not configurable
- **No ARM64 images** — Upstream AC images are amd64 only