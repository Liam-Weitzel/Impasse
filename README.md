# Impasse

A competitive, tick-based maze game played over SSH and over a bot API. The world is
rendered in real-time 3D from a top-down angle straight into the terminal.
Players need no client install and no GPU. The server renders and ships Unicode.

This file is the technical reference and the progress log. The design document is kept
outside the repo.

Impasse started as a fork of [ssh3d](https://gitlab.com/sascha.l.teichmann/ssh3d), an
experiment in rendering real-time 3D into a terminal over SSH. That experiment proved
the idea works and supplies the rendering stack. The game is new work. MIT licensed,
see [LICENSE](./LICENSE).

## How it works

### Process model

```
ssh client ──ssh──> impasse-server ──pty──> impasse-client (one per session)
                         │                     │
                    world state          unix socket
                         └─────────────────────┘
```

`impasse-server` is the SSH server. For every session it accepts it spawns a separate
renderer process on a pty and pipes the two together, so the renderer writes escape
sequences straight to the player's terminal. Renderers talk back to the server over a
unix socket.

The renderer draws with OpenGL ES 3.1 into an offscreen framebuffer, reads the pixels
back, and converts them to coloured Unicode block characters. Each terminal cell
carries a 4x8 pixel patch, matched against a table of block glyphs to pick the best
foreground/background split. That code is `gfx/runeconverter.go` and it is the hot
path.

This needs an SDL2 built with the `offscreen` video driver, which distro packages
almost never enable. The nix flake builds one from source.

### Packages

| Path | Role |
| --- | --- |
| `cmd/impasse-server` | SSH server, session handling, world state |
| `cmd/impasse-client` | Renderer and client, one process per session |
| `grid` | The world model. Map parsing, walkability, movement rules |
| `proto` | Wire format shared by the server and every client |
| `gfx` | Terminal output. Pixels to Unicode blocks, colour, screen setup |
| `render` | GL layer. Shaders, meshes, framebuffer |

### World model

The world is a flat 2D grid of cells, held by the server and simulated there. The 3D
geometry is generated from the grid, never the other way round.

`grid` is pure data with no GL and no networking. It parses the ASCII map, answers
walkability, and owns the movement rule. Movement is 8 way on `QWEADZXC` and world
locked, so `W` is always north whatever the camera is doing. A diagonal needs both
adjoining orthogonal cells open, otherwise a player would slip between two walls
meeting at a corner and visibly clip through the geometry.

The server runs a 600ms tick. Clients queue one action, queuing again before the tick
locks replaces it, and every queued action resolves at once when the tick fires. The
client interpolates between the last two ticks so movement looks smooth, but nothing
about resolution is continuous.

Players stack. Nothing blocks a move onto an occupied cell, so geometry is the only
thing that can refuse a move.

Objectives are pickups marked `*` in the map. Standing on one and channelling for four
ticks collects it. The channel is per player, so several people can race for the same
pickup and each makes their own progress, and it resets to nothing the moment that
player does anything else.

A pickup only comes free when exactly one player completes a channel on that tick. If
two or more finish together nobody takes it, and they sit there at a full channel until
one of them stops. The tick after that, the other collects. This is the standoff the
game is named for.

Collected pickups do not come back. Respawn is undesigned.

### Stun

`S` bursts every other player in the 3x3 around you. It is area of effect, with no
target selection.

One tick of startup, then it holds a victim for two ticks and fully wipes their loot
channel. Targets are chosen when the burst is cast, not when it lands, so stepping out
during the startup tick does not save you, and stepping in does not catch you.

The cooldown is three ticks, and it has to stay above the two tick duration. Cast at T
lands at T+1 and holds through T+2, so a cooldown of two would let the next burst land
at T+3 and a single attacker could keep someone stunned forever. At three the victim
gets exactly one free tick per cycle. `TestStunLockLeavesExactlyOneFreeTick` measures
this rather than trusting the comment.

A burst in flight is public. `casting` on each player counts the ticks until it lands,
and the client paints the 3x3 it will cover on the floor for everyone to see. This is
resolved state rather than queued intent, so it gives nothing away about what anyone
means to do next, and it lands whatever the victim does. What it buys is letting
bystanders see one coming and decide whether to close in. Casting at empty air
telegraphs identically, so a wind up is not proof that anyone is in range.

Casting is not looting, so it drops the caster's own channel. That is what makes
attacking into a standoff lose it: the opponent becomes the sole finisher on the very
tick you cast, a full tick before your burst can land. Stun earns its keep earlier,
while someone is still climbing their channel, where spending one tick to erase two of
theirs pays off.

### Protocol

JSON objects, one per line. `welcome` carries the player id, the tick length and the
map. `state` carries every player as of the last resolved tick, and is a full snapshot,
so a client that falls behind can drop stale ones and lose nothing. `queue` goes the
other way and asks for an action on the next tick.

One protocol, two listeners. A unix socket for the SSH renderers and TCP for bots. A
bot and a human are the same thing to the server: same handshake, same messages, same
world, same tick. Nothing below the listener knows which it is talking to.

`welcome` carries the player id, the tick length, the rules (loot ticks, stun ticks,
cooldown and radius) and the map. Sending the rules rather than letting clients hardcode
them means a client's display and the server's behaviour cannot drift apart.

`state` is a full snapshot per tick: every player with position, score, channel, stun
and cast state, plus the pickups still uncollected. Because it is a snapshot and not a
delta, a client that falls behind can throw away stale ones and lose nothing.

`queue` goes the other way and asks for an action on the next tick: a move with a
direction, a loot, or a stun. Sending another before the tick locks replaces the first.

Bots get raw cells, never a graph. Turning the map into something you can search is the
bot author's job, and `examples/bot.py` shows the whole of it in about thirty lines.

Neither transport has any authentication yet. Anyone who can reach the port can join.
That is milestone 5.

### Rendering

`glReadPixels` returns rows starting at the bottom left, while `image.RGBA` row 0 is the
top. The projection mirrors clip space vertically to cancel that out. Without it the
picture reaches the terminal upside down. Because the mirror reverses triangle
orientation on screen, **front faces are clockwise**.

Nothing culls. The mesh is split by vertex count to stay under the `uint16` index
limit, not by region, so a bounding box per chunk would not help. Culling needs
spatial chunking first.

### Textures

There are none yet, and the MVP needs them. Surfaces currently take a flat colour from
the ambient and diffuse uniforms. When textures return they will be keyed to the grid
and a tileset rather than to per shape material data, so the vertex format gains back a
texture coordinate and the fragment shader gains a sampler. Nothing from the old X3D
texture cache was worth keeping, which is why it went.

### Scale and units

1 tick is 0.6 seconds.

1 cell is 64 world units, with walls 96 units tall. The cell size is arbitrary in
principle, but the fragment shader carries Quake-scaled constants such as
`fogFar = 1500` and the light attenuation radius. At 64 units fog lands around 23 cells
out, which is a sensible fade for a clamped top-down view. If you change the cell size,
retune `render/shape.frag` with it.

`updateProjection` halves the vertical FOV to compensate for terminal cells being
roughly twice as tall as they are wide.

## Building and running

The nix flake provides Go, an SDL2 with `offscreen` enabled, and the GL runtime bits.

```sh
nix develop
go build -o bin/impasse-server ./cmd/impasse-server
go build -o bin/impasse-client ./cmd/impasse-client
```

Without nix you need an SDL2 built with `--enable-video-offscreen=yes` and
`PKG_CONFIG_PATH` pointed at it. SDL `dlopen`s libEGL and libGLESv2 by soname, so they
must be findable at run time. The flake puts libglvnd on `LD_LIBRARY_PATH` for that
reason.

Run the server:

```sh
./bin/impasse-server --renderer ./bin/impasse-client --map maps/open.txt
```

Maps are plain ASCII. `#` is wall, `.` is floor, `S` is the spawn point and `*` is an
objective. Edit one in any text editor, restart the server, and the new geometry is
there.

Every player enters on the `S`, so the start of a round is a scramble out of the same
door. A map may hold at most one, and two is an error. A map with no `S` still loads,
falling back to the first cell of the largest region, and the server says so.

`maps/open.txt` is the one to use for movement testing. It is wide open in places so
diagonals have room to work, and it has single cell gaps and blocks meeting at corners
where the corner rule refuses them. `maps/test.txt` is a tight maze where diagonals
almost never apply.

The server warns at startup about cells that cannot be reached from the spawn, since
that usually means a sealed off pocket rather than a deliberate choice.

Connect as a human:

```sh
ssh -p2222 localhost -o "UserKnownHostsFile /dev/null" -o "StrictHostKeyChecking=no"
```

Or as a bot:

```sh
python3 examples/bot.py --address 127.0.0.1:2223
```

`examples/bot.py` walks to the nearest pickup and channels it. It is deliberately naive,
and greedy nearest is a bad strategy, since everyone else is racing you to the same
pickups. It exists to show the protocol, not to play well. Pass `--bots ""` to the
server to turn the API off.

Use a fast truecolor terminal such as kitty, Alacritty or Konsole. On VTE-based
terminals like GNOME and XFCE, set `COLORTERM=truecolor` and add
`-o 'SendEnv COLORTERM'`.

### Tests

```sh
nix develop -c bash -c 'go vet ./... && gofmt -l . && go build ./... && go test ./...'
```

Renderer behaviour needs a real pty. Driving `ssh` under `script -qec` with a `timeout`
gives scriptable headless clients, which is how multiplayer changes get verified. Run
two, have one disconnect, then check the survivor's log.

## Progress log

### Done

Environment:

* Nix flake with SDL2 built `--enable-video-offscreen=yes`.
* Fixed SDL failing to `dlopen` libEGL. libglvnd added to `LD_LIBRARY_PATH`.
* Fixed the SSH handler replacing the renderer's environment with the empty session
  env, which stripped the GL paths and broke every connection.

Bug fixes:

* Ghost players. Renderers are SIGKILLed when a session drops, so `leave` was never
  sent and disconnected players lingered forever. The server now tracks each
  connection's attendee id and broadcasts the leave itself.
* The graceful-quit `leave` raced with connection teardown and was usually lost. Added
  a flush with timeout on shutdown.
* `broadcast` blocked the server's command loop on a full client buffer, so one stuck
  client froze everyone. Now a non-blocking send.
* `closeCon` leaked a goroutine per connection and was not idempotent.
* `ReadImage` passed `image.Black.Bounds().Dy()`, which is two billion, as the
  `glReadPixels` height instead of the image's own.
* `gl.DeleteBuffers` was used to free texture names in two places.
* A swallowed `compileShapes` error meant a broken level rendered nothing silently.
* A closed-channel `break` inside a `select` spun a core at 100% forever.
* `flag.Parse()` ran after the flags were read, so `--connection` was ignored.
* HUD frame timing double-counted the Unicode conversion. `HSLToRGB` returned white for
  achromatic colours.

Cleanup:

* Deleted three superseded binaries: `ssh3dserver`, `x3dclient`, `ssh3dclient`. About
  1400 lines.
* Removed unused symbols across the shared packages. `deadcode ./cmd/...` is clean.
* Restored the test pipeline. `go vet` and `go test` were both failing on a stale test
  file that referenced a since-moved type and a missing embed. Replaced with real
  coverage of the rune converter.
* Renamed `BoundingSphere.Radius` to `RadiusSqr`. It was compared against squared
  distances, so every caller had to pre-square it with nothing saying so.

Milestone 1, the loop:

* `grid` package. ASCII parsing, walkability, 8-way movement with the corner rule.
* `opengl.MeshBuilder` builds shapes from quads and splits them before the uint16
  index limit.
* Grid extruded into floor, wall and wall top geometry. Untextured, flat colours.
* Server owns the world. 600ms tick, queued actions resolved simultaneously.
* NDJSON protocol replaced the old `h`/`p`/`l` relay format.
* Client is a view. Sends intent, interpolates between ticks, redraws at 15fps.
* Top-down follow-cam. Pitch 65 adjustable 35 to 85, yaw clamped to plus or minus 30
  degrees, clamped zoom.
* Fixed two orientation bugs. World Y ran south, which mirrored east and west. Frames
  reached the terminal upside down because of the `glReadPixels` row order.
* Retired the X3D loader, the first person camera and render path, the texture cache
  and the scene parser. About 1500 lines. `deadcode ./cmd/...` is clean.
* Renamed everything that still said ssh3d or x3d. Module is now
  `github.com/Liam-Weitzel/Impasse`, binaries are `impasse-server` and
  `impasse-client`, the GL package is `render`, and the env var is
  `IMPASSE_CONNECTION`.
* Added `S` to the map format for the spawn point. All players start there.
* `maps/open.txt`, a wide map for exercising 8 way movement.

Milestone 2, objectives:

* `*` in the map places a pickup. Four ticks of channelling collects it.
* Per player channels, reset by any other action.
* Simultaneous completion collects nothing. The standoff holds until one player
  stops, and the other takes it on the next tick.
* An in-world arrow above your own marker points at the nearest pickup by straight
  line bearing. It will point through walls on purpose. It says where, never how.
* Score and pickups remaining in the HUD.

Milestone 3, stun:

* `S` bursts the 3x3 around you. Area of effect, no target selection.
* 1 tick startup, 2 tick duration, 3 tick cooldown, full loot reset.
* Targets are picked at cast, so fleeing during startup does not help.
* Loot moved to space, since `S` is now the attack.
* In-flight bursts are published and drawn as a floor patch over the 3x3 they will
  cover, with CASTING in the HUD.

Milestone 4, bot API:

* TCP listener alongside the unix socket, same protocol and same world.
* The server takes a list of addresses rather than one, so transports are just
  entries in that list.
* `examples/bot.py`, a reference client that pathfinds and loots.
* End to end tests over a real socket, covering the handshake, JSON on the wire,
  movement, walls, looting, two clients seeing each other, disconnect cleanup and
  malformed input. Both transports.

### Next

Milestone 5, identity. SSH public key as account, one active session per key.

### Known issues

`go vet` panics inside its own `hostport` analyzer if you write
`net.Dial(parseAddr(addr))`, spreading a two value call into the two parameters. Not our
bug, but it takes the whole vet run down, so the tests split the call in two.

The `geoms` flag threaded through `gfx.BlitRunes` into `NewRuneConverter` selects nine
extra geometric block glyphs. Nothing passes `true` any more, since the only caller
that did was a deleted binary. Kept as a real rendering capability, currently
unreachable.
