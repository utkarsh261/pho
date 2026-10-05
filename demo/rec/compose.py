#!/usr/bin/env python3
"""Turn a VHS frame dump into the final 4K clip: key chips, small labels and
key clicks on top, encoded once.

usage: compose.py <take> <cut.json> <out.mp4> [poster-seconds]

With poster-seconds, the frame at that time (without overlays) becomes frame 0
and is also saved next to the video as .jpg, so thumbnails are never blank.

VHS writes the terminal grid at 2x as PNG pairs (text + cursor) and captures
slower than real time, unevenly. Each PNG's mtime is its capture time, so the
frames are played back on those real timestamps instead of a fixed rate. The
key times come from pho's own input log (script -r), on the same clock.
"""
import glob, json, math, os, random, re, struct, subprocess, sys, wave

take, cut_path, out = sys.argv[1:4]
poster_at = float(sys.argv[4]) if len(sys.argv) > 4 else None
cut = json.load(open(cut_path))
ev = json.load(open(f"{take}.events.json"))
FPS = 30
W, H = 3840, 2160
BG = "0x1e2021"

texts = sorted(glob.glob(f"{take}-frames/frame-text-*.png"))
cursors = sorted(glob.glob(f"{take}-frames/frame-cursor-*.png"))
times = [os.stat(p).st_mtime for p in texts]
t0 = times[0]
SLOW = ev.get("slow", 1)
times = [(t - t0) / SLOW for t in times]
last_dur = 1 / 15 / SLOW
A = times[-1] + last_dur                 # real visible duration
# Real key times: pho runs under `script -r`, which logs every input record
# with a wall-clock timestamp (same clock as the frame mtimes). Event p is the
# index of the keystroke pho received; shell keys before pho starts have none.
def key_times(path):
    out, buf = [], open(path, "rb").read()
    i = 0
    while i + 24 <= len(buf):
        ln, sec, usec, dirn = struct.unpack("<QQII", buf[i:i + 24])
        data = buf[i + 24:i + 24 + ln]
        # skip the terminal's own replies (colour query, cursor position), not keystrokes
        if dirn == ord("i") and not data.startswith(b"\x1b]") and not re.fullmatch(rb"\x1b\[\d+;\d+R", data):
            out.append(sec + usec / 1e6)
        i += 24 + ln
    return out


keys_rec = key_times(f"{take}.keys.rec")
P = ev["events"][-1]["p"]
if len(keys_rec) != P:
    print(f"warning: {len(keys_rec)} key records for {P} expected keystrokes; timing may drift")
c = (A - ev["duration"]) / max(ev["events"][-1].get("n", 1), 1)


def real(t, n=0, p=None):
    if p is not None and p < len(keys_rec):
        return (keys_rec[p] - t0) / SLOW
    return t + c * n


for e in ev["events"]:
    if "dur" in e:
        e["step"] = e["dur"] / len(e["type"])
    e["t"] = real(e["t"], e.get("n", 0), e.get("p"))
D = A


def concat(paths, name):
    lines = ["ffconcat version 1.0"]
    for i, p in enumerate(paths):
        d = (times[i + 1] - times[i]) if i + 1 < len(paths) else last_dur
        lines += [f"file '{os.path.abspath(p)}'", f"duration {d:.5f}"]
    lines.append(f"file '{os.path.abspath(paths[-1])}'")
    open(name, "w").write("\n".join(lines) + "\n")
    return name


# ---- key clicks: soft, low, short; typed characters a touch quieter ----
SR = 48000
buf = [0.0] * int((D + 1) * SR)
rnd = random.Random(7)


def click(t, gain):
    n0 = int(t * SR)
    lp = 0.0
    f = 150 + rnd.uniform(-15, 15)
    for i in range(int(0.045 * SR)):
        s = i / SR
        lp += 0.18 * (rnd.uniform(-1, 1) - lp)          # one-pole low-pass: takes the hiss off
        body = math.sin(2 * math.pi * f * s) * math.exp(-s / 0.012)
        tick = lp * math.exp(-s / 0.004)
        if n0 + i < len(buf):
            buf[n0 + i] += gain * (0.55 * tick + 0.45 * body)


for e in ev["events"]:
    if "key" in e:
        click(e["t"], 0.16)
    elif "type" in e:
        for i in range(len(e["type"])):
            click(e["t"] + i * e["step"], 0.11)
# small room: two quiet early reflections so clicks sit in a space, not on top
d1, d2 = int(0.023 * SR), int(0.041 * SR)
wet = [buf[i] + 0.22 * (buf[i - d1] if i >= d1 else 0) + 0.12 * (buf[i - d2] if i >= d2 else 0) for i in range(len(buf))]
with wave.open(f"{take}.clicks.wav", "w") as w:
    w.setnchannels(1); w.setsampwidth(2); w.setframerate(SR)
    w.writeframes(b"".join(struct.pack("<h", max(-32767, min(32767, int(x * 32767)))) for x in wet))

# ---- video graph ----
inputs = ["-f", "concat", "-safe", "0", "-i", concat(texts, f"{take}.text.ffconcat"),
          "-f", "concat", "-safe", "0", "-i", concat(cursors, f"{take}.cursor.ffconcat"),
          "-i", f"{take}.clicks.wav"]
px, py = (W - 3696) // 2, (H - 2052) // 2
g = ["[0:v]format=rgba[t]", "[1:v]format=rgba[c]", "[t][c]overlay=0:0[tc]",
     f"[tc]pad={W}:{H}:{px}:{py}:color={BG},fps={FPS},format=rgba[b0]"]
idx, last = 3, "b0"


# Overlays only live in the status-bar strip, and each one only for its own
# window: a short clip cropped to the strip, shifted to its start time.
STRIP_Y, STRIP_H = 2020, 140


def layer(png, a, b, fin, fout):
    global idx, last
    d = max(b - a, 0.05)
    inputs.extend(["-loop", "1", "-framerate", str(FPS), "-t", f"{d:.3f}", "-i", png])
    g.append(f"[{idx}:v]crop={W}:{STRIP_H}:0:{STRIP_Y},format=rgba,"
             f"fade=t=in:st=0:d={fin}:alpha=1,fade=t=out:st={max(d - fout, 0):.3f}:d={fout}:alpha=1,"
             f"setpts=PTS+{a:.3f}/TB[l{idx}]")
    g.append(f"[{last}][l{idx}]overlay=0:{STRIP_Y}:eof_action=pass[o{idx}]")
    last = f"o{idx}"; idx += 1


for lab in cut.get("timeline", []):
    layer(f"cards/label-{lab['label']}.png", real(lab["from"], lab["nFrom"], lab.get("pFrom")) + 0.1, min(real(lab["to"], lab["nTo"], lab.get("pTo")) - 0.05, D), 0.3, 0.3)
keys = [e for e in ev["events"] if "key" in e]
for i, e in enumerate(keys):
    end = min(e["t"] + 0.75, keys[i + 1]["t"] if i + 1 < len(keys) else D)
    slug = re.sub(r"[^a-z0-9]+", "_", e["key"].lower())
    layer(f"cards/key-{slug}.png", e["t"], end, 0.06, 0.12)

if poster_at is not None:
    i = min(range(len(times)), key=lambda i: abs(times[i] - poster_at))
    poster = f"{take}.poster.png"
    subprocess.run(["ffmpeg", "-v", "error", "-y", "-i", texts[i], "-vf",
                    f"pad={W}:{H}:{px}:{py}:color={BG}", poster], check=True)
    subprocess.run(["ffmpeg", "-v", "error", "-y", "-i", poster, "-q:v", "2",
                    os.path.splitext(out)[0] + ".jpg"], check=True)
    inputs.extend(["-i", poster])
    g.append(f"[{last}][{idx}:v]overlay=0:0:enable='eq(n,0)'[poster]")
    last = "poster"

cmd = ["ffmpeg", "-v", "error", "-y", *inputs, "-filter_complex", ";".join(g),
       "-map", f"[{last}]", "-map", "2:a", "-t", f"{D:.3f}", "-r", str(FPS),
       "-c:v", "libx264", "-profile:v", "high", "-pix_fmt", "yuv420p", "-crf", "14", "-preset", "slow",
       "-tune", "animation", "-c:a", "aac", "-b:a", "160k", "-movflags", "+faststart", out]
subprocess.run(cmd, check=True)
print(f"{out}: {D:.2f}s real time ({len(texts)} captured frames, {len(keys_rec)} timed keys)")
