#!/usr/bin/env python3
"""Record a take with asciinema: run pho on a pty and type the take's steps
at their times. Step times are seconds into the final video; recording starts
`preroll` seconds earlier so pho has loaded by then.

A step without "at" follows the previous one: keys and typed text wait their
"gap" (seconds) after them, and {"sleep": s} waits. With "shell": true the take
starts at a zsh prompt in the repos dir instead of in pho.

usage: cast.py <take.json> <out.cast>
Writes <out.cast> and <out.cast>.keys.json (when each key was sent, in video
time, and the take's steps with their times filled in).
"""
import fcntl, json, os, pty, select, struct, subprocess, sys, termios, time

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

SEQ = {"down": "\x1b[B", "up": "\x1b[A", "enter": "\r", "esc": "\x1b", "tab": "\t", "shift+tab": "\x1b[Z",
       "space": " ", "ctrl+p": "\x10", "ctrl+d": "\x04", "ctrl+u": "\x15", "ctrl+s": "\x13"}

# Bubble Tea names a read of "u" then "p" the key "up", the up arrow, and
# pho's text fields act on it. pho can be mid-render when both arrive, so hold
# back the second of any typed pair that spells a key name.
HOLD = 0.3


def char_times(text, speed):
    times, extra = [], 0.0
    for i, ch in enumerate(text):
        if text[i - 1:i + 1] == "up":
            extra += HOLD
        times.append(i * speed + extra)
    return times


# give every step its time
t = 0.0
for s in take["steps"]:
    t = s.setdefault("at", t)
    if "key" in s:
        t += s.get("gap", 0)
    elif "type" in s:
        t += char_times(s["type"], s["speed"])[-1] + s["speed"] + s.get("gap", 0)
    elif "sleep" in s:
        t += s["sleep"]
take.setdefault("duration", round(t, 3))

# (time, bytes, event) for every keystroke
sends = []
for s in take["steps"]:
    cap = {} if s.get("cap", True) else {"cap": False}
    if "key" in s:
        sends.append((s["at"], SEQ.get(s["key"], s["key"]), {"key": s["key"], **cap}))
    elif "type" in s:
        for i, (ch, dt) in enumerate(zip(s["type"], char_times(s["type"], s["speed"]))):
            sends.append((s["at"] + dt, ch, {"char": ch, "text": s["type"], **cap} if i == 0 else None))

pid, fd = pty.fork()
if pid == 0:
    os.chdir(f"{W}/world/repos")
    cmd = "pho"
    if take.get("shell"):
        zdot = f"{W}/rec/zdot"
        os.makedirs(zdot, exist_ok=True)
        with open(f"{zdot}/.zshrc", "w") as f:
            f.write("PROMPT='%F{8}~/code%f %F{141}❯%f '\nunsetopt PROMPT_SP\n")
        env["ZDOTDIR"] = zdot
        cmd = "zsh -i"
    os.execvpe("asciinema", ["asciinema", "rec", "--overwrite", "-q", "-f", "asciicast-v2", "--window-size",
                             f"{cols}x{rows}", "-c", cmd, out], env)
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
if take.get("shell"):
    # asciinema waits for its shell, so hang that up (it takes pho with it)
    subprocess.run(["pkill", "-HUP", "-P", str(pid)])
else:
    os.kill(pid, 15)
while True:  # drain until asciinema closes the pty, or it blocks on exit
    try:
        if not os.read(fd, 65536):
            break
    except OSError:
        break
os.waitpid(pid, 0)
json.dump({"preroll": pre, "first_out": first_out, "duration": take["duration"], "steps": take["steps"], "keys": log},
          open(out + ".keys.json", "w"), indent=1)
print(f"{out}: {len(log)} key events")
