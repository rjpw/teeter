# teeter
![](docs/images/teeter.webp)

A [narrow-gauge](https://en.wikipedia.org/wiki/Narrow-gauge_railway) mod manager for AzerothCore. Some assembly required.

> Up and awa ... oh!

## Motivation

If you've ever modded Skyrim (for me it was "Oldrim") you'll know there are excellent tools to customize your virtual world. Such is not the case for a lesser-known open world solo RPG, namely `AzerothCore-WotLK`, played solo with `mod-playerbots`.

I know. A table for one, and a `CLOSED` sign on the door of this massively multiplayer online role-playing game? When I play with other people, I prefer the official [World of Warcraft](https://worldofwarcraft.blizzard.com/en-us/) experience. When I want to sharpen my DevOps skills, I build the Azeroth equivalent of a train set in the basement, and when it falls down, nobody's progression is hampered.

## Assumptions

The `AzerothCore` team publishes a [catalogue](https://www.azerothcore.org/catalogue#/) of mods, tools, and scripts. The `teeter` CLI tool is meant to help smoosh those together into a teetering, tottering, but hopefully cooperating assemblage of open-source shinies.

I'm a DevOps guy, and I'm going to assume you're like me and you don't feel like cluttering up your systems with a dozen different database engines, build tools, templating kits, and assorted gnomish foot cannons.

For that reason, you're going to get `teeter` to help you compose a compact and repeatable Docker Compose file, you're going to feed that composition the most lovingly crafted manifest, and you're going to treat the resulting edifice like the fragile [Jenga](https://en.wikipedia.org/wiki/Jenga) tower that it is.

The good news is, `teeter` will hopefully help you stand the tower back up when your cat knocks it over.

## Installation & Quick Start

*Under construction.*

Once the initial build is wired up, you'll be able to install `teeter` via Go:

```sh
go install github.com/rjpw/teeter@latest
```

And check available commands with:

```sh
teeter --help
```

## Documentation

* [Design & Architecture](docs/design.md) *(coming soon)*
* [Manifest Specification](docs/manifest.md) *(coming soon)*

## License

This project is licensed under the [MIT License](LICENSE.md).
