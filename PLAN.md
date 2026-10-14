# payhook — build plan

Private brief for building the project with a coding agent. Not for the public repo.

## Ground rules

- Use **only public provider docs** (Midtrans, Xendit, Stripe). No code, names or patterns from any employer.
- **Sandbox keys only.** The agent never sees real credentials.
- Before coding each provider, re-check its current webhook docs. Signature formats change.
- Keep it small: target **under ~1,500 lines of Go** excluding tests.

## Repo layout

```
cmd/payhook/main.go        wiring, config, graceful shutdown
internal/provider/         midtrans.go, xendit.go, stripe.go (+ _test.go each)
internal/store/            postgres queries, migrations
internal/payment/          state machine
internal/outbox/           worker, backoff
scripts/                   sample payload senders
migrations/                001_init.sql
docker-compose.yml, Dockerfile, Makefile, .github/workflows/ci.yml
```

## Milestones

Each milestone ends with passing tests and a commit.

### M1 — Skeleton (½ day)
- `POST /webhooks/{provider}`, `GET /healthz`, slog JSON logs, graceful shutdown.
- Postgres via docker-compose, migration for `events`, `payments`, `outbox`.
- **Done when:** `docker compose up` works and `/healthz` returns 200.

### M2 — Verification (1 day)
- Implement `Provider` for Midtrans, Xendit, Stripe.
- `hmac.Equal` / `subtle.ConstantTimeCompare` for all comparisons.
- Stripe: reject timestamps older than 5 minutes.
- Cap request body size (e.g. 1 MB).
- **Done when:** table tests cover valid, tampered body, wrong key, missing header, expired Stripe timestamp.

### M3 — Exactly-once recording (1 day)
- `events` table: `UNIQUE (provider, event_id)`; `INSERT ... ON CONFLICT DO NOTHING`.
- Same transaction: update `payments` via state machine, insert `outbox` row.
- Duplicate → return 200, do nothing else.
- **Done when:** integration test sends the same webhook 10 times concurrently → exactly 1 outbox row.

### M4 — State machine and amount check (½ day)
- States: `pending → paid → refunded`, `pending → failed/expired`. Reject backward moves; log them.
- "Paid" with an amount different from the stored order → reject and log.
- **Done when:** tests for out-of-order (`paid` then late `pending`) and amount mismatch.

### M5 — Outbox worker (1 day)
- Poll with `SELECT ... FOR UPDATE SKIP LOCKED LIMIT n`.
- Exponential backoff with jitter; `dead` status after `MAX_ATTEMPTS`.
- Forward request carries an `Idempotency-Key` header = event ID.
- **Done when:** test with a flaky fake downstream (fails 3×, then succeeds) → delivered once; always-failing downstream → `dead`.

### M6 — Polish (½ day)
- GitHub Actions: `go vet`, `go test -race`, build image.
- Sample scripts that sign payloads with sandbox keys.
- README screenshots/log samples.

**Total: about 4–5 working days** of agent time plus your reviews, spread over 2 weeks at 1–2 h/day.

## The "naive" version (for the fix-it case study)

Build `naive/` as a separate branch: a typical first-try implementation with these planted problems. Then audit it as if a client sent it to you.

| # | Planted problem | Real-world impact |
|---|---|---|
| 1 | No signature check | Anyone can mark an order paid with curl |
| 2 | Signature compared with `==` | Timing leak |
| 3 | No dedupe | Repeat webhook ships the order twice |
| 4 | Status overwritten blindly | Late `pending` un-pays a paid order |
| 5 | Amount not checked | Underpayment accepted as paid |
| 6 | Calls downstream inside the request | Provider times out and retries → more duplicates |
| 7 | Server key hard-coded and logged | Secret leaks via repo and logs |
| 8 | No body size limit | Easy memory exhaustion |

Case study format: one page per problem — **what I found → how to reproduce → fix → test that proves it**.

## Article outlines (from this repo)

**1. "Payment webhooks that don't double-charge: idempotency in Go"** (pitch: Semaphore, Honeybadger-style blogs, general dev blogs)
- Why providers retry and why duplicates are normal
- Unique constraint vs. "check then insert" race
- Outbox pattern in one transaction
- Test: 10 concurrent duplicates → 1 effect

**2. "Deploying a Go webhook service with Docker on a $6 VPS"** (pitch: Vultr, Civo)
- Multi-stage Dockerfile, non-root user
- Postgres + compose, migrations on start
- TLS with Caddy, health checks, zero-downtime restart

**3. "CI for a Go service: race detector, integration tests with Postgres"** (pitch: Semaphore)
- Service containers in CI
- `go test -race`, test DB per run
- Caching modules, building the image

**4. Case study post** (your own blog / Fiverr gig): "8 bugs I find in almost every first-try payment webhook"

## Definition of done (portfolio-ready)

- [ ] CI green, badge in README
- [ ] Coverage on `provider/`, `payment/`, `outbox/`
- [ ] Case study published
- [ ] At least one article published and linked from README
- [ ] Pinned on GitHub profile
