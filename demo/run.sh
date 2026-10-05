#!/bin/zsh
# Record the pho demo video end to end against a fake GitHub.
#
#   demo/run.sh [take]           # default take: full  -> demo/out/pho-demo.mp4 (+ .jpg)
#   demo/run.sh --fresh [take]   # rebuild the fake repos and pho first
#
# See demo/README.md for what each step does and what it needs.
set -e
DEMO=${0:A:h}
ROOT=${DEMO:h}
export DEMO_WORK=${DEMO_WORK:-$DEMO/.work}
W=$DEMO_WORK
FRESH=0
[[ $1 == --fresh ]] && { FRESH=1; shift; }
TAKE=${1:-full}
OUT=${OUT:-$DEMO/out/pho-demo.mp4}
POSTER_AT=${POSTER_AT:-7.5}

for c in vhs ffmpeg go node python3 git script curl; do
  command -v $c >/dev/null || { echo "missing: $c"; exit 1; }
done
fonts=(~/Library/Fonts/JetBrainsMono-*(N) /Library/Fonts/JetBrainsMono-*(N))
(( ${#fonts} )) || { echo "missing font: JetBrains Mono (https://www.jetbrains.com/lp/mono/)"; exit 1; }

if (( FRESH )); then
  git -C $ROOT worktree remove --force $W/pho-src 2>/dev/null || true
  rm -rf $W
fi
mkdir -p $W/bin $W/rec ${OUT:h}

# ---- fake world: four repos from bundles, each with a local bare "origin" ----
if [[ ! -d $W/world/repos/tidepool ]]; then
  echo "==> unpacking fake repos"
  mkdir -p $W/world/repos $W/world/remotes
  for r in tidepool tidepool-web infra sdk-go; do
    git clone -q --mirror $DEMO/world/repos/$r.bundle $W/world/remotes/$r.git
    git clone -q $W/world/remotes/$r.git $W/world/repos/$r
    # pho reads the GitHub owner/name from the origin URL; git fetches from the local mirror
    git -C $W/world/repos/$r remote set-url origin https://github.com/tidepool-dev/$r.git
    git -C $W/world/repos/$r config url."$W/world/remotes/$r.git".insteadOf https://github.com/tidepool-dev/$r.git
    git -C $W/world/repos/$r fetch -q origin
  done
  for r in tidepool-web infra sdk-go; do git -C $W/world/repos/$r checkout -q main; done
  git -C $W/world/repos/tidepool checkout -q feat/s3-sink-multipart
fi

# ---- pho with the demo hooks (mock API URL, slow-motion timers) ----
if [[ ! -x $W/bin/pho ]]; then
  echo "==> building pho with demo hooks"
  git -C $ROOT worktree remove --force $W/pho-src 2>/dev/null || true
  git -C $ROOT worktree add -q --detach $W/pho-src HEAD
  git -C $W/pho-src apply $DEMO/pho-demo.patch
  (cd $W/pho-src && go build -o $W/bin/pho ./cmd/pho)
fi
(cd $DEMO/mockgh && go build -o $W/bin/mockgh .)
cp $DEMO/bin/gh $W/bin/gh

# ---- record ----
echo "==> recording take '$TAKE'"
node $DEMO/rec/gen.mjs $TAKE
SLOW=$(python3 -c "import json;print(json.load(open('$W/rec/$TAKE.events.json'))['slow'])")
$DEMO/rec/reset.sh 0 $((90 * SLOW))ms
cd $W/rec
rm -rf $TAKE-frames $TAKE.keys.rec
vhs $TAKE.tape > vhs-$TAKE.log 2>&1 || { tail -20 vhs-$TAKE.log; exit 1; }
lsof -ti tcp:${${MOCK_ADDR:-127.0.0.1:8788}##*:} | xargs kill 2>/dev/null || true

# ---- compose ----
echo "==> composing $OUT"
rm -rf cards
python3 $DEMO/rec/cards.py cards $TAKE.cut.json
python3 $DEMO/rec/compose.py $TAKE $TAKE.cut.json $OUT $POSTER_AT
echo "==> done: $OUT"
