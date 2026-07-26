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
| `examples` | Reference bot |

### World model

The world is a flat 2D grid of cells, held by the server and simulated there. The 3D
geometry is generated from the grid, never the other way round.

`grid` is pure data with no GL and no networking. It parses the ASCII map, answers
walkability, and owns the movement rule. Movement is four way on `WASD` and world
locked, so `W` is always north whatever the camera is doing. There are no diagonals, so
there is no corner to cut and no corner rule, and distance is Manhattan. Cells touching
only at a corner are not connected.

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

### Matches

The world runs in rounds. A match lasts two minutes, then there is a fifteen second
break, then the next one starts. Both are configurable with `--match` and
`--intermission`.

At the start of a match every pickup is restored, every score resets, and everyone is
put back on the spawn. That is the objective respawn rule: pickups come back when the
match does. Between matches there is nothing to collect and nothing scores, but the
scores from the match just finished stay up so you can see how it went.

The clock is in every `state` message, so bots and human players both see the phase, the
match number and how many ticks are left. How long is left is what decides whether a
pickup across the map is worth starting for.

Rounds also stop the standoff rule from wedging a session. Two players deadlocked over
the last pickup only hold it until the clock runs out.

### Scores

Results go into SQLite, keyed by the account fingerprint. `best` is the high water mark
for a single match and `total` accumulates, because ranking on total alone just measures
who left their bot running longest. The leaderboard sorts on best, then total.

Display names are cosmetic and changeable. The fingerprint is the identity underneath,
so renaming yourself does not move anyone else's scores.

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

### The menu

Connecting drops you in a pre-game menu rather than straight into the world. It runs in
the server process, over the SSH session, because it needs the store and the live world.
The renderer is a separate process and is only spawned once you choose Play.

It shows who you are signed in as and your record, the leaderboard, who is in the world
right now and what is driving them, your bot token, and the same match countdown the in
game HUD shows. Display names can be changed there.

It is bubbletea and lipgloss. Over SSH there is no local terminal to probe, so the
colour profile comes from the client's `TERM` and `COLORTERM` rather than from
auto detection, and window size comes from the pty request and the resize channel
instead of from an ioctl.

### Identity

**A player is a GitHub account.** Not an SSH key, and not a machine.

Keys are free, so a keypair cannot be an identity: one person could hold as many
players as they cared to run `ssh-keygen`. A GitHub account is harder to farm, so that
is what a player is, and the server refuses to start without one configured.

Signing in uses GitHub's **device flow**. The server shows a code, you enter it at
github.com/login/device, and the server polls until you have. There is no callback URL,
no HTTPS listener and no client secret. Nothing has to be publicly reachable, because
the server only makes outbound requests.

Run it with the client id, which is not a secret but is kept out of the repo anyway:

```sh
export IMPASSE_GITHUB_CLIENT_ID=Ov23li...
./bin/impasse-server --map maps/big.txt
```

The **client secret is never used and must never be passed**. Device flow does not need
one. If you have generated a secret, it does nothing here.

**SSH keys are optional.** You do not need one to connect and you do not need one to
play. If your client offers a key, it is remembered against your player so that machine
skips signing in next time. A second machine's key points at the same player, because
the rule is one person one player, not one machine one player. With no key you simply
sign in each session.

Accounts key on the numeric GitHub id rather than the login, since logins can be
renamed and the old one then becomes available to somebody else.

### Rendering

Geometry is built one 16x16 block of cells at a time, so each shape covers a known patch
of ground. Every frame the chunks that cannot be on screen are dropped by testing their
bounding box against the view frustum. On a 200x120 map that draws about 6 chunks out of
208. Splitting by vertex count instead would scatter each shape across the whole map and
leave nothing usefully cullable, which is why the mesh is chunked by region.

The test is conservative. A chunk is only dropped when every corner of its box falls
outside the same frustum plane, so culling can keep something it did not need to, but
can never remove something that is on screen.

`glReadPixels` returns rows starting at the bottom left, while `image.RGBA` row 0 is the
top. The projection mirrors clip space vertically to cancel that out. Without it the
picture reaches the terminal upside down. Because the mirror reverses triangle
orientation on screen, **front faces are clockwise**.

### Textures

Surfaces sample a tile atlas: one texture holding a grid of square tiles, so the whole
world is one bind and the mesh never has to be sorted by material. Tiles are chosen per
cell from a hash of its coordinates, which is deterministic, so every client draws the
same map the same way and a wall does not shimmer as the camera moves.

Tiles are 16x16 pixels, and that is not a placeholder. The renderer turns a 4x8 pixel
patch of the framebuffer into one terminal character, so a game cell is only a few dozen
pixels across even zoomed in. Detail finer than that is averaged away before anyone sees
it. What survives is value contrast and large shapes.

The atlas UV rectangle is inset by a fraction of a tile, because linear filtering
otherwise reaches into the neighbouring tile and draws a bright seam along every cell
edge. Mipmaps are off for the same reason: they blend neighbouring tiles together at
distance.

`-theme` on the renderer picks the look, and `--tiles atlas.png` replaces the tileset. With no path it generates one
in code, so the game always has something to look at and a missing file is never fatal.
A path that fails to load is an error rather than a silent fallback, since a typo would
otherwise look like the art simply not arriving.

Players, pickups and the pointer are never textured. They are solid colour so they stay
readable against whatever the ground is doing.

### Look

Two themes, `matrix` and `gritty`, chosen with `-theme` on the renderer. A theme owns
everything that has to agree: the clear colour and the fog colour are one value, so
distance fades into the void rather than into a seam, and player colours are confined to
a slice of the hue wheel so one off palette marker cannot read as a bug. See
[`docs-art.md`](./docs-art.md).

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

The server warns at startup about cells that cannot be reached from the spawn, since
that usually means a sealed off pocket rather than a deliberate choice.

Connect as a human. No SSH key needed:

```sh
ssh -p2222 localhost -o "UserKnownHostsFile /dev/null" -o "StrictHostKeyChecking=no"
```

Sign in with GitHub when the menu asks. Then fetch your bot token, from the menu or
with:

```sh
ssh -p2222 localhost token
python3 examples/bot.py --address 127.0.0.1:2223 --token <token>
```

Do both at once and the SSH session is a spectator view of your own bot, on the same
character. Your keys still work, so you can take over from it mid game.

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

Art:

* Two themes with their own palettes, tile art, fog, and player colours.
* Fog colour and clear colour are one value, and fog distance is per theme.
* The stun telegraph is a frame rather than a filled square, so it does not hide
  whoever is standing in it, and it swells over its tick.
* The pointer and pickups bob, on the wall clock so they do not stutter when a tick
  is late.
* `tools/genmap.py` generates maps and refuses to write an unreachable one.

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

The pre-game menu:

* Play, leaderboard, live player list, display name, bot token, quit.
* Match countdown, the same one players in the world see.
* Runs in the server over the SSH session, then hands the terminal to the renderer.

GitHub sign in, required:

* Device flow, so no web listener and no client secret.
* A player is a GitHub account. The server will not start without a client id.
* SSH keys are optional and are only a remembered convenience, never an identity.
* Several machines reach one player.

Milestone 5, identity:

* SSH public key is the account. No registration, no password.
* One account, one character. One terminal and one bot may drive it, which is what
  makes spectating your own bot work. Any more of either is refused.
* Token handshake on the protocol. One shot tokens for renderers, long lived ones for
  bots via `ssh <host> token`.
* The character leaves only when the last connection driving it does.

Matches and scores:

* Two minute matches with a fifteen second break, both configurable. Pickups, scores
  and positions reset at the start of each one.
* The round clock is published to bots and humans alike.
* SQLite store for accounts, names, best and total scores, and match counts.

### Next

Textures. Surfaces are flat colours and the MVP needs a tileset.

Two minutes per match is a starting point rather than a considered number. Tune it with
`--match` once it has been played properly.

### Known issues

`go vet` panics inside its own `hostport` analyzer if you write
`net.Dial(parseAddr(addr))`, spreading a two value call into the two parameters. Not our
bug, but it takes the whole vet run down, so the tests split the call in two.

The `geoms` flag threaded through `gfx.BlitRunes` into `NewRuneConverter` selects nine
extra geometric block glyphs. Nothing passes `true` any more, since the only caller
that did was a deleted binary. Kept as a real rendering capability, currently
unreachable.
