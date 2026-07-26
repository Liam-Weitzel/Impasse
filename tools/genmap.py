#!/usr/bin/env python3
"""Generate an Impasse map.

    python3 tools/genmap.py --out maps/vault.txt --width 160 --height 90

Movement is four way, so anything joined only at a corner is not joined at all.
Every map is checked for that before it is written: a pickup behind a diagonal
pinch is a pickup nobody can reach, and it would only show up as a player
standing next to it looking confused.
"""

import argparse
import random
from collections import deque

WALL, FLOOR, SPAWN, OBJ = "#", ".", "S", "*"
DIRS = [(0, -1), (1, 0), (0, 1), (-1, 0)]


def carve_room(g, x, y, w, h):
    for j in range(y, y + h):
        for i in range(x, x + w):
            g[j][i] = FLOOR


def carve_corridor(g, x1, y1, x2, y2, width=1):
    """An L bend. Width 2 corridors let two players pass, which matters a lot
    when the only way to interfere with someone is to stand next to them."""
    for i in range(min(x1, x2), max(x1, x2) + 1):
        for w in range(width):
            if 0 <= y1 + w < len(g):
                g[y1 + w][i] = FLOOR
    for j in range(min(y1, y2), max(y1, y2) + 1):
        for w in range(width):
            if 0 <= x2 + w < len(g[0]):
                g[j][x2 + w] = FLOOR


def reachable(g, start):
    h, w = len(g), len(g[0])
    seen = {start}
    q = deque([start])
    while q:
        x, y = q.popleft()
        for dx, dy in DIRS:
            n = (x + dx, y + dy)
            if (0 <= n[0] < w and 0 <= n[1] < h
                    and g[n[1]][n[0]] != WALL and n not in seen):
                seen.add(n)
                q.append(n)
    return seen


def generate(width, height, rooms, objectives, seed):
    rnd = random.Random(seed)
    g = [[WALL] * width for _ in range(height)]

    # Rooms of mixed size. A few big halls give the open fights somewhere to
    # happen, and the small ones give the routing something to route around.
    placed = []
    for _ in range(rooms * 4):
        if len(placed) >= rooms:
            break
        big = rnd.random() < 0.25
        w = rnd.randint(10, 20) if big else rnd.randint(4, 9)
        h = rnd.randint(7, 14) if big else rnd.randint(4, 8)
        x = rnd.randint(1, max(1, width - w - 2))
        y = rnd.randint(1, max(1, height - h - 2))

        if any(x < px + pw + 2 and px < x + w + 2 and
               y < py + ph + 2 and py < y + h + 2
               for px, py, pw, ph in placed):
            continue

        carve_room(g, x, y, w, h)
        placed.append((x, y, w, h))

    # Join each room to the next, then add a few extra links so the map has
    # loops. A tree of rooms makes every route forced, which is the opposite of
    # a game about choosing routes.
    centres = [(x + w // 2, y + h // 2) for x, y, w, h in placed]
    for i in range(1, len(centres)):
        a, b = centres[i - 1], centres[i]
        carve_corridor(g, a[0], a[1], b[0], b[1], width=rnd.choice([1, 2]))
    for _ in range(len(centres) // 2):
        a, b = rnd.sample(centres, 2)
        carve_corridor(g, a[0], a[1], b[0], b[1], width=rnd.choice([1, 2]))

    # Pillars in the big halls, so open space is not featureless.
    for x, y, w, h in placed:
        if w < 10 or h < 7:
            continue
        for _ in range(rnd.randint(2, 5)):
            px = rnd.randint(x + 2, x + w - 3)
            py = rnd.randint(y + 2, y + h - 3)
            g[py][px] = WALL
            if rnd.random() < 0.5 and px + 1 < x + w - 1:
                g[py][px + 1] = WALL

    # Wall the border, whatever the rooms did.
    for i in range(width):
        g[0][i] = g[height - 1][i] = WALL
    for j in range(height):
        g[j][0] = g[j][width - 1] = WALL

    # Spawn in the most central room, so nobody starts with a free head start
    # on one side of the map.
    cx, cy = width // 2, height // 2
    centres.sort(key=lambda c: abs(c[0] - cx) + abs(c[1] - cy))
    spawn = centres[0]

    # Keep only what a player can actually walk to from the spawn.
    live = reachable(g, spawn)
    for y in range(height):
        for x in range(width):
            if g[y][x] != WALL and (x, y) not in live:
                g[y][x] = WALL

    g[spawn[1]][spawn[0]] = SPAWN

    # Pickups spread out, and never on the spawn.
    candidates = [p for p in live if p != spawn]
    rnd.shuffle(candidates)
    chosen = []
    for p in candidates:
        if len(chosen) >= objectives:
            break
        if all(abs(p[0] - q[0]) + abs(p[1] - q[1]) > 6 for q in chosen):
            chosen.append(p)
    for x, y in chosen:
        g[y][x] = OBJ

    return g, spawn, len(chosen)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", required=True)
    ap.add_argument("--width", type=int, default=160)
    ap.add_argument("--height", type=int, default=90)
    ap.add_argument("--rooms", type=int, default=26)
    ap.add_argument("--objectives", type=int, default=45)
    ap.add_argument("--seed", type=int, default=1)
    args = ap.parse_args()

    g, spawn, placed = generate(
        args.width, args.height, args.rooms, args.objectives, args.seed)

    live = reachable(g, spawn)
    floors = sum(1 for r in g for c in r if c != WALL)
    if len(live) != floors:
        raise SystemExit(
            "generator bug: %d floor cells but only %d reachable"
            % (floors, len(live)))

    with open(args.out, "w") as f:
        f.write("\n".join("".join(r) for r in g) + "\n")

    print("%s: %dx%d, %d floor cells, %d pickups, spawn %s"
          % (args.out, args.width, args.height, floors, placed, spawn))


if __name__ == "__main__":
    main()
