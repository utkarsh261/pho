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
