#!/usr/bin/env python3
"""Render the overlay PNGs at 2x for the 4K frame: key chips and small labels that sit in the empty
right end of pho's status bar, using the headless Chromium already on the machine.

usage: cards.py <out_dir> <cut.json>
cut.json: {"labels": {"id": "text"}, "keys": ["Enter", ...], ...}
"""
import glob, html, json, os, re, subprocess, sys

def find_chrome():
    if os.environ.get("CHROME"):
        return os.environ["CHROME"]
    candidates = ["/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
                  "/Applications/Chromium.app/Contents/MacOS/Chromium"]
    candidates += sorted(glob.glob(os.path.expanduser(
        "~/Library/Caches/ms-playwright/chromium-*/chrome-mac*/*.app/Contents/MacOS/*")), reverse=True)
    for c in candidates:
        if os.access(c, os.X_OK):
            return c
    sys.exit("cards.py: no Chrome/Chromium found; set CHROME=/path/to/chrome")


CHROME = find_chrome()
out, spec = sys.argv[1], json.load(open(sys.argv[2]))
os.makedirs(out, exist_ok=True)

ROW_Y, ROW_H = 1027, 40      # pho's status bar row
CHIP_RIGHT = 60              # chip's right edge, from the frame's right edge
LABEL_RIGHT = 196            # label's right edge (leaves room for the chip)

BASE = """<!doctype html><meta charset=utf-8><style>
html,body{margin:0;width:1920px;height:1080px;overflow:hidden;background:transparent}
body{font-family:'JetBrains Mono',monospace;font-size:18px;-webkit-font-smoothing:antialiased}
.row{position:absolute;top:%dpx;height:%dpx;display:flex;align-items:center}
</style>""" % (ROW_Y, ROW_H)


def shot(name, body):
    path = os.path.abspath(f"{out}/{name}.html")
    open(path, "w").write(BASE + body)
    subprocess.run([CHROME, "--headless=new", "--disable-gpu", "--hide-scrollbars",
                    "--default-background-color=00000000", "--window-size=1920,1080",
                    "--force-device-scale-factor=2", f"--screenshot={out}/{name}.png", f"file://{path}"],
                   check=True, capture_output=True)
    os.remove(path)


def slug(s):
    return re.sub(r"[^a-z0-9]+", "_", s.lower()) or "key"


for k in spec.get("keys", []):
    shot(f"key-{slug(k)}", f"""<div class=row style="right:{CHIP_RIGHT}px"><div style="color:#ECEDEE;
 font-weight:500;background:#2B2D30;border:1px solid #45484D;border-bottom-width:2px;border-radius:6px;
 padding:3px 11px;min-width:14px;text-align:center">{html.escape(k)}</div></div>""")

for lid, text in spec.get("labels", {}).items():
    # commentary is set in the system sans so it never reads as pho's own UI
    shot(f"label-{lid}", f"""<div class=row style="right:{LABEL_RIGHT}px;font-family:-apple-system,'SF Pro Text',system-ui,sans-serif;
 font-size:21px;font-weight:500;letter-spacing:.005em;color:#B8AFEA">{html.escape(text)}</div>""")
print("cards ->", out)
