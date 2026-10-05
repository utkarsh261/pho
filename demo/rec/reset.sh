#!/bin/zsh
# Reset the fake world before a take: fresh mock server, empty pho caches,
# tidepool back on its unpushed branch, tidepool listed first.
# usage: reset.sh [e2e-turns-success-after-seconds] [mock-latency]
DEMO=${0:A:h:h}
W=${DEMO_WORK:-$DEMO/.work}
MOCK=${MOCK_ADDR:-127.0.0.1:8788}
E2E=${1:-0}
LAT=${2:-90ms}
R=$W/world/repos

lsof -ti tcp:${MOCK##*:} | xargs kill 2>/dev/null; sleep 0.3   # whatever holds the mock port
rm -rf $W/xdg; mkdir -p $W/xdg/c $W/xdg/s $W/xdg/d $W/rec
git -C $R/tidepool checkout -q -f feat/s3-sink-multipart
for b in $(git -C $R/tidepool branch --format='%(refname:short)'); do
  [[ $b == feat/s3-sink-multipart ]] || git -C $R/tidepool branch -q -D $b
done
# restore every PR branch from the local remote (checkout scenes create/move them)
for r in tidepool tidepool-web infra sdk-go; do
  for b in $(git -C $R/$r for-each-ref --format='%(refname:lstrip=3)' refs/remotes/origin); do
    [[ $b == HEAD || $b == $(git -C $R/$r branch --show-current) ]] || git -C $R/$r branch -q -f $b origin/$b
  done
done

python3 - "$DEMO/world/prs.json" "$W/rec/take-fixture.json" "$R" "$E2E" <<'EOF'
import json, sys
src, dst, repos, e2e = sys.argv[1:5]
d = json.load(open(src))
d["reposDir"] = repos
for p in d["prs"]:
    if p["number"] == 482:
        for c in p["checks"]:
            if c["name"] == "e2e":
                c["turnsSuccessAfter"] = int(e2e)
json.dump(d, open(dst, "w"))
EOF

# repo order in pho follows directory mtime: newest first
i=0; for r in infra sdk-go tidepool-web tidepool; do
  i=$((i+1)); t=$(date -v+${i}M +%Y%m%d%H%M.%S)
  touch -t $t $R/$r $R/$r/.git $R/$r/.git/config
done

nohup $W/bin/mockgh -fixture $W/rec/take-fixture.json -addr $MOCK -latency $LAT > $W/rec/mock.log 2>&1 &
q='{"query":"query ViewerQuery { viewer { login } }"}'
for i in {1..30}; do curl -s -o /dev/null -X POST http://$MOCK/graphql -d $q && break; sleep 0.3; done
curl -s -o /dev/null -w "mock %{http_code}\n" -X POST http://$MOCK/graphql -d $q
