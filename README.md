# Impasse

A competitive, tick-based maze game played over SSH and over a bot API. The world is
rendered in real-time 3D from a 45 degree top-down angle straight into the terminal.
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
ssh client ──ssh──> ssh3dmulti ──pty──> x3dmulticlient (one per session)
                         │                     │
                    world state          unix socket
                         └─────────────────────┘
```

`ssh3dmulti` is the SSH server. For every session it accepts it spawns a separate
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
| `cmd/ssh3dmulti` | SSH server, session handling, world state |
| `cmd/x3dmulticlient` | Renderer and client, one process per session |
| `gfx` | Terminal output. Pixels to Unicode blocks, colour, screen setup |
| `x3d/opengl` | GL layer. Shaders, meshes, textures, framebuffer, camera |
| `x3d` | Scene data and bounding volumes. X3D loading is being retired |

### Current architecture vs target

The inherited codebase is not shaped like the game. Worth knowing before reading it.

**The server holds no game state.** `cmd/ssh3dmulti/server.go` is a relay. It forwards
lines between clients and simulates nothing. Clients are authoritative over their own
positions and simply announce them. Impasse needs the inverse, where the server owns
the world and resolves ticks.

**There is no collision detection.** `Camera.Forward` adds a vector to a position.
Nothing consults level geometry. You can walk through every wall.

**The world is X3D triangle soup** loaded from Quake maps. The Impasse world is an
authoritative 2D cell grid with geometry generated from it, so the X3D loader
(`x3d/parser.go`, `x3d/scene.go`) is on its way out. The GL layer beneath it stays.

**The wire protocol** is a line format (`h`/`p`/`l` for hello, position, leave)
carrying client-authoritative positions. It is being replaced by NDJSON carrying queued
actions and authoritative tick state.

### Protocol direction

One protocol, two listeners. NDJSON over the unix socket for SSH renderers, and the
same over TCP for bots. That makes milestone 4 a listener rather than a second
protocol.

### Scale and units

1 tick is 0.6 seconds.

1 cell is 64 world units, with walls 96 units tall. The cell size is arbitrary in
principle, but the fragment shader carries Quake-scaled constants such as
`fogFar = 1500` and the light attenuation radius. At 64 units fog lands around 23 cells
out, which is a sensible fade for a clamped top-down view. If you change the cell size,
retune `x3d/opengl/texture.frag` with it.

`updateProjection` halves the vertical FOV to compensate for terminal cells being
roughly twice as tall as they are wide.

## Building and running

The nix flake provides Go, an SDL2 with `offscreen` enabled, and the GL runtime bits.

```sh
nix develop
go build -o bin/ssh3dmulti ./cmd/ssh3dmulti
go build -o bin/x3dmulticlient ./cmd/x3dmulticlient
```

Without nix you need an SDL2 built with `--enable-video-offscreen=yes` and
`PKG_CONFIG_PATH` pointed at it. SDL `dlopen`s libEGL and libGLESv2 by soname, so they
must be findable at run time. The flake puts libglvnd on `LD_LIBRARY_PATH` for that
reason.

Placeholder level data, until the grid pipeline lands:

```sh
wget https://gitlab.com/sascha.l.teichmann/quake-x3d/-/archive/main/quake-x3d-main.tar.gz
tar xfz quake-x3d-main.tar.gz
```

Run the server:

```sh
./bin/ssh3dmulti --renderer ./bin/x3dmulticlient -- \
    -scene quake-x3d-main/data/e1m1.x3d.gz
```

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

### Next

Milestone 1, the loop. In progress.

* `grid` package. ASCII parsing, walkability, 8-way movement with the corner rule.
* Extrude the grid into wall and floor meshes. Flat colours via the existing
  `useTexture=0` path, so no texture pipeline is needed.
* Server owns the grid and every player's cell, runs the 600ms tick, resolves queued
  actions simultaneously.
* Replace the `h`/`p`/`l` protocol with NDJSON.
* Client drops to a view. Send intent, receive authoritative state, interpolate,
  render continuously.
* 45 degree follow-cam, yaw clamped to plus or minus 30 degrees, clamped zoom.
* Retire `x3d/parser.go` and `x3d/scene.go` once the grid path renders.

Milestone 2, objectives. 4-tick loot channel and the in-world nearest-objective arrow,
pointed by straight-line bearing.

Milestone 3, stun. 1 cell range checked at cast, 1 tick startup, 2 tick duration, 3
tick cooldown, full loot reset.

Milestone 4, bot API. Same NDJSON protocol on a TCP listener.

Milestone 5, identity. SSH public key as account, one active session per key.

### Known issues

`x3d/opengl/shapes.go` uses `uint16` vertex indices with `MaxUint16` as the
primitive-restart marker. Any shape above 65535 unique vertices silently wraps and
corrupts geometry. The generated grid mesh needs chunking, or 32-bit indices.

The `geoms` flag threaded through `gfx.BlitRunes` into `NewRuneConverter` selects nine
extra geometric block glyphs. Nothing passes `true` any more, since the only caller
that did was a deleted binary. Kept as a real rendering capability, currently
unreachable.
