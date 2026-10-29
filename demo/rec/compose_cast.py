#!/usr/bin/env python3
"""Turn an asciinema take into the final 4K/60 MP4 and a GIF.

usage: compose_cast.py <take.json> <take.cast> <out.mp4> [out.gif]

agg draws the cast (no browser) at a size where the 200x53 grid is exactly
3840x2120. The grid is cut out and padded to 3840x2160 with the colour of its
own bottom-right cell, so the border always matches the theme on screen. Key
chips and labels (chips.swift) sit at the empty right end of pho's status
bar, at the times cast.py sent each key. There is no audio track.

The camera (camera.py) follows <cast>.cam.json when autocam.py has written
one, zooming in up to 2x; otherwise a step can carry "zoom": [c0, r0, c1, r1],
the grid cells (inclusive) to frame from that step on, or null for the whole
screen, eased over "zt" seconds (default 0.8) from "zd" seconds after the step,
zooming in up to 1.5x. Takes with a camera draw the cast at the largest zoom
times the size, so text is still drawn 1:1 at full zoom, and give the labels a
backing pill since they can then sit over pho's content.

Takes with "open" and "section" steps get title cards (titlecards.py): the
opening card, a card at each section, and the closing card, cross-faded in.
"""
import json, os, struct, subprocess, sys

take_path, cast, out = sys.argv[1:4]
gif = sys.argv[4] if len(sys.argv) > 4 else None
take = json.load(open(take_path))
keys = json.load(open(cast + ".keys.json"))
# cast.py fills in step times the take left to follow on from the previous step
take["steps"], take["duration"] = keys.get("steps", take["steps"]), keys.get("duration", take.get("duration"))
HERE = os.path.dirname(os.path.abspath(__file__))
D, FPS = float(take["duration"]), 60
W, H = 3840, 2160
GW, GH = 3840, 2120                           # the 200x53 grid
ROW = GH // take["rows"]                      # 40px rows
STATUS_Y = (H - GH) // 2 + (take["rows"] - 1) * ROW
work = os.path.splitext(cast)[0]
sys.path.insert(0, HERE)
import camera
if os.path.exists(cast + ".cam.json"):
    moves = json.load(open(cast + ".cam.json"))["moves"]
else:
    camera.ZMAX = 1.5
    moves = [(s["at"] + s.get("zd", 0), s["zoom"], s.get("zt", 0.8)) for s in take["steps"] if "zoom" in s]
zooms = bool(moves)
PREVIEW = bool(os.environ.get("PREVIEW"))     # quick look: 1x source, fast encode
SC = camera.ZMAX if zooms and not PREVIEW else 1  # source scale: the grid is drawn SC times 4K
GX, GY = round(19 * SC), round(20 * SC)       # agg's grid inside its frame

# ---- render the cast; agg's timeline is the cast's own clock ----
agg_gif = work + ".agg.gif"
subprocess.run(["agg", "-q", "--font-size", str(round(32 * SC)), "--line-height", "1.25", "--fps-cap", "60",
                "--idle-time-limit", "1000", "--last-frame-duration", "2", "--text-font-family", "JetBrains Mono",
                "--theme", "1e2021,e6edf3,1e2021,ff7b72,3fb950,d29922,58a6ff,bc8cff,39c5cf,b1bac4,"
                           "6e7681,ffa198,56d364,e3b341,79c0ff,d2a8ff,56d4dd,ffffff",
                cast, agg_gif], check=True)

# cast time of video zero: pho entering the alternate screen, minus how far
# into the video cast.py saw it
# asciicast v2: absolute event times (v3's per-event deltas are rounded to
# the millisecond and drift by about a second over a take)
lines = open(cast).read().splitlines()
assert json.loads(lines[0])["version"] == 2, "record with -f asciicast-v2"
first_cast_t = next(e[0] for e in map(json.loads, lines[1:]) if "\x1b[?1049h" in e[2])
start = first_cast_t - keys["first_out"]
assert start >= 0, f"preroll too short: video zero is {start:.3f}s into the cast"

# ---- overlays ----
SEQ_CAP = {"down": "↓", "up": "↑", "enter": "Enter", "esc": "Esc", "tab": "Tab", "shift+tab": "Shift Tab",
           "space": "Space", "ctrl+p": "Ctrl P", "ctrl+d": "Ctrl D", "ctrl+u": "Ctrl U", "ctrl+s": "Ctrl S"}
chips, spans = {}, []
evs = keys["keys"]
for i, e in enumerate(evs):
    if e.get("cap") is False:
        continue
    nxt = evs[i + 1]["t"] if i + 1 < len(evs) else D
    if "key" in e:
        text = SEQ_CAP.get(e["key"], e["key"])
        end = min(e["t"] + 0.8, nxt)
    else:  # typed word: one chip for the whole word
        text = e["text"]
        typed = [s for s in take["steps"] if s.get("type") == text][0]
        end = min(e["t"] + len(text) * typed["speed"] + 0.6, D)
        nxt_key = [k for k in evs[i + 1:] if "key" in k or "text" in k]
        end = min(end, nxt_key[0]["t"]) if nxt_key else end
    slug = "k%02d" % len(chips)
    chips[slug] = text
    spans.append(("key-" + slug, e["t"], end, 0.05, 0.12))
labels, steps = {}, [s for s in take["steps"] if "label" in s]
for i, s in enumerate(steps):
    slug = "l%02d" % i
    labels[slug] = s["label"]
    nxt = steps[i + 1]["at"] if i + 1 < len(steps) else D
    spans.append(("label-" + slug, s["at"], nxt - 0.1, 0.3, 0.3))

cards = work + "-cards"
spec = work + ".chips.json"
json.dump({"keys": chips, "labels": labels, "opts": {"pill": "1"} if zooms else {}}, open(spec, "w"))
subprocess.run(["swift", os.path.join(HERE, "chips.swift"), cards, spec], check=True)


def png_size(p):
    with open(p, "rb") as f:
        f.seek(16)
        return struct.unpack(">II", f.read(8))


# ---- camera ----
SW, SH = round(W * SC), round(H * SC)
segs, last_view = camera.segments(moves)


# ---- video graph ----
inputs = ["-i", agg_gif]
# agg writes a frame only when the screen changes, so go to a steady frame
# rate before trimming: trimming first would drop the frame on screen at the
# cut and pull the next change back to zero.
g = [f"[0:v]fps={FPS},trim=start={start:.4f},setpts=PTS-STARTPTS,tpad=stop_mode=clone:stop_duration=2,"
     f"format=rgb24,split[a][b]",
     f"[a]crop={round(GW * SC)}:{round(GH * SC)}:{GX}:{GY}[grid]",
     f"[b]crop=4:4:{GX + round(GW * SC) - 8}:{GY + round(GH * SC) - 8},scale={SW}:{SH}:flags=neighbor[bg]",
     f"[bg][grid]overlay=0:{round((H - GH) / 2 * SC)}" + ("[v0]" if not zooms else "[full]")]
if zooms:
    ex = [camera.expr(segs, last_view, k, SC, FPS) for k in range(3)]
    g.append(f"[full]zoompan=z='{SW}/({ex[2]})':x='{ex[0]}':y='{ex[1]}':d=1:s={W}x{H}:fps={FPS}[v0]")
last, idx = "v0", 1
CHIP_RIGHT, LABEL_RIGHT = 36, 300
for png, a, b, fin, fout in spans:
    path = os.path.join(cards, png + ".png")
    pw, ph = png_size(path)
    right = CHIP_RIGHT if png.startswith("key-") else LABEL_RIGHT
    x, y = W - right - pw, STATUS_Y + (ROW - ph) // 2
    d = max(b - a, 0.05)
    inputs += ["-loop", "1", "-framerate", str(FPS), "-t", f"{d:.3f}", "-i", path]
    g.append(f"[{idx}:v]format=rgba,fade=t=in:st=0:d={fin}:alpha=1,"
             f"fade=t=out:st={max(d - fout, 0):.3f}:d={fout}:alpha=1,setpts=PTS+{a:.3f}/TB[o{idx}]")
    g.append(f"[{last}][o{idx}]overlay={x}:{y}:eof_action=pass[v{idx}]")
    last, idx = f"v{idx}", idx + 1

# ---- title cards: cut the take at each section and cross-fade the cards in ----
total = D
if take.get("open"):
    import titlecards
    built = titlecards.build(take, work + "-titles")
    XF = 0.4
    pieces, prev = [built["open"]], 0.0
    for c, card in zip([s["at"] for s in take["steps"] if "section" in s], built["sections"]):
        if c > prev + 0.05:
            pieces.append((prev, c))
        pieces.append(card)
        prev = c
    pieces += [(prev, D), built["end"]]
    mains = [p for p in pieces if not isinstance(p[0], str)]
    g.append(f"[{last}]split={len(mains)}" + "".join(f"[m{i}]" for i in range(len(mains))))
    norm = f"format=yuv420p,setsar=1,settb=AVTB,fps={FPS}"
    labels, durs, m = [], [], 0
    for k, p in enumerate(pieces):
        if isinstance(p[0], str):
            inputs += ["-i", p[0]]
            g.append(f"[{idx}:v]{norm}[p{k}]")
            idx += 1
            durs.append(p[1])
        else:
            g.append(f"[m{m}]trim=start={p[0]:.4f}:end={p[1]:.4f},setpts=PTS-STARTPTS,{norm}[p{k}]")
            m += 1
            durs.append(p[1] - p[0])
        labels.append(f"p{k}")
    cur, total = labels[0], durs[0]
    for k in range(1, len(labels)):
        g.append(f"[{cur}][{labels[k]}]xfade=transition=fade:duration={XF}:offset={total - XF:.4f}[x{k}]")
        cur, total = f"x{k}", total + durs[k] - XF
    last = cur
    print(f"title cards in: {total:.1f}s")

subprocess.run(["ffmpeg", "-v", "error", "-y", *inputs, "-filter_complex", ";".join(g),
                "-map", f"[{last}]", "-t", f"{total:.3f}", "-r", str(FPS),
                "-c:v", "libx264", "-profile:v", "high", "-pix_fmt", "yuv420p", "-crf", "12",
                # level 5.2 caps 4K at 4 reference frames; with more, Apple's
                # decoder (QuickTime, Safari) shows stale blocks after a seek
                *(["-preset", "ultrafast"] if PREVIEW else ["-preset", "slow", "-tune", "animation"]),
                "-refs", "4", "-level:v", "5.2",
                "-movflags", "+faststart", out], check=True)

if not gif:
    print(out)
    sys.exit()

# ---- GIF: 1600 wide, 20 fps, one palette per take ----
pal = work + ".palette.png"
vf = "fps=20,scale=1600:-1:flags=lanczos"
subprocess.run(["ffmpeg", "-v", "error", "-y", "-i", out, "-vf", f"{vf},palettegen=stats_mode=diff:max_colors=256",
                pal], check=True)
subprocess.run(["ffmpeg", "-v", "error", "-y", "-i", out, "-i", pal, "-lavfi",
                f"{vf}[x];[x][1:v]paletteuse=dither=sierra2_4a:diff_mode=rectangle", gif], check=True)
print(f"{out}\n{gif}")
