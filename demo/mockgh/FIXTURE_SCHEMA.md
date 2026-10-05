# mockgh fixture schema (JSON)

Times are relative to server start: "-2h15m", "-3d", "-1w" (units s m h d w), "" = now, or RFC3339.
Unset/omitted fields are fine everywhere. See sample-fixture.json.

Top level
- viewer: login of the person using pho (default "kmiller")
- reposDir: dir holding the git repos (relative to the fixture file; default ../repos)
- people: { "git author email": "login" } for commit author -> user mapping
- repos: { "owner/name": { dir, defaultBranch ("main"), extraPRs: [{number,title,author,state MERGED|CLOSED,updated}] } }
  (dir defaults to the repo name under reposDir; extraPRs only appear in the All tab)
- prs: list of PRs

PR
- repo "owner/name", number, branch (local git branch; diff = `git diff <base>...<branch>`), base (default repo's defaultBranch)
- title, body (markdown), author, state OPEN|CLOSED|MERGED (default OPEN), isDraft, created, updated
- labels: [{name,color}], assignees: [login], reviewRequests: [login]
- reviews: [{author, state APPROVED|CHANGES_REQUESTED|COMMENTED, body, at}]
- reviewThreads: [{path, line (new side), resolved, resolvedBy, comments: [{author, body, at}]}]
- comments: [{author, body, at}]   (issue comments)
- checks: [{name, status COMPLETED|IN_PROGRESS|QUEUED (default COMPLETED), conclusion SUCCESS|FAILURE|..., turnsSuccessAfter: seconds after server start -> becomes COMPLETED/SUCCESS}]
- mergeable (MERGEABLE|CONFLICTING|UNKNOWN, default MERGEABLE), mergeStateStatus (override), reviewDecision (override; else derived)

Derived from git: additions/deletions/changedFiles, files+patches, headRefOid, commits (author email -> people), commit diffs.
Derived: CI rollup from checks, reviewDecision from reviews/requests, latest activity.
