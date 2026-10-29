# mockgh fixture schema (JSON)

Times are relative to server start: "-2h15m", "-3d", "-1w" (units s m h d w), "" = now, or RFC3339.
Unset/omitted fields are fine everywhere. See sample-fixture.json.

Top level
- viewer: login of the person using pho (default "kmiller")
- reposDir: dir holding the git repos (relative to the fixture file; default ../repos)
- people: { "git author email": "login" } for commit author -> user mapping
- repos: { "owner/name": { dir, defaultBranch ("main"), extraPRs: [{number,title,author,state MERGED|CLOSED,updated}], labels: [{name,color}] } }
  (dir defaults to the repo name under reposDir; extraPRs only appear in the All tab; labels are the repo's labels beyond those its PRs use, for the label picker)
- prs: list of PRs

PR
- repo "owner/name", number, branch (local git branch; diff = `git diff <base>...<branch>`), base (default repo's defaultBranch)
- title, body (markdown), author, state OPEN|CLOSED|MERGED (default OPEN), isDraft, created, updated
- labels: [{name,color}], assignees: [login], reviewRequests: [login]
- reviews: [{author, state APPROVED|CHANGES_REQUESTED|COMMENTED, body, at, commit}]
  (commit: git rev the review was on, e.g. "feat/x~1"; default the PR head. Drives "changes since your last review")
- reviewThreads: [{path, line (new side), resolved, resolvedBy, comments: [{author, body, at}]}]
- comments: [{author, body, at}]   (issue comments)
- checks: [{name, status COMPLETED|IN_PROGRESS|QUEUED (default COMPLETED), conclusion SUCCESS|FAILURE|..., turnsSuccessAfter: seconds after server start -> becomes COMPLETED/SUCCESS,
  log, rerunLog, rerunTakes}]
  (log: the Actions job log served for the check; rerunLog: the log once a re-run has passed; rerunTakes: seconds a re-run stays in progress, default 8.
   A re-run reports QUEUED for 2 s, then IN_PROGRESS, then COMPLETED/SUCCESS. Names with "/" get a non-Actions details URL)
- mergeable (MERGEABLE|CONFLICTING|UNKNOWN, default MERGEABLE), mergeStateStatus (override), reviewDecision (override; else derived)

Derived from git: additions/deletions/changedFiles, files+patches, headRefOid, commits (author email -> people), commit diffs, compare (base...head).
Git commit dates are shifted so the world's "now" (2026-10-05 15:00 UTC) is server start.

REST beyond PRs: Actions job logs and re-runs (`actions/jobs/{id}/logs|rerun`), repo labels (`labels`), issue labels
(`issues/{n}/labels`, add and remove), and `compare/{base}...{head}` (JSON, or a diff with the diff Accept header).
Derived: CI rollup from checks, reviewDecision from reviews/requests, latest activity.
