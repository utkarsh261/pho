#!/usr/bin/env python3
"""Record a take with asciinema: run pho on a pty and type the take's steps
at their times. Step times are seconds into the final video; recording starts
`preroll` seconds earlier so pho has loaded by then.

usage: cast.py <take.json> <out.cast>
Writes <out.cast> and <out.cast>.keys.json (when each key was sent, in video time).
"""
import fcntl, json, os, pty, select, struct, sys, termios, time

take_path, out = sys.argv[1:3]
take = json.load(open(take_path))
W = os.environ["DEMO_WORK"]
cols, rows, pre = take["cols"], take["rows"], take["preroll"]

env = dict(os.environ,
           PATH=f"{W}/bin:" + os.environ["PATH"],
           PHO_MOCK_API="http://" + os.environ.get("MOCK_ADDR", "127.0.0.1:8788"),
           XDG_CACHE_HOME=f"{W}/xdg/c", XDG_STATE_HOME=f"{W}/xdg/s",
           XDG_DATA_HOME=f"{W}/xdg/d", XDG_CONFIG_HOME=f"{W}/xdg/cfg",
           TERM="xterm-256color", COLORTERM="truecolor")

SEQ = {"down": "\x1b[B", "up": "\x1b[A", "enter": "\r", "esc": "\x1b", "tab": "\t",
       "ctrl+p": "\x10", "ctrl+d": "\x04", "ctrl+u": "\x15"}

# (time, bytes, event) for every keystroke
sends = []
for s in take["steps"]:
    if "key" in s:
        sends.append((s["at"], SEQ.get(s["key"], s["key"]), {"key": s["key"]}))
    elif "type" in s:
        for i, ch in enumerate(s["type"]):
            sends.append((s["at"] + i * s["speed"], ch, {"char": ch, "text": s["type"]} if i == 0 else None))

pid, fd = pty.fork()
if pid == 0:
    os.chdir(f"{W}/world/repos")
    os.execvpe("asciinema", ["asciinema", "rec", "--overwrite", "-q", "-f", "asciicast-v2", "--window-size",
                             f"{cols}x{rows}", "-c", "pho", out], env)
fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))
t0 = time.time() + pre  # video time zero


# video time pho switched to the alternate screen: the same moment is in the
# cast, which lines up the two clocks (asciinema writes its own queries first)
ANCHOR = b"\x1b[?1049h"
first_out = None


def pump(until):
    global first_out
    while (left := until - time.time()) > 0:
        r, _, _ = select.select([fd], [], [], left)
        if not r:
            continue
        try:
            data = os.read(fd, 65536)
        except OSError:
            return
        if first_out is None and ANCHOR in data:
            first_out = time.time() - t0
        # answer pho's terminal queries the way a real terminal would
        if b"\x1b]11;?" in data:
            os.write(fd, b"\x1b]11;rgb:1e1e/2020/2121\x1b\\")
        if b"\x1b[6n" in data:
            os.write(fd, b"\x1b[1;1R")


log = []
for at, seq, ev in sends:
    pump(t0 + at)
    os.write(fd, seq.encode())
    if ev:
        log.append(dict(ev, t=round(time.time() - t0, 4)))
pump(t0 + take["duration"] + 0.5)
os.kill(pid, 15)
while True:  # drain until asciinema closes the pty, or it blocks on exit
    try:
        if not os.read(fd, 65536):
            break
    except OSError:
        break
os.waitpid(pid, 0)
json.dump({"preroll": pre, "first_out": first_out, "keys": log}, open(out + ".keys.json", "w"), indent=1)
print(f"{out}: {len(log)} key events")
