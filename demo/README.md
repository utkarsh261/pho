# pho demo video

Records the pho demo video end to end: the real pho binary, driven by a
script, against a fake GitHub org (`tidepool-dev`) served from a local mock.
Nothing talks to github.com.

```sh
demo/run.sh            # full take  -> demo/out/pho-demo.mp4 + pho-demo.jpg
demo/run.sh sample     # 10s take, handy for checking the look
demo/run.sh --fresh    # rebuild the fake repos and pho first
```

A full take takes about 7 minutes to record and 6 to encode. Output is 4K,
30 fps, with key clicks.

## Requirements (macOS)

- `vhs`, `ffmpeg`, `go`, `node`, `python3`, `git`, `curl`
- JetBrains Mono installed (the terminal font)
- Google Chrome or Chromium (renders the key chips and labels; set `CHROME=` to override)
- `script` (BSD, ships with macOS) logs pho's real key timings

## How it works

1. **Fake world**: `world/repos/*.bundle` are four small git repos
   (`tidepool`, `tidepool-web`, `infra`, `sdk-go`) with a branch per PR. They
   unpack into `.work/world/repos`, each with a local mirror as `origin` so
   checkout works offline. `world/prs.json` holds the PR metadata: titles,
   bodies, reviews, threads and checks (schema in `mockgh/FIXTURE_SCHEMA.md`).
   People and PRs are listed in `world/MANIFEST.md`.
2. **Mock GitHub** (`mockgh/`, stdlib Go): answers pho's GraphQL and REST
   calls from the fixture. Diffs, files and commits come from the git repos,
   so they're real. Mutations (review, reply, resolve, merge, create PR) change
   in-memory state, and the >300-file PR returns `406 too_large` like GitHub.
3. **pho with demo hooks** (`pho-demo.patch`, applied to a throwaway worktree):
   - `PHO_MOCK_API` points pho at the mock.
   - `PHO_SLOWMO` stretches status-message timers to match the slow-motion
     recording (step 4).
   - `bin/gh` stubs `gh auth` as `kmiller`.
4. **Recording** (`rec/gen.mjs`, `rec/takes.mjs`):
   - The take is a list of steps (keys, typed text, pauses, labels) and becomes
     a VHS tape. VHS only captures roughly 4–15 frames/s at this size, so takes
     run `slow` times slower (the full take uses 3) and play back sped up.
   - Each frame's file mtime is its capture time. pho runs under `script -r`,
     so every keystroke pho receives is logged on the same clock.
5. **Compose** (`rec/cards.py`, `rec/compose.py`): frames are padded onto a 4K
   canvas with no rescaling and played on their real timestamps. Key chips and
   labels go in the status bar at the exact key times, and the clicks are
   synthesised. One encode. A clean dashboard frame is baked in as frame 0
   (the poster).

`rec/reset.sh` resets everything between takes: mock state, pho caches,
branches, and repo order.

## Editing the take

Steps in `rec/takes.mjs`:

| step | does |
|---|---|
| `{ keys: ['j', 'enter'], gap: 0.5 }` | presses keys, each followed by `gap` seconds |
| `{ type: 'text', speed: 40 }` | types text, `speed` ms per character |
| `{ sleep: 1.2 }` | waits |
| `{ label: 'search the diff' }` | sets the status-bar label until the next one |
| `cap: false` | on keys or type: no key chip for this step |
| `shell: true` | keys that go to the shell, not pho (typing `pho` itself) |

Everything before `{ show: true }` is off camera. Use it to warm caches or set
up state. To look at a stretch of footage without encoding, run
`python3 demo/rec/sheet.py full 40 60 2 sheet.png` in `demo/.work/rec`.

Re-run `demo/run.sh --fresh` after changing the bundles or the patch.

## Themes demo (asciinema)

```sh
demo/run.sh --fresh sample   # once, or after changing pho: builds the fake world and pho
demo/themes.sh               # -> demo/out/pho-themes.mp4 (4K, 60 fps) + pho-themes.gif
```

A second pipeline that needs no browser: `asciinema`, `agg`, `ffmpeg`,
`swift` (ships with the Xcode command line tools) and `python3`.

1. `rec/cast.py` runs pho under `asciinema rec` (asciicast v2) on a 200x53 pty
   and sends each step of `rec/themes.json` at its time. It answers pho's
   terminal queries (background colour, cursor position) itself, and logs
   when each key went out.
2. `rec/compose_cast.py` renders the cast with agg at a size where the grid
   is exactly 3840x2120, pads it to 4K with the theme's own background, cuts
   exactly `duration` seconds, and overlays key chips and labels drawn by
   `rec/chips.swift` (CoreText). There is no audio. The GIF is made from the
   MP4 at 1600px, 20 fps.

Step times in `themes.json` are seconds into the final video; recording
starts `preroll` seconds earlier so pho has loaded. Steps are `{at, key}`,
`{at, type, speed}` (one chip for the word) or `{at, label}` (runs until the
next label). The take records into `$DEMO_WORK/xdg/cfg`, so the theme the
picker saves never touches your own config.

## Full demo (asciinema)

The full feature demo uses the same asciinema pipeline, with a camera and
title cards on top. Build the world and pho once with
`demo/run.sh --fresh sample`, then:

```sh
demo/full.sh                          # rec/full.json    -> out/pho-demo.mp4     (whole screen)
TAKE=full-v2 demo/full.sh             # rec/full-v2.json -> out/pho-demo-v2.mp4  (hand-placed zooms, theme switch)
TAKE=full-v3 demo/full.sh             # rec/full-v3.json -> out/pho-demo-v3.mp4  (camera follows the action, title cards)
SKIP_RECORD=1 PREVIEW=1 TAKE=full-v3 demo/full.sh   # re-cut the last recording quickly
```

A take records in about 3 minutes; the v3 compose takes about 25 (the cast
is drawn at 2x so text stays sharp when zoomed). `PREVIEW=1` draws at 1x
with a fast encode, for checking camera work. The poster is the frame at
the take's `poster` seconds (default 6). Before recording, `full.sh` resets
the world and opens two PRs off camera so the Recent tab has history.

Takes (`rec/full*.json`) set `cols`, `rows`, `preroll`, `shell` (start at
a zsh prompt and type `pho`) and a list of steps. A step without `at`
follows the previous one.

| step | does |
|---|---|
| `{key, gap}` | sends a key (`j`, `enter`, `ctrl+p`, `shift+tab`, ...), then waits `gap` s |
| `{type, speed, gap}` | types text, `speed` s per character; one key chip for the word |
| `{sleep}` | waits |
| `{label}` | caption pill until the next label |
| `cap: false` | on a key or type step: no key chip |
| `{section: {tab, title, sub, hints, theme}, sleep}` | a title card is cut in here (v3) |
| `cam` | the v3 camera for this key's action: `"full"`, `"hold"` (stay put), `[c0, r0, c1, r1]` (grid cells to frame), or a shot list `[[delay, rect or "full"], ...]` |
| `zoom`, `zd`, `zt` | v2 only: frame cells `[c0, r0, c1, r1]` (or `null`) from this step, `zd` s later, easing over `zt` s |

Take-level `open` (`tagline`, `version`, `theme`) and `end` (`cmd`, `url`,
`theme`) add the opening and closing cards, `autocam: true` runs the camera,
and `poster` picks the poster frame. Card themes are `pho-default` or
`tokyo-night` (colours from `internal/ui/theme`).

- `rec/autocam.py` replays the cast in a terminal emulator (pyte, installed
  into `.work/venv` by `full.sh`) and writes `<cast>.cam.json`. Keys under
  0.45 s apart are one action; the camera frames what each action changed on
  screen, moves ahead of typed text to where it lands, pulls back on scene
  changes, and holds a shot at least 0.9 s. Steps' `cam` override it; set
  `AUTOCAM_DEBUG=1` to see its choices. Each section cuts back to the whole
  screen under its card.
- `rec/verify.py` checks the camera path against the cast: typed text and
  local changes (popups, notices, selections) must be in frame. `full.sh`
  writes its report to `.work/rec/<take>.verify.txt`; some flags are by
  design (the camera staying on a list while the preview updates).
- `rec/camera.py` is the camera maths shared by both and by
  `rec/compose_cast.py`, which renders it with ffmpeg's `zoompan` up to 2x.
- `rec/titlecards.py` animates the cards from layers drawn by
  `rec/titlecard.swift`: pho's rounded box with `pho › 02 / 08` in its top
  border, a tab bar whose highlight slides to the section, the title typed in
  behind a block cursor, and a status-bar style key hint.
- `rec/chips.swift` gives captions a backing pill in zooming takes, since
  they can sit over pho's content.

To look at a stretch of a take, read frames out of the preview with ffmpeg
(`fps=2`) rather than trusting a few stills: problems show up mid-action.

Things that bit:

- Keys sent less than about 0.15 s apart reach pho in one read and Bubble Tea
  turns them into a single key that matches nothing. Keep gaps at 0.22 s or
  more.
- Typed `u` then `p` in one read is the key `up`, which text fields act on
  (the cursor moves, the text loses "up"). `cast.py` holds back the second of
  such a pair by 0.3 s.
- pho receives keys about 0.07 s after their scheduled time, so `autocam.py`
  pairs steps with logged keys by order, not by time.
- x264 must stay at 4 reference frames and level 5.2 at 4K. With more,
  QuickTime and Safari show stale blocks after a seek or a big screen change.
- ffmpeg expressions nest at most about 100 levels, so `camera.py` builds the
  `zoompan` path as a balanced tree of `if`s on time, not a chain.
- A moving camera costs bitrate: the v3 video is about 165 MB. Re-encode a
  copy at a higher CRF for anything with a 100 MB limit.
