"""The camera path shared by compose_cast.py (which renders it) and verify.py
(which checks nothing that matters leaves the frame).

A move is (t, rect, dur): from video time t, ease over dur seconds to frame
rect, grid cells [c0, r0, c1, r1] inclusive, or None for the whole screen.
The view is a 16:9 rect in 4K frame pixels; the grid sits PAD pixels down.
"""
import math

W, H = 3840, 2160
COLS, ROWS = 200, 53
CW, RH = W / COLS, 40.0          # cell size in 4K pixels
PAD = (H - ROWS * RH) / 2         # grid offset from the top
ZMAX = 2.0


def view(rect):
    """Frame pixels (x, y, w) framing grid cells rect, or the whole frame."""
    if rect is None:
        return (0.0, 0.0, float(W))
    c0, r0, c1, r1 = rect
    x0, x1 = c0 * CW, (c1 + 1) * CW
    y0, y1 = PAD + r0 * RH, PAD + (r1 + 1) * RH
    z = min(W / (x1 - x0), H / (y1 - y0), ZMAX)
    w, h = W / z, H / z
    x = min(max((x0 + x1 - w) / 2, 0), W - w)
    y = min(max((y0 + y1 - h) / 2, 0), H - h)
    return (x, y, w)


def ease(p):
    return 0.5 - 0.5 * math.cos(math.pi * min(max(p, 0), 1))


def segments(moves):
    """[(t0, t1, from, to, dur)]: the view eases from -> to over dur from t0
    and the segment ends at t1, sooner when the next move cuts it short."""
    segs, cur, t = [], view(None), 0.0
    moves = sorted((t0, view(r), d) for t0, r, d in moves)
    for i, (at, to, dur) in enumerate(moves):
        if at > t:
            segs.append((t, at, cur, cur, 1))
        end = min(at + dur, moves[i + 1][0]) if i + 1 < len(moves) else at + dur
        if end > at and dur > 0:
            segs.append((at, end, cur, to, dur))
        cur = tuple(a + (b - a) * (ease((end - at) / dur) if dur else 1) for a, b in zip(cur, to))
        t = max(end, at)
    return segs, cur


def at(segs, last, t):
    """The view (x, y, w) at time t."""
    for t0, t1, a, b, dur in segs:
        if t < t1:
            e = ease((t - t0) / dur) if a != b else 0
            return tuple(x + (y - x) * e for x, y in zip(a, b))
    return last


def cells(v):
    """The grid cells (c0, r0, c1, r1), fractional, that view v shows."""
    x, y, w = v
    h = w * H / W
    return (x / CW, (y - PAD) / RH, (x + w) / CW, (y + h - PAD) / RH)


def expr(segs, last, k, scale, fps):
    """zoompan expression for component k (0 x, 1 y, 2 w) of the view, in
    source pixels (frame pixels times scale). ffmpeg's expression parser
    stops at about 100 levels of nesting, so this picks the segment with a
    balanced tree of ifs on time (about log2(segments) deep) rather than a
    chain."""
    def value(seg):
        t0, t1, a, b, dur = seg
        if a[k] == b[k]:
            return f"{a[k] * scale:.2f}"
        return (f"({a[k] * scale:.2f}+{(b[k] - a[k]) * scale:.2f}"
                f"*(0.5-0.5*cos(3.14159265*(T-{t0:.3f})/{dur:.3f})))")

    # each piece holds from its start time until the next piece starts
    pieces = [(t0, value(sg)) for sg in segs for t0 in [sg[0]]]
    pieces.append((segs[-1][1] if segs else 0.0, f"{last[k] * scale:.2f}"))

    def tree(lo, hi):
        if hi - lo == 1:
            return pieces[lo][1]
        mid = (lo + hi) // 2
        return f"if(lt(T,{pieces[mid][0]:.3f}),{tree(lo, mid)},{tree(mid, hi)})"

    return tree(0, len(pieces)).replace("T", f"(in/{fps})")
