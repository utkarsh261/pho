# tidepool-dev — fake world manifest (shared by every builder)

Fictional org `tidepool-dev` on github.com. Nothing here refers to real people or companies.
"Now" for the video is 2026-10-05 ~15:00 UTC. All git dates fall in 2026-08-01 … 2026-10-05 14:00 UTC.

**Tidepool** is an open-source event ingestion & routing service written in Go: devices/apps POST
event batches to an HTTP/gRPC ingest API, events are written to a WAL, routed by rules, and
delivered to sinks (Kafka, S3, webhooks). Multi-tenant: every request carries an API key that maps
to a tenant.

## People (login — name — email)
- kmiller — Kate Miller — kate@tidepool.dev   ← THE VIEWER (the person using pho)
- sarahhill — Sarah Hill — sarah@tidepool.dev
- jcole — James Cole — james@tidepool.dev
- emward — Emma Ward — emma@tidepool.dev
- tbaker — Tom Baker — tom@tidepool.dev
- aclark — Amy Clark — amy@tidepool.dev
- bhughes — Ben Hughes — ben@tidepool.dev
- lgrant — Lucy Grant — lucy@tidepool.dev
- bots: dependabot[bot], renovate[bot]

## Repos (local paths under world/repos/<name>, default branch `main`)
- tidepool — Go 1.25 service (module github.com/tidepool-dev/tidepool)
- tidepool-web — TypeScript + React 19 + Vite admin dashboard
- infra — Terraform + Helm for the hosted deployment (AWS, EKS)
- sdk-go — Go client SDK (module github.com/tidepool-dev/sdk-go)

## Pull requests (open unless stated). Branch → PR. Each branch forks from main.
### tidepool
- #482 `feat/tenant-rate-limits` — "Add per-tenant rate limiting to ingest API" — sarahhill — HERO PR.
  Review requested: kmiller, jcole. ~8–10 files, roughly +380/−40, 4–5 commits.
- #497 `chore/proto-v2-regen` — "Regenerate protobufs for v2 event schema" — tbaker — ~1,240 changed files.
- #489 `fix/wal-fsync-on-rotate` — "Fix WAL segment fsync on rotation" — kmiller
- #491 `feat/otel-trace-propagation` — "Propagate W3C trace context through router" — jcole
- #486 `refactor/router-worker-pool` — "Replace router channel fan-out with bounded worker pool" — bhughes (draft)
- #493 `dependabot/go_modules/google.golang.org/grpc-1.68.0` — "Bump google.golang.org/grpc from 1.66.2 to 1.68.0" — dependabot[bot]
- #495 `fix/kafka-sink-retry-jitter` — "Add jitter to Kafka sink retry backoff" — aclark
- #498 `docs/config-reference` — "Document TIDEPOOL_* environment variables" — kmiller
- (no PR) `feat/s3-sink-multipart` — kmiller's local branch, pushed but no PR yet (for the Create-PR scene)
### tidepool-web
- #214 `feat/rate-limit-usage-chart` — "Show per-tenant rate limit usage on tenant page" — emward (review requested: kmiller)
- #211 `fix/stream-table-virtualization` — "Virtualize stream table for tenants with 10k+ streams" — lgrant
- #217 `renovate/vite-6.x` — "Update dependency vite to v6.0.3" — renovate[bot]
- #209 `feat/settings-theme-toggle` — "Add theme toggle to settings" — kmiller
### infra
- #77 `feat/keda-ingest-autoscaling` — "Scale ingest pods on WAL queue depth with KEDA" — tbaker (review requested: kmiller)
- #75 `chore/staging-eu-central-1` — "Move staging cluster to eu-central-1" — kmiller
### sdk-go
- #61 `fix/retry-after-429` — "Respect Retry-After on 429 responses" — aclark (kmiller mentioned/involved)
- #58 `feat/batch-client` — "Add batching client with flush on size or interval" — jcole
