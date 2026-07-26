# Impasse

A competitive, tick-based maze game played over SSH and over a bot API. The world is
rendered in real-time 3D straight into the terminal. Players need no client install and
no GPU. The server renders and ships Unicode.

Impasse started as a fork of [ssh3d](https://gitlab.com/sascha.l.teichmann/ssh3d), an
experiment in rendering real-time 3D into a terminal over SSH. That experiment supplies
the rendering stack. The game is new work. MIT licensed, see [LICENSE](./LICENSE).

## Running it

You need a GitHub OAuth app with device flow enabled. Its client id is not a secret, but
it is kept out of the repo. The client secret is never used and must not be passed.

### Deploying with nix

`services.impasse` runs it as a systemd service and sets up the port, so nothing has to
be done by hand:

```nix
{
  inputs.impasse.url = "github:Liam-Weitzel/Impasse";

  # in the NixOS configuration
  imports = [ inputs.impasse.nixosModules.default ];

  services.impasse = {
    enable = true;
    clientIdFile = "/run/secrets/impasse-github-client-id";
    openFirewall = true;
  };
}
```

The client id comes from a file rather than a string so it stays out of the world
readable nix store, and systemd passes it in through `LoadCredential`. Options are
`port` (default 22), `botPort`, `map`, `package`, `openFirewall` and
`lowerUnprivilegedPorts`.

### Running it by hand

```sh
nix run . -- --map maps/vault.txt        # or: nix build .#impasse
```

`nix build` wraps the renderer with the GL library paths baked in, so it does not depend
on the environment it is started from, and points the server's `--renderer` at it. To
work on the code instead:

```sh
nix develop
go build -o bin/impasse-server ./cmd/impasse-server
go build -o bin/impasse-client ./cmd/impasse-client

export IMPASSE_GITHUB_CLIENT_ID=Ov23li...
./bin/impasse-server --map maps/vault.txt --renderer ./bin/impasse-client
```

### Port 22

It listens on port 22, which normally needs privilege. Lower the floor on what an
unprivileged process may bind instead of giving the process any privilege:

```sh
sudo sysctl -w net.ipv4.ip_unprivileged_port_start=22
```

The NixOS module does this for you whenever `port` is below 1024. To set it by hand and
keep it across reboots, `boot.kernel.sysctl."net.ipv4.ip_unprivileged_port_start" = 22;`.
The knob covers IPv6 as well despite the name. Use `--port` for an unprivileged port
instead.

Do not use `setcap cap_net_bind_service+ep` on the server. It binds the port, but every
renderer then dies with "Could not initialize OpenGL / GLES library". A child of a
file-capability binary execs with `AT_SECURE=1`, and glibc in that mode ignores
`LD_LIBRARY_PATH` when resolving libraries, including for `dlopen`. SDL loads libEGL by
`dlopen`. The variable is still in the environment, which makes this look fine until you
check whether the loader honours it. Running as root avoids the problem but puts every
player's renderer under root.

### Connecting

Connect as a human. No SSH key needed:

```sh
ssh localhost -o "UserKnownHostsFile /dev/null" -o "StrictHostKeyChecking=no"
```

Sign in with GitHub when the menu asks, then fetch a bot token from the menu or with
`ssh localhost token`, and run a bot:

```sh
python3 examples/bot.py --address 127.0.0.1:2223 --token <token>
```

Do both at once and the SSH session is a spectator view of your own bot, on the same
character. Your keys still work, so you can take over mid game.

Use a fast truecolor terminal such as kitty, Alacritty or Konsole. On VTE-based
terminals like GNOME and XFCE, set `COLORTERM=truecolor` and add
`-o 'SendEnv COLORTERM'`.

### Flags

| Flag | Meaning |
| --- | --- |
| `--map` | ASCII map to load |
| `--port` | SSH port, default 22 |
| `--bots` | Bot API address, default `:2223`, empty to disable |
| `--db` | Score database, default `impasse.db` |
| `--match` | Match length, default 2m |
| `--intermission` | Break between matches, default 15s |
| `--github-client-id` | Also read from `IMPASSE_GITHUB_CLIENT_ID` |
| `--github-client-id-file` | Read the client id from a file, for systemd credentials |

Renderer flags go after `--` and are passed through: `-tiles` for a replacement tile
atlas, `-log` for a log file, `-idle` and `-duration` for session limits.

### Building without nix

You need an SDL2 built with `--enable-video-offscreen=yes` and `PKG_CONFIG_PATH` pointed
at it. Distro packages almost never enable that driver, so this usually means building
SDL from source.

SDL `dlopen`s libEGL and libGLESv2 by soname, so they are in no RUNPATH and have to be
findable at run time. The dev shell puts them on `LD_LIBRARY_PATH`; the built package
bakes the same paths into a wrapper around the renderer, which is why it runs correctly
from an empty environment.

## How it works

```
ssh client ──ssh──> impasse-server ──pty──> impasse-client (one per session)
                         │                     │
                    world state          unix socket
                         └─────────────────────┘
```

`impasse-server` is the SSH server. For every session it accepts it spawns a separate
renderer process on a pty and pipes the two together, so the renderer writes escape
sequences straight to the player's terminal. Renderers talk back over a unix socket.

The renderer draws with OpenGL ES 3.1 into an offscreen framebuffer, reads the pixels
back, and converts them to coloured Unicode block characters. Each terminal cell carries
a 4x8 pixel patch, matched against a table of block glyphs to pick the best
foreground/background split. That code is `gfx/runeconverter.go` and it is the hot path.

This needs an SDL2 with the `offscreen` video driver, which distro packages almost never
enable. The nix flake builds one from source.

| Path | Role |
| --- | --- |
| `cmd/impasse-server` | SSH server, session handling, menu, world state |
| `cmd/impasse-client` | Renderer and client, one process per session |
| `grid` | The world model. Map parsing, walkability, movement rules |
| `proto` | Wire format shared by the server and every client |
| `gfx` | Terminal output. Pixels to Unicode blocks, colour, screen setup |
| `render` | GL layer. Shaders, meshes, textures, framebuffer |
| `examples` | Reference bot |
| `tools` | Map generator |
| `nix` | NixOS module for running it as a service |

## The world

A flat 2D grid of cells, held by the server and simulated there. The 3D geometry is
generated from the grid, never the other way round.

Movement is four way on `WASD` and world locked, so `W` is always north whatever the
camera is doing. There are no diagonals, so distance is Manhattan and cells touching
only at a corner are not connected.

`Space` loots, `E` stuns, the arrow keys move the camera and `+`/`-` zoom. `Esc` goes
back to the menu without dropping the connection, and `Ctrl-C` ends the session. Zooming
out stops before the camera reaches the fog distance, past which there is nothing to see
anyway.

The server runs a 600ms tick. Clients queue one action, queuing again before the tick
locks replaces it, and every queued action resolves at once when the tick fires. The
client interpolates between the last two ticks so movement looks smooth, but nothing
about resolution is continuous.

Players stack. Nothing blocks a move onto an occupied cell, so geometry is the only
thing that can refuse a move.

### Objectives

Pickups are marked `*` in the map. Standing on one and channelling for four ticks
collects it. The channel is per player, so several people can race for the same pickup
and each makes their own progress, and it resets to nothing the moment that player does
anything else.

A pickup only comes free when exactly one player completes a channel on that tick. If
two or more finish together nobody takes it, and they sit there at a full channel until
one of them stops. The tick after that, the other collects. This is the standoff the
game is named for.

### Stun

`E` bursts every other player in the 3x3 around you. Area of effect, no target
selection.

One tick of startup, then it holds a victim for two ticks and fully wipes their loot
channel. Targets are chosen when the burst is cast, not when it lands, so stepping out
during the startup tick does not save you and stepping in does not catch you.

The cooldown is three ticks and has to stay above the two tick duration. Cast at T lands
at T+1 and holds through T+2, so a cooldown of two would let the next burst land at T+3
and a single attacker could keep someone stunned forever. At three the victim gets
exactly one free tick per cycle. `TestStunLockLeavesExactlyOneFreeTick` measures this.

Casting is not looting, so it drops the caster's own channel. That is what makes
attacking into a standoff lose it: the opponent becomes the sole finisher on the very
tick you cast, a full tick before your burst can land. Stun earns its keep earlier,
while someone is still climbing their channel.

A burst in flight is public. `casting` counts the ticks until it lands, and the client
paints the 3x3 it will cover. This is resolved state rather than queued intent, so it
gives nothing away about what anyone means to do next.

### Matches

Two minutes, then a fifteen second break, then the next one. At the start of a match
every pickup is restored, every score resets, and everyone is put back on the spawn.
That is the objective respawn rule. Between matches there is nothing to collect and
nothing scores, but the scores from the match just finished stay up.

Dropping out and reconnecting inside one match gives your score and position back.
Nothing carries across a match boundary.

Rounds also stop the standoff rule from wedging a session: two players deadlocked over
the last pickup only hold it until the clock runs out.

## Identity

**A player is a GitHub account.** Not an SSH key, and not a machine. Keys are free, so a
keypair cannot be an identity: one person could hold as many players as they cared to
run `ssh-keygen`. The server refuses to start without a client id configured.

Signing in uses GitHub's device flow. The server shows a code, you enter it at
github.com/login/device, and the server polls until you have. No callback URL, no HTTPS
listener and no client secret, only outbound requests.

SSH keys are optional. If your client offers one it is remembered against your player so
that machine skips signing in next time, and a second machine's key points at the same
player. With no key you sign in each session.

An account may hold **one terminal and one bot** at once, both driving the same
character, and whichever queues an action last before the tick locks is the one that
runs. That is what makes watching your own bot and taking over from it work. A second
terminal is refused, because two terminals on one character would mean two cameras with
no answer for which is the real view. Which kind a connection is comes from the token it
presented, not from anything it claims.

Accounts key on the numeric GitHub id rather than the login, since logins can be renamed
and the old one then becomes available to somebody else.

Scores live in SQLite. Best is tracked apart from total, because ranking on total alone
measures who left their bot running longest.

## Protocol

JSON objects, one per line. One protocol, two listeners: a unix socket for the SSH
renderers and TCP for bots. A bot and a human are the same thing to the server.

`welcome` carries the player id, the tick length, the rules and the map. Sending the
rules rather than letting clients hardcode them means a client's display and the
server's behaviour cannot drift apart.

`state` is a full snapshot per tick: every player with position, score, channel, stun
and cast state, plus the pickups still uncollected and the match clock. Because it is a
snapshot and not a delta, a client that falls behind can throw away stale ones and lose
nothing.

`queue` asks for an action on the next tick: a move with a direction, a loot, or a stun.
Sending another before the tick locks replaces the first.

Bots get raw cells, never a graph. Turning the map into something you can search is the
bot author's job, and `examples/bot.py` shows the whole of it in about thirty lines.

## Maps

Plain ASCII. `#` is wall, `.` is floor, `S` is the spawn point and `*` is an objective.
Edit one in any text editor and restart the server.

Every player enters on the `S`, so the start of a match is a scramble out of the same
door. A map may hold at most one, and two is an error. A map with no `S` still loads,
falling back to the first cell of the largest region. The server warns at startup about
cells it cannot reach from the spawn, since that usually means a sealed off pocket.

`tools/genmap.py` generates them, with mixed room sizes, corridors of width one and two,
loops rather than a tree, and pillars in the halls. Movement is four way, so anything
joined only at a corner is not joined at all: the generator flood fills from the spawn,
deletes what it cannot reach, and refuses to write a map that fails its own check.

```sh
python3 tools/genmap.py --out maps/vault.txt --width 160 --height 90 --seed 7
```

## Rendering

Surfaces sample a tile atlas: one texture of square tiles, so the whole world is one
bind and the mesh never needs sorting by material. Tiles are picked per cell from a hash
of its coordinates, deterministic, so every client draws the same map and a wall does
not shimmer as the camera moves. `-tiles atlas.png` replaces the tileset.

Tiles are 16x16 and that is not a placeholder. A 4x8 pixel patch becomes one terminal
character, so a game cell is only a few dozen pixels across even zoomed in. Detail finer
than that is averaged away. What survives is value contrast and large shapes, which is
what the palette is built around.

The atlas UV rectangle is inset, because linear filtering otherwise reaches into the
neighbouring tile and draws a bright seam along every cell edge. Mipmaps are off for the
same reason at distance.

Players are spheres and pickups are diamonds, drawn untextured in solid colour so they
read against whatever the ground is doing, and so the two never read as the same object.
Pickups spin and bob. The stun telegraph is a frame rather than a filled square, so it
does not hide whoever is standing in it, and it swells across its tick.

A terminal cell is a 4x8 patch of the framebuffer, so the renderer asks for exactly four
times the column count by eight times the row count. Above 1360x768 pixels, meaning
about 340 columns by 96 rows, it stops growing and the patches get resampled instead,
which is softer than an exact fit.

Geometry is built one 16x16 block of cells at a time and chunks outside the view frustum
are dropped every frame. On a 200x120 map that draws about 6 chunks out of 208. The test
is conservative: a chunk goes only when every corner of its box falls outside the same
plane, so culling can keep something it did not need to but can never remove something
on screen.

`glReadPixels` returns rows from the bottom left while `image.RGBA` row 0 is the top, so
the projection mirrors clip space vertically. Without it the picture arrives upside
down. That mirror reverses triangle orientation, so **front faces are clockwise**.

The clear colour and the fog colour are one value. If they differ, distance fades into
one colour and then meets a hard edge of another.

## Tests

```sh
nix develop -c bash -c 'go vet ./... && gofmt -l . && go build ./... && go test ./...'
```

Renderer behaviour needs a real pty. Driving `ssh` under `script -qec` with a `timeout`
gives scriptable headless clients. The renderer can also be run against a stub server
speaking the protocol on a unix socket, which is how the look is checked without going
through sign in.

`go vet` panics inside its own `hostport` analyzer if you write
`net.Dial(parseAddr(addr))`, spreading a two value call into the two parameters. Not our
bug, but it takes the whole vet run down, so the tests split the call in two.
