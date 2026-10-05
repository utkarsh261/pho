#!/usr/bin/env python3
"""Contact sheet of a recorded take, for checking a stretch of footage
without encoding the video.

usage: sheet.py <take> <from-s> <to-s> <step-s> <out.png>   (run in $DEMO_WORK/rec)
"""
import glob, json, os, subprocess, sys

take, a, b, step, out = sys.argv[1], float(sys.argv[2]), float(sys.argv[3]), float(sys.argv[4]), sys.argv[5]
slow = json.load(open(f"{take}.events.json")).get("slow", 1)
fs = sorted(glob.glob(f"{take}-frames/frame-text-*.png"))
t0 = os.stat(fs[0]).st_mtime
ts = [(os.stat(f).st_mtime - t0) / slow for f in fs]

picks, t = [], a
while t <= b:
    picks.append(fs[min(range(len(ts)), key=lambda i: abs(ts[i] - t))])
    t += step

n, cols = len(picks), 3
args = [x for f in picks for x in ("-i", f)]
filt = "".join(f"[{i}:v]scale=1280:-1[s{i}];" for i in range(n))
filt += "".join(f"[s{i}]" for i in range(n)) + f"xstack=inputs={n}:layout="
filt += "|".join(f"{(i % cols) * 1280}_{(i // cols) * 711}" for i in range(n)) + ":fill=black"
subprocess.run(["ffmpeg", "-v", "error", "-y", *args, "-filter_complex", filt, out], check=True)
