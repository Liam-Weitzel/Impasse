#!/usr/bin/env python3
"""A reference Impasse bot.

    ./impasse-server --map maps/open.txt
    ssh -p2222 localhost token          # prints your bot token
    python3 examples/bot.py --token <token>

It walks to the nearest pickup and channels it. That is all. It is deliberately
naive so the protocol is easy to read off, not because greedy nearest is a good
strategy. It is not: everyone else is racing you to the same pickups, and
working out which ones are worth going for is the actual game.

The server sends grid data, never a graph. Building connectivity out of the
cells is the bot's job, and the flood fill below is the whole of it.
"""

import argparse
import json
import socket
from collections import deque

WALL = "#"

# The four directions, and the wire names the server uses for them.
DIRECTIONS = [(0, -1, "n"), (1, 0, "e"), (0, 1, "s"), (-1, 0, "w")]


class Connection:
    def __init__(self, address, token):
        if address.startswith("unix:"):
            self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            self.sock.connect(address[len("unix:"):])
        else:
            host, _, port = address.rpartition(":")
            self.sock = socket.create_connection((host or "127.0.0.1", int(port)))
        self.stream = self.sock.makefile("rwb")
        # Every client authenticates before anything else. The token ties the
        # bot to your SSH key, and to the one character that key owns.
        self.send({"type": "auth", "token": token})

    def read(self):
        line = self.stream.readline()
        if not line:
            raise SystemExit("server closed the connection")
        return json.loads(line)

    def send(self, message):
        self.stream.write((json.dumps(message) + "\n").encode())
        self.stream.flush()

    def move(self, direction):
        self.send({"type": "queue", "action": "move", "dir": direction})

    def loot(self):
        self.send({"type": "queue", "action": "loot"})

    def stun(self):
        self.send({"type": "queue", "action": "stun"})


class Maze:
    """The cells, plus the pathfinding the server refuses to do for you."""

    def __init__(self, rows):
        self.rows = rows
        self.width = len(rows[0])
        self.height = len(rows)

    def walkable(self, x, y):
        if not (0 <= x < self.width and 0 <= y < self.height):
            return False
        return self.rows[y][x] != WALL

    def legal(self, x, y, dx, dy):
        """Movement is orthogonal, so only the target cell matters."""
        return self.walkable(x + dx, y + dy)

    def step_towards(self, start, goal):
        """First direction along a shortest path, or None if unreachable.

        Breadth first, because every step costs one tick and movement is
        orthogonal, so distance here is Manhattan.
        """
        if start == goal:
            return None

        came_from = {start: None}
        queue = deque([start])

        while queue:
            current = queue.popleft()
            if current == goal:
                break
            cx, cy = current
            for dx, dy, _ in DIRECTIONS:
                nxt = (cx + dx, cy + dy)
                if nxt in came_from or not self.legal(cx, cy, dx, dy):
                    continue
                came_from[nxt] = current
                queue.append(nxt)

        if goal not in came_from:
            return None

        # Walk the chain back to the cell right after the start.
        node = goal
        while came_from[node] != start:
            node = came_from[node]

        dx = node[0] - start[0]
        dy = node[1] - start[1]
        for ddx, ddy, name in DIRECTIONS:
            if (ddx, ddy) == (dx, dy):
                return name
        return None


def nearest_reachable(maze, me, objectives):
    """Closest pickup by actual walking distance.

    Note this is more than the human client gets. Its arrow points by straight
    line bearing and happily points through a wall. Doing better is exactly the
    advantage a bot is supposed to have.
    """
    best, best_steps = None, None
    for goal in objectives:
        steps = path_length(maze, me, goal)
        if steps is not None and (best_steps is None or steps < best_steps):
            best, best_steps = goal, steps
    return best


def path_length(maze, start, goal):
    if start == goal:
        return 0
    seen = {start}
    queue = deque([(start, 0)])
    while queue:
        (cx, cy), dist = queue.popleft()
        for dx, dy, _ in DIRECTIONS:
            nxt = (cx + dx, cy + dy)
            if nxt in seen or not maze.legal(cx, cy, dx, dy):
                continue
            if nxt == goal:
                return dist + 1
            seen.add(nxt)
            queue.append((nxt, dist + 1))
    return None


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--address", default="127.0.0.1:2223",
                        help="server address, or unix:/path/to/impasse.sock")
    parser.add_argument("--token", required=True,
                        help="bot token, from `ssh -p2222 <host> token`")
    args = parser.parse_args()

    con = Connection(args.address, args.token)

    welcome = con.read()
    if welcome.get("type") == "error":
        raise SystemExit("rejected: %s" % welcome.get("message"))
    if welcome.get("type") != "welcome":
        raise SystemExit("expected a welcome, got %r" % welcome.get("type"))

    me_id = welcome["id"]
    maze = Maze(welcome["map"])
    print("bot %d joined, %dx%d, loot takes %d ticks"
          % (me_id, maze.width, maze.height, welcome["loot_ticks"]))

    while True:
        state = con.read()
        if state.get("type") != "state":
            continue

        me = next((p for p in state["players"] if p["id"] == me_id), None)
        if me is None:
            raise SystemExit("we are no longer in the world")

        # Nothing to do while stunned, the server ignores us anyway.
        if me["stunned"]:
            continue

        here = (me["x"], me["y"])
        objectives = [(o["x"], o["y"]) for o in state["objectives"]]
        if not objectives:
            print("nothing left to collect")
            continue

        goal = nearest_reachable(maze, here, objectives)
        if goal is None:
            continue

        if goal == here:
            # Standing on it. Channel, and keep channelling: a loot is not
            # consumed by the tick the way a move is.
            if not me["channel"]:
                con.loot()
            continue

        direction = maze.step_towards(here, goal)
        if direction:
            con.move(direction)


if __name__ == "__main__":
    main()
