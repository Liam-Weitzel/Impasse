# Art direction

Two looks, both built, switchable with `-theme` on the renderer. Nothing is
committed to as final. Run each and pick, or take pieces of both.

```sh
./bin/impasse-server --map maps/vault.txt -- -theme matrix
./bin/impasse-server --map maps/vault.txt -- -theme gritty
```

## What the medium allows

This decided more than taste did.

The renderer reduces a 4x8 pixel patch of the framebuffer to one terminal
character with a single foreground and background colour. A game cell spans
roughly 8 to 12 characters at normal zoom, so a floor tile arrives as about two
dozen colour samples in total.

Fine detail is averaged away before anyone sees it. What survives is **value
contrast** and **large shapes**. Both themes are built from that constraint
rather than against it: dark grounds, one dominant hue, and all the saturation
spent on the few things you need to see.

## The Green (`-theme matrix`)

**The story.** Impasse is a game about writing programs, so this look takes the
maze as what it literally is: a data structure being walked. The world is a
terminal inside a terminal. Floors are the faint lattice of an empty
allocation. Walls are lit traces carrying something downwards. Pickups are
values nobody has read yet.

You are a process. So is everything else in here, and the ones that are not
yours are racing you to the same memory. A stun is a scheduler interrupt: it
does not damage you, it takes your turn. You see it coming for exactly one
tick, which is long enough to understand what is about to happen and not long
enough to stop it.

**The look.** Near black with one green hue. Floors carry a lattice at two
scales, walls carry vertical traces that brighten downwards, and the ground
under a pickup is a socket the value was pulled out of. Players are all the
same green, separated by brightness rather than hue, because they are all the
same kind of thing. Fog reaches further than in The Substrate, so the maze
recedes rather than closes in.

## The Substrate (`-theme gritty`)

**The story.** Same maze, opposite reading. This is not a clean abstraction, it
is a real place that has been executing the same loop for longer than anyone
remembers, and it shows. The floors are worn where other processes walked them.
The walls are oxidising. The last few values still worth anything glow, because
somebody put a lot of work into them a long time ago.

A stun here is not an interrupt, it is a fault. Something in you stops
responding for two ticks and you get to watch it happen.

**The look.** Warm dark greys and rust. Detail is blotchy rather than
structured, because wear is not regular and because blotches survive
downsampling better than lines. Wall streaks run downwards. Players are warm
signal colours against cold ground, so a player is the only thing on screen
that looks powered. Pickups are amber, the stun frame is red, and the fog is
tighter so the map feels like it is closing in.

## Effects

Both themes share the shapes and differ only in colour.

**The stun telegraph is a frame, not a filled square.** A filled one hides
whoever is standing in it, which is exactly what you need to see in the one
tick you have to react. It swells and brightens across its tick, because a
static shape on the floor reads as scenery and a moving one reads as a warning.

**The pointer bobs**, so the eye finds it without it having to be bright enough
to compete with the pickups it is pointing at.

**Pickups bob** too, on a slower cycle, so they are the only moving thing on an
empty floor.

Both use the wall clock rather than the tick, because they are decoration and
should not stutter when a tick arrives late.

## Maps

`tools/genmap.py` generates them. Rooms of mixed size so open fights have
somewhere to happen, corridors of width one and two, extra links so the graph
has loops rather than being a tree, and pillars in the big halls so open space
is not featureless.

Movement is four way, so anything joined only at a corner is not joined at all.
The generator flood fills from the spawn, deletes anything unreachable, and
refuses to write a map that fails its own check. A pickup behind a diagonal
pinch would otherwise just look like a bot standing next to it doing nothing.

```sh
python3 tools/genmap.py --out maps/vault.txt --width 160 --height 90 --seed 7
```

`maps/vault.txt` is 160x90 with 45 pickups. `maps/grid.txt` is a tighter
120x70. `maps/open.txt` is the small hand made one.

## What is not decided

Whether players should be spheres at all. They are the one shape that has had
no thought put into it, and at this resolution a sphere and a cube are nearly
the same handful of characters.

Whether the walls should be this tall. 96 units against a 64 unit cell reads
well from a steep angle and hides a lot when you tilt down.

Whether The Green should have scanlines. It would be cheap in the fragment
shader and might be too much.
