"""Title cards: the opening card, one per section, and the closing card,
rendered to short 4K/60 clips that compose_cast.py cuts into the take.

The layers come from titlecard.swift (CoreText, with pho's block logo drawn
cell by cell). This module animates them with ffmpeg: the active tab's
highlight slides over from the previous section's tab, the title types in
behind a block cursor, and the subtitle and key hints fade up.
"""
import json, os, struct, subprocess

W, H, FPS = 3840, 2160, 60
HERE = os.path.dirname(os.path.abspath(__file__))
CPS = 0.045                       # seconds per typed character

# pho's palettes (internal/ui/theme): the colours each card is drawn in
THEMES = {
    "pho-default": {"bg": "#1E2021", "border": "#30363D", "accent": "#7C3AED", "accentTx": "#A78BFA",
                    "text": "#E6EDF3", "bright": "#FFFFFF", "dim": "#9CA3AF", "faint": "#4B5563",
                    "muted": "#6B7280", "hintKey": "#C9D1D9", "onAccent": "#FFFFFF"},
    "tokyo-night": {"bg": "#1A1B26", "border": "#3B4261", "accent": "#7AA2F7", "accentTx": "#BB9AF7",
                    "text": "#C0CAF5", "bright": "#E4E9FF", "dim": "#A9B1D6", "faint": "#545C7E",
                    "muted": "#737AA2", "hintKey": "#C0CAF5", "onAccent": "#1A1B26"},
}


def png_size(p):
    with open(p, "rb") as f:
        f.seek(16)
        return struct.unpack(">II", f.read(8))


def layers(cards, work):
    spec = os.path.join(work, "spec.json")
    json.dump({"cards": cards}, open(spec, "w"))
    subprocess.run(["swift", os.path.join(HERE, "titlecard.swift"), work, spec], check=True)
    return json.load(open(os.path.join(work, "meta.json")))


def encode(inputs, graph, last, dur, out):
    args = []
    for p in inputs:
        args += ["-loop", "1", "-framerate", str(FPS), "-t", f"{dur:.3f}", "-i", p]
    subprocess.run(["ffmpeg", "-v", "error", "-y", *args, "-filter_complex", ";".join(graph), "-map", f"[{last}]",
                    "-t", f"{dur:.3f}", "-r", str(FPS), "-c:v", "libx264", "-crf", "8", "-preset", "veryfast",
                    "-pix_fmt", "yuv420p", out], check=True)


def typed(meta, ts, cursor_from):
    """Graph pieces that type the title in from ts: a bg mask that uncovers it
    one character at a time, and a block cursor that leads, then blinks."""
    n, cw, tx = meta["n"], meta["char_w"], meta["title_x"]
    te = ts + n * CPS
    k = f"{cw:.3f}*min({n},max(0,floor((t-{ts:.3f})/{CPS})+1))"
    mask = f"overlay=x='{k}':y=0:eval=frame:format=rgb"
    cur = (f"overlay=x='{tx}+{k}':y={meta['cursor_y']}:eval=frame:"
           f"enable='gte(t,{cursor_from:.3f})*(lt(t,{te:.3f})+lt(mod(t-{te:.3f},0.9),0.5))'")
    return mask, cur, te


def build(take, work):
    """Render every card. Returns {"open": (path, dur), "end": (path, dur),
    "sections": [(path, dur), ...]} in the order the sections appear."""
    os.makedirs(work, exist_ok=True)
    secs = [s["section"] for s in take["steps"] if "section" in s]
    tabs = [s["tab"] for s in secs]
    cards = []
    for i, s in enumerate(secs):
        cards.append({"name": f"s{i}", "kind": "section", "theme": THEMES[s["theme"]], "title": s["title"],
                      "sub": s["sub"], "hints": s["hints"], "tabs": tabs, "active": i, "index": i + 1,
                      "count": len(secs)})
    o, e = take["open"], take["end"]
    cards.append({"name": "open", "kind": "open", "theme": THEMES[o["theme"]], "tagline": o["tagline"],
                  "version": o["version"]})
    cards.append({"name": "end", "kind": "end", "theme": THEMES[e["theme"]], "cmd": e["cmd"], "url": e["url"]})
    meta = layers(cards, work)
    L = lambda name: os.path.join(work, name + ".png")
    out = {"sections": []}

    for i, s in enumerate(secs):
        m, n = meta[f"s{i}"], f"s{i}"
        x0, x1 = m["slots"][max(i - 1, 0)], m["slots"][i]
        pill = (f"overlay=x='{x0}+({x1 - x0})*(0.5-0.5*cos(3.14159265*min(max((t-0.25)/0.5,0),1)))'"
                f":y={m['pill_y']}:eval=frame")
        mask, cur, te = typed(m, 0.55, 0.3)
        dur = round(te + 1.5, 2)
        g = ["[0:v][2:v]" + pill + "[p]", "[p][1:v]overlay=0:0[a]",
             "[7:v]format=rgba,fade=t=in:st=0.62:d=0.2:alpha=1[pt]", f"[a][pt]overlay={x1}:{m['pill_y']}[b]",
             f"[3:v][4:v]{mask}[tt]",
             f"[b][tt]overlay={m['title_x']}:{m['title_y']}[d]", f"[d][5:v]{cur}[e]",
             f"[6:v]format=rgba,fade=t=in:st={te + 0.05:.3f}:d=0.45:alpha=1[s]", "[e][s]overlay=0:0[v]"]
        path = os.path.join(work, f"{n}.mp4")
        encode([L(f"{n}-bg"), L(f"{n}-base"), L(f"{n}-pill"), L(f"{n}-title"), L(f"{n}-mask"), L(f"{n}-cursor"),
                L(f"{n}-sub"), L(f"{n}-pilltext")], g, "v", dur, path)
        out["sections"].append((path, dur))

    # opening: the logo rises in, then the tagline and version
    lp = L("open-logo")
    lw, lh = png_size(lp)
    lx, ly = (W - lw) // 2, 880 - lh // 2
    dur = 3.0
    g = [f"[1:v]format=rgba,fade=t=in:st=0.15:d=0.6:alpha=1[l]",
         f"[0:v][l]overlay=x={lx}:y='{ly}+46*(1-(0.5-0.5*cos(3.14159265*min(max((t-0.15)/0.8,0),1))))':eval=frame[a]",
         "[2:v]format=rgba,fade=t=in:st=0.7:d=0.5:alpha=1[t]", "[a][t]overlay=0:0[b]",
         "[3:v]format=rgba,fade=t=in:st=1.1:d=0.45:alpha=1[r]", "[b][r]overlay=0:0[v]"]
    path = os.path.join(work, "open.mp4")
    encode([L("open-bg"), lp, L("open-tag"), L("open-ver")], g, "v", dur, path)
    out["open"] = (path, dur)

    # closing: the logo, then the install command typed at a prompt
    lp = L("end-logo")
    lw, lh = png_size(lp)
    m = meta["end"]
    mask, cur, te = typed(m, 0.75, 0.5)
    dur = round(te + 2.4, 2)
    g = ["[1:v]format=rgba,fade=t=in:st=0:d=0.5:alpha=1[l]", f"[0:v][l]overlay={(W - lw) // 2}:{780 - lh // 2}[a]",
         "[2:v]format=rgba,fade=t=in:st=0.35:d=0.35:alpha=1[p]", "[a][p]overlay=0:0[b]",
         f"[3:v][4:v]{mask}[tt]", f"[b][tt]overlay={m['title_x']}:{m['title_y']}[d]", f"[d][5:v]{cur}[e]",
         f"[6:v]format=rgba,fade=t=in:st={te + 0.2:.3f}:d=0.5:alpha=1[s]", "[e][s]overlay=0:0[v]"]
    path = os.path.join(work, "end.mp4")
    encode([L("end-bg"), lp, L("end-prompt"), L("end-title"), L("end-mask"), L("end-cursor"), L("end-sub")],
           g, "v", dur, path)
    out["end"] = (path, dur)
    return out


if __name__ == "__main__":
    import sys
    print(json.dumps(build(json.load(open(sys.argv[1])), sys.argv[2]), indent=1))
