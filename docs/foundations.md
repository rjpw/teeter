# System Foundations

The `teeter` project is all about solo play with helpful bots, and tinkering to make the system yours. As of this writing (late November 2026), running AzerothCore with `mod-playerbots` is best achieved by starting with its fork [mod-playerbots/azerothcore-wotlk](https://github.com/mod-playerbots/azerothcore-wotlk).

Quoting from their documentation:

> You must use the Playerbots fork of AzerothCore. The standard azerothcore-wotlk repository will not work — you will get hundreds of compilation errors if you try to build mod-playerbots against the standard core.

Their starting point begins:

```bash
git clone https://github.com/mod-playerbots/azerothcore-wotlk.git --branch=Playerbot
cd azerothcore-wotlk/modules
git clone https://github.com/mod-playerbots/mod-playerbots.git --branch=master
```

From here they defer to the AzerothCore docs for how to install a server. There is a document to [Install with Docker](https://www.azerothcore.org/wiki/install-with-docker), and I've done so with some success. But I've also stubbed my toe several times when adding mods.