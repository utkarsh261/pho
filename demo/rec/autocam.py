#!/usr/bin/env python3
"""Point the camera at whatever each action changes on screen.

Replays the cast in a terminal emulator (pyte) and splits the keys cast.py
sent into actions: keys less than GROUP seconds apart (j j j) are one action,
and typed text is always an action of its own. For a key action the camera
frames the cells it changed, from just before it until the screen settles; a
change over most of the screen pulls back to the whole screen. For typed text
it frames the text where it ends up on screen. The camera stays put when the
change is already well framed, so it doesn't twitch on every key.

Each "section" step cuts back to the whole screen (under its title card).
A step can script its own shots instead: "cam": "full", "cam": "hold" (no
move), "cam": [c0, r0, c1, r1] (grid cells, inclusive), or a shot list
"cam": [[delay, rect | "full"], ...] with delays in seconds after the step.

usage: autocam.py <take.cast>      (reads <cast>.keys.json)
Writes <cast>.cam.json: {"moves": [[t, rect | null, dur], ...]}
"""
import json, os, sys
import pyte

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import camera

GROUP = 0.45      # keys closer than this are one action
SETTLE = 1.1      # how long after an action its screen is read, at most
MOVE = 0.55       # camera move duration
LAG = 0.08        # camera starts moving this long after the action
HOLD = 0.9        # a shot lasts at least this long unless the change is out of it
MARGIN = (6, 3)   # context around the changes, in cols, rows
FULL = 0.5        # changes over this share of the screen pull back to it
DEBUG = bool(os.environ.get("AUTOCAM_DEBUG"))


def replay(cast):
    """Video-time screen snapshots [(t, rows of cell tuples)] and the keys log."""
    keys = json.load(open(cast + ".keys.json"))
    lines = open(cast).read().splitlines()
    hdr = json.loads(lines[0])
    evs = [json.loads(l) for l in lines[1:]]
    start = next(e[0] for e in evs if "\x1b[?1049h" in e[2]) - keys["first_out"]
    scr = pyte.Screen(hdr["width"], hdr["height"])
    stream = pyte.Stream(scr)
    snaps = []
    for e in evs:
        if e[1] != "o":
            continue
        stream.feed(e[2])
        snaps.append((e[0] - start, [tuple((c.data, c.fg, c.bg, c.reverse, c.bold)
                                           for c in (scr.buffer[r][x] for x in range(scr.columns)))
                                     for r in range(scr.lines)]))
    return snaps, keys


def screen_at(snaps, t):
    s = snaps[0][1]
    for ts, c in snaps:
        if ts > t:
            break
        s = c
    return s


def text_rows(screen):
    return ["".join(c[0] for c in row) for row in screen]


def find_text(screen, text, before=None):
    """Bounding cells of where typed text sits on screen, or None. It may wrap,
    or scroll in a one-line field, so this looks for its head and its tail,
    preferring places that hold something else in the before screen."""
    rows, old = text_rows(screen), text_rows(before) if before else None
    hits = []
    for probe in (text[:12], text[-12:]):
        found = [(r, c) for r, line in enumerate(rows) for c in [line.find(probe)] if c >= 0]
        fresh = [(r, c) for r, c in found if not old or old[r][c:c + len(probe)] != probe]
        if fresh or found:
            r, c = (fresh or found)[-1]
            hits.append((c, r, c + len(probe) - 1, r))
    if not hits:
        return None
    c0, r0 = min(h[0] for h in hits), min(h[1] for h in hits)
    c1, r1 = max(h[2] for h in hits), max(h[3] for h in hits)
    if len(hits) == 1 and len(text) > 12:
        # only the tail is on screen: the field scrolled, so it fills its row
        # leftwards, as far as the field's border or prompt
        line = rows[r0]
        while c0 > 0 and c1 - c0 < len(text) and line[c0 - 1] not in "│┃›❯>":
            c0 -= 1
    return (c0, r0, c1, r1)


def frame(box):
    """The rect to frame box with context, or None when that is barely a zoom."""
    c0, r0, c1, r1 = box
    rect = [max(c0 - MARGIN[0], 0), max(r0 - MARGIN[1], 0),
            min(c1 + MARGIN[0], camera.COLS - 1), min(r1 + MARGIN[1], camera.ROWS - 1)]
    return rect if camera.W / camera.view(rect)[2] >= 1.15 else None


def shown(box, rect, slack=0.5):
    x0, y0, x1, y1 = camera.cells(camera.view(rect))
    c0, r0, c1, r1 = box
    return c0 >= x0 - slack and c1 + 1 <= x1 + slack and r0 >= y0 - slack and r1 + 1 <= y1 + slack


def zoom(rect):
    return camera.W / camera.view(rect)[2]


def main(cast):
    snaps, keys = replay(cast)
    rows, cols = len(snaps[0][1]), len(snaps[0][1][0])
    # cast.py logs one key event per key or typed step, in order, so pair them
    # up to give each event its step's scripted shots (times drift a little)
    sent = [s for s in keys["steps"] if "key" in s or "type" in s]
    assert len(sent) == len(keys["keys"]), "keys log doesn't match the take's steps"
    for s, e in zip(sent, keys["keys"]):
        if "cam" in s:
            e["cam"] = s["cam"]

    actions = []
    for e in keys["keys"]:
        last = actions[-1] if actions else None
        if last and "key" in e and "key" in last[-1] and e["t"] - last[-1]["t"] < GROUP:
            last.append(e)
        else:
            actions.append([e])

    moves, cur = [], None

    def move(t, rect):
        nonlocal cur
        if rect != cur:
            moves.append([round(t, 3), rect, MOVE])
            cur = rect

    def held(t):
        return bool(moves) and t - moves[-1][0] < HOLD

    # each section opens on the whole screen, cut under its title card
    cuts = [round(s["at"], 3) for s in keys["steps"] if "section" in s]

    for i, a in enumerate(actions):
        t0, t1 = a[0]["t"], a[-1]["t"]
        while cuts and cuts[0] <= t0:
            c = cuts.pop(0)
            if cur is not None:
                moves.append([c, None, 0])
                cur = None
        nxt = actions[i + 1][0]["t"] if i + 1 < len(actions) else keys["duration"]
        # several scripted keys in one action: the last real shot wins over holds
        cams = [e["cam"] for e in a if "cam" in e]
        cam = next((c for c in reversed(cams) if c != "hold"), cams[0] if cams else None)
        if cam == "hold":
            continue
        if cam is not None:
            shots = cam if isinstance(cam, list) and cam and isinstance(cam[0], list) else [[0, cam]]
            for delay, rect in shots:
                move(t0 + LAG + delay, None if rect == "full" else rect)
            continue
        if "text" in a[0]:
            text = a[0]["text"]
            done = t0 + len(text) * 0.06 + 0.3
            box = find_text(screen_at(snaps, min(done, nxt - 0.02)), text, screen_at(snaps, t0 - 0.01))
            if box is None:
                continue
        else:
            settle = min(t1 + SETTLE, nxt - 0.02)
            before, after = screen_at(snaps, t0 - 0.01), screen_at(snaps, settle)
            pts = [(x, r) for r in range(rows - 1) for x in range(cols) if before[r][x] != after[r][x]]
            if not pts:  # only the status bar changed (a notice, a prompt)
                pts = [(x, rows - 1) for x in range(cols) if before[rows - 1][x] != after[rows - 1][x]]
            if not pts:
                continue
            xs, ys = [p[0] for p in pts], [p[1] for p in pts]
            box = (min(xs), min(ys), max(xs), max(ys))
            area = (box[2] - box[0] + 1) * (box[3] - box[1] + 1)
            if len(pts) > FULL * cols * rows * 0.5 or area > FULL * cols * rows:
                if DEBUG:
                    print(f"{t0:7.2f} {a[0].get('key', '')[:14]:14s} full  n={len(pts)} box={box}")
                move(t0 + LAG, None)
                continue
        target = frame(box)
        if DEBUG:
            print(f"{t0:7.2f} {a[0].get('key', a[0].get('text', ''))[:14]:14s} box={box} -> {target} cur={cur}")
        # already framed well enough, or the last shot only just started: stay put
        if shown(box, cur) and (held(t0) or cur is not None and zoom(target) <= zoom(cur) * 1.35):
            continue
        # typed text lands where it was found, so get there before it starts
        prev_t = actions[i - 1][-1]["t"] if i else 0
        move(max(prev_t + 0.15, t0 - 0.45) if "text" in a[0] else t0 + LAG, target)

    json.dump({"moves": moves}, open(cast + ".cam.json", "w"))
    print(f"{cast}.cam.json: {len(moves)} camera moves")


if __name__ == "__main__":
    main(sys.argv[1])
