#!/bin/zsh
# Record the 30s themes demo against the fake GitHub: asciinema + agg, no browser.
#
#   demo/themes.sh            # -> demo/out/pho-themes.mp4 and pho-themes.gif
#
# Reuses demo/run.sh's fake world and pho build (run `demo/run.sh --fresh sample`
# once first, or after changing pho).
set -e
DEMO=${0:A:h}
export DEMO_WORK=${DEMO_WORK:-$DEMO/.work}
W=$DEMO_WORK
OUT=${OUT:-$DEMO/out/pho-themes.mp4}

for c in asciinema agg ffmpeg swift python3; do
  command -v $c >/dev/null || { echo "missing: $c"; exit 1; }
done
[[ -x $W/bin/pho && -d $W/world/repos/tidepool ]] || { echo "run demo/run.sh --fresh sample first"; exit 1; }
(cd $DEMO/mockgh && go build -o $W/bin/mockgh .)
cp $DEMO/bin/gh $W/bin/gh
mkdir -p $W/rec ${OUT:h}

echo "==> recording"
$DEMO/rec/reset.sh 0 90ms
python3 $DEMO/rec/cast.py $DEMO/rec/themes.json $W/rec/themes.cast
lsof -ti tcp:${${MOCK_ADDR:-127.0.0.1:8788}##*:} | xargs kill 2>/dev/null || true

echo "==> composing"
python3 $DEMO/rec/compose_cast.py $DEMO/rec/themes.json $W/rec/themes.cast $OUT ${OUT:r}.gif
