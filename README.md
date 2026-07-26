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

### Protocol

JSON objects, one per line. `welcome` carries the player id, the tick length and the
map. `state` carries every player as of the last resolved tick, and is a full snapshot,
so a client that falls behind can drop stale ones and lose nothing. `queue` goes the
other way and asks for an action on the next tick.

One protocol, two listeners. Unix socket for SSH renderers now, TCP for bots later.
That makes milestone 4 a listener rather than a second protocol.

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

Maps are plain ASCII. `#` is wall, `.` is floor and `S` is the spawn point. Edit one in
any text editor, restart the server, and the new geometry is there.

Every player enters on the `S`, so the start of a round is a scramble out of the same
door. A map may hold at most one, and two is an error. A map with no `S` still loads,
falling back to the first cell of the largest region, and the server says so.

`maps/open.txt` is the one to use for movement testing. It is wide open in places so
diagonals have room to work, and it has single cell gaps and blocks meeting at corners
where the corner rule refuses them. `maps/test.txt` is a tight maze where diagonals
almost never apply.

The server warns at startup about cells that cannot be reached from the spawn, since
that usually means a sealed off pocket rather than a deliberate choice.

Connect:

```sh
ssh -p2222 localhost -o "UserKnownHostsFile /dev/null" -o "StrictHostKeyChecking=no"
```

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

### Next

Milestone 2, objectives. 4-tick loot channel and the in-world nearest-objective arrow,
pointed by straight-line bearing.

Milestone 3, stun. 1 cell range checked at cast, 1 tick startup, 2 tick duration, 3
tick cooldown, full loot reset.

Milestone 4, bot API. Same NDJSON protocol on a TCP listener.

Milestone 5, identity. SSH public key as account, one active session per key.

### Known issues

Nothing automated covers the socket path. `world_test.go` drives the simulation
directly, so the welcome handshake, the JSON encoding and disconnect cleanup are only
verified by hand. Worth an end to end test that spins the server on a temp socket and
drives it with a real client, probably alongside milestone 4 when the same protocol
gets a TCP listener.

The `geoms` flag threaded through `gfx.BlitRunes` into `NewRuneConverter` selects nine
extra geometric block glyphs. Nothing passes `true` any more, since the only caller
that did was a deleted binary. Kept as a real rendering capability, currently
unreachable.
