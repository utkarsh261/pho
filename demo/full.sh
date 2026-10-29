#!/bin/zsh
# Record the full demo against the fake GitHub: asciinema + agg, no browser.
#
#   demo/full.sh                       # -> demo/out/pho-demo.mp4 (+ .jpg poster)
#   TAKE=full-v2 demo/full.sh          # the zooming take -> demo/out/pho-demo-v2.mp4
#   TAKE=full-v3 demo/full.sh          # camera that follows the action, title cards
#   SKIP_RECORD=1 PREVIEW=1 ...        # recompose the last recording, quickly
#
# Reuses demo/run.sh's fake world and pho build (run `demo/run.sh --fresh sample`
# once first, or after changing pho or the bundles).
set -e
DEMO=${0:A:h}
export DEMO_WORK=${DEMO_WORK:-$DEMO/.work}
W=$DEMO_WORK
TAKE=${TAKE:-full}
OUT=${OUT:-$DEMO/out/pho-demo${TAKE#full}.mp4}
POSTER_AT=${POSTER_AT:-$(python3 -c "import json,sys; print(json.load(open(sys.argv[1])).get('poster', 6))" $DEMO/rec/$TAKE.json)}

for c in asciinema agg ffmpeg swift python3 tmux; do
  command -v $c >/dev/null || { echo "missing: $c"; exit 1; }
done
[[ -x $W/bin/pho && -d $W/world/repos/tidepool ]] || { echo "run demo/run.sh --fresh sample first"; exit 1; }
(cd $DEMO/mockgh && go build -o $W/bin/mockgh .)
cp $DEMO/bin/gh $W/bin/gh
mkdir -p $W/rec ${OUT:h}

if [[ -z $SKIP_RECORD ]]; then
echo "==> warming up"
$DEMO/rec/reset.sh 0 90ms
# open two PRs off camera so the Recent tab has history
tmux kill-session -t pho-warmup 2>/dev/null || true
tmux new-session -d -s pho-warmup -x 200 -y 53 -c $W/world/repos \
  "env PATH=$W/bin:\$PATH PHO_MOCK_API=http://${MOCK_ADDR:-127.0.0.1:8788} XDG_CACHE_HOME=$W/xdg/c XDG_CONFIG_HOME=$W/xdg/cfg \
   XDG_STATE_HOME=$W/xdg/s XDG_DATA_HOME=$W/xdg/d TERM=xterm-256color pho"
k() { for x in "$@"; do tmux send-keys -t pho-warmup "$x"; sleep 0.5; done }
sleep 4
for q in trace jitter; do
  k C-p; tmux send-keys -t pho-warmup -l $q; sleep 0.8; k Enter; sleep 3; k q; sleep 1
done
k q; sleep 1
tmux kill-session -t pho-warmup 2>/dev/null || true

echo "==> recording"
python3 $DEMO/rec/cast.py $DEMO/rec/$TAKE.json $W/rec/$TAKE.cast
lsof -ti tcp:${${MOCK_ADDR:-127.0.0.1:8788}##*:} | xargs kill 2>/dev/null || true
fi

if grep -q '"autocam": true' $DEMO/rec/$TAKE.json; then
  echo "==> camera"
  [[ -x $W/venv/bin/python ]] || { python3 -m venv $W/venv && $W/venv/bin/pip -q install pyte; }
  $W/venv/bin/python $DEMO/rec/autocam.py $W/rec/$TAKE.cast
  $W/venv/bin/python $DEMO/rec/verify.py $W/rec/$TAKE.cast | tee $W/rec/$TAKE.verify.txt || true
fi

echo "==> composing"
python3 $DEMO/rec/compose_cast.py $DEMO/rec/$TAKE.json $W/rec/$TAKE.cast $OUT
ffmpeg -v error -y -ss $POSTER_AT -i $OUT -frames:v 1 -q:v 2 ${OUT:r}.jpg
echo "==> done: $OUT"
