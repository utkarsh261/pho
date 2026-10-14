#!/usr/bin/env python3
"""Turn an asciinema take into the final 4K/60 MP4 and a GIF.

usage: compose_cast.py <take.json> <take.cast> <out.mp4> <out.gif>

agg draws the cast (no browser) at a size where the 200x53 grid is exactly
3840x2120. The grid is cut out and padded to 3840x2160 with the colour of its
own bottom-right cell, so the border always matches the theme on screen. Key
chips and labels (chips.swift) sit at the empty right end of pho's status
bar, at the times cast.py sent each key, with soft key clicks under them.
"""
import json, math, os, random, struct, subprocess, sys, wave

take_path, cast, out, gif = sys.argv[1:5]
take = json.load(open(take_path))
keys = json.load(open(cast + ".keys.json"))
HERE = os.path.dirname(os.path.abspath(__file__))
D, FPS = float(take["duration"]), 60
W, H = 3840, 2160
GX, GY, GW, GH = 19, 20, 3840, 2120           # agg's grid inside its frame
ROW = GH // take["rows"]                      # 40px rows
STATUS_Y = (H - GH) // 2 + (take["rows"] - 1) * ROW
work = os.path.splitext(cast)[0]

# ---- render the cast; agg's timeline is the cast's own clock ----
agg_gif = work + ".agg.gif"
subprocess.run(["agg", "-q", "--font-size", "32", "--line-height", "1.25", "--fps-cap", "60",
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
SEQ_CAP = {"down": "↓", "up": "↑", "enter": "Enter", "esc": "Esc", "tab": "Tab",
           "ctrl+p": "Ctrl P", "ctrl+d": "Ctrl D", "ctrl+u": "Ctrl U"}
chips, spans = {}, []
evs = keys["keys"]
for i, e in enumerate(evs):
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
json.dump({"keys": chips, "labels": labels}, open(spec, "w"))
subprocess.run(["swift", os.path.join(HERE, "chips.swift"), cards, spec], check=True)


def png_size(p):
    with open(p, "rb") as f:
        f.seek(16)
        return struct.unpack(">II", f.read(8))


# ---- key clicks ----
SR = 48000
buf = [0.0] * int((D + 1) * SR)
rnd = random.Random(7)


def click(t, gain):
    n0, lp, f = int(t * SR), 0.0, 150 + rnd.uniform(-15, 15)
    for i in range(int(0.045 * SR)):
        s = i / SR
        lp += 0.18 * (rnd.uniform(-1, 1) - lp)
        v = gain * (0.55 * lp * math.exp(-s / 0.004) + 0.45 * math.sin(2 * math.pi * f * s) * math.exp(-s / 0.012))
        if 0 <= n0 + i < len(buf):
            buf[n0 + i] += v


for e in evs:
    if "key" in e:
        click(e["t"], 0.16)
    else:
        typed = [s for s in take["steps"] if s.get("type") == e["text"]][0]
        for i in range(len(e["text"])):
            click(e["t"] + i * typed["speed"], 0.11)
d1, d2 = int(0.023 * SR), int(0.041 * SR)
wet = [buf[i] + 0.22 * (buf[i - d1] if i >= d1 else 0) + 0.12 * (buf[i - d2] if i >= d2 else 0) for i in range(len(buf))]
wav = work + ".clicks.wav"
with wave.open(wav, "w") as w:
    w.setnchannels(1); w.setsampwidth(2); w.setframerate(SR)
    w.writeframes(b"".join(struct.pack("<h", max(-32767, min(32767, int(x * 32767)))) for x in wet))

# ---- video graph ----
inputs = ["-i", agg_gif, "-i", wav]
# agg writes a frame only when the screen changes, so go to a steady frame
# rate before trimming: trimming first would drop the frame on screen at the
# cut and pull the next change back to zero.
g = [f"[0:v]fps={FPS},trim=start={start:.4f},setpts=PTS-STARTPTS,tpad=stop_mode=clone:stop_duration=2,"
     f"format=rgb24,split[a][b]",
     f"[a]crop={GW}:{GH}:{GX}:{GY}[grid]",
     f"[b]crop=4:4:{GX + GW - 8}:{GY + GH - 8},scale={W}:{H}:flags=neighbor[bg]",
     f"[bg][grid]overlay=0:{(H - GH) // 2}[v0]"]
last, idx = "v0", 2
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

subprocess.run(["ffmpeg", "-v", "error", "-y", *inputs, "-filter_complex", ";".join(g),
                "-map", f"[{last}]", "-map", "1:a", "-t", f"{D:.3f}", "-r", str(FPS),
                "-c:v", "libx264", "-profile:v", "high", "-pix_fmt", "yuv420p", "-crf", "12",
                "-preset", "slow", "-tune", "animation", "-c:a", "aac", "-b:a", "160k",
                "-movflags", "+faststart", out], check=True)

# ---- GIF: 1600 wide, 20 fps, one palette per take ----
pal = work + ".palette.png"
vf = "fps=20,scale=1600:-1:flags=lanczos"
subprocess.run(["ffmpeg", "-v", "error", "-y", "-i", out, "-vf", f"{vf},palettegen=stats_mode=diff:max_colors=256",
                pal], check=True)
subprocess.run(["ffmpeg", "-v", "error", "-y", "-i", out, "-i", pal, "-lavfi",
                f"{vf}[x];[x][1:v]paletteuse=dither=sierra2_4a:diff_mode=rectangle", gif], check=True)
print(f"{out}\n{gif}")
