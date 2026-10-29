#!/usr/bin/env python3
"""Check the camera never crops what the viewer should be watching.

Replays the cast against the camera path in <cast>.cam.json and reports:
- typed text that is not fully in frame while it is being typed;
- a change to a part of the screen (a popup, a notice, a moved selection)
  that is still out of frame LAG seconds after it appears.
Changes to most of the screen at once are not checked: those are scene changes
the camera answers by pulling back. The status bar is checked only when the
camera is framing the bottom rows, since it is out of frame by design.

usage: verify.py <take.cast>       exits 1 on any problem
"""
import json, os, sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import camera
from autocam import find_text, replay, screen_at

LAG = 0.75        # the camera may take this long to bring a change in
BIG = 0.3         # changes over this share of the screen are scene changes
BORDER = set("─│╭╮╰╯├┤┬┴┼ ▎")

cast = sys.argv[1]
snaps, keys = replay(cast)
segs, last = camera.segments(json.load(open(cast + ".cam.json"))["moves"])
rows, cols = len(snaps[0][1]), len(snaps[0][1][0])


def in_view(box, t, slack=0.5):
    x0, y0, x1, y1 = camera.cells(camera.at(segs, last, t))
    c0, r0, c1, r1 = box
    return c0 >= x0 - slack and c1 + 1 <= x1 + slack and r0 >= y0 - slack and r1 + 1 <= y1 + slack


problems = []
steps = {s["type"]: s for s in keys["steps"] if "type" in s}
for e in keys["keys"]:
    if "text" not in e:
        continue
    text, speed = e["text"], steps[e["text"]]["speed"]
    nxt = next((k["t"] for k in keys["keys"] if k["t"] > e["t"] and "key" in k), 1e9)
    t = e["t"] + 0.2
    while t < min(e["t"] + len(text) * speed + 0.5, nxt):
        n = max(1, min(len(text), int((t - e["t"]) / speed)))
        box = find_text(screen_at(snaps, t), text[:n], screen_at(snaps, e["t"] - 0.01)) if n >= 3 else None
        if box and not in_view(box, t):
            problems.append((t, f"typing {text[:30]!r} at cells {box} is out of frame"))
            break
        t += 0.1

prev = snaps[0][1]
for t, cur in snaps[1:]:
    # what the characters say, not their colours: focus moving restyles text
    # and borders all over the screen, which is not something to watch
    pts = [(x, r) for r in range(rows) for x in range(cols)
           if cur[r][x][0] != prev[r][x][0] and not {cur[r][x][0], prev[r][x][0]} <= BORDER]
    prev = cur
    if not pts:
        continue
    body = [p for p in pts if p[1] < rows - 1]
    status = [p for p in pts if p[1] == rows - 1]
    for group, what in ((body, "change"), (status, "status bar")):
        if not group:
            continue
        xs, ys = [p[0] for p in group], [p[1] for p in group]
        box = (min(xs), min(ys), max(xs), max(ys))
        if (box[2] - box[0] + 1) * (box[3] - box[1] + 1) > BIG * rows * cols:
            continue
        if what == "status bar":
            _, y0, _, y1 = camera.cells(camera.at(segs, last, t + LAG))
            if y1 < rows - 2:
                continue
        if not in_view(box, t + LAG):
            text = "".join(cur[box[1]][x][0] for x in range(box[0], box[2] + 1)).strip()[:50]
            problems.append((t, f"{what} at cells {box} out of frame: {text!r}"))

# report each problem once per second of video
seen = set()
for t, p in sorted(problems):
    k = (int(t), p.split(" at ")[0])
    if k in seen:
        continue
    seen.add(k)
    print(f"{t:7.2f}  {p}")
print(f"{len(seen)} problem(s)")
sys.exit(1 if seen else 0)
