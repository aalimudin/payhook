# payhook
A small, production-minded **payment webhook receiver in Go**. It accepts webhooks from Midtrans, Xendit and Stripe, verifies them, records each event exactly once, and reliably forwards it to your application.

Payment webhooks look simple but fail in quiet, expensive ways: forged requests, duplicate deliveries that ship an order twice, events arriving out of order, and a downstream app that is down at the wrong moment. payhook handles those so your app doesn't have to.

## What it does

| Problem | How payhook handles it |
|---|---|
| Forged or tampered webhooks | Verifies each provider's signature with a constant-time comparison; Stripe timestamps are checked to block replays |
| Same webhook delivered twice | Deduplicates on `(provider, event_id)` with a unique constraint, so a repeat is acknowledged but never processed again |
| Events arriving out of order | A payment state machine refuses backward moves (e.g. `paid` → `pending`) |
| Amount mismatch | Rejects a "paid" event whose amount doesn't match the stored order |
| Your app is down | Transactional outbox + background worker with exponential backoff and a dead-letter state |
| Slow provider timeouts | Responds `200` as soon as the event is safely stored; forwarding happens asynchronously |

## Architecture

```
Provider ──POST /webhooks/{provider}──▶ verify signature
                                         │
                                         ▼
                         ┌── one DB transaction ──┐
                         │ insert event (dedupe)  │
                         │ update payment state   │
                         │ insert outbox row      │
                         └────────────────────────┘
                                         │  200 OK
                                         ▼
                     worker (FOR UPDATE SKIP LOCKED)
                                         │  retry w/ backoff
                                         ▼
                              your app's endpoint
```

## Supported providers

| Provider | Verification | Event ID used for dedupe |
|---|---|---|
| Midtrans | `signature_key` = SHA-512(`order_id` + `status_code` + `gross_amount` + server key) | `transaction_id` + `transaction_status` |
| Xendit | `x-callback-token` header matches your verification token | payload `id` + `status` |
| Stripe | `Stripe-Signature` HMAC-SHA256 over `timestamp.payload`, 5-minute tolerance | `event.id` |

Adding a provider means implementing one interface:

```go
type Provider interface {
    Name() string
    Verify(r *http.Request, body []byte) error
    Parse(body []byte) (Event, error)
}
```

## Quick start

```bash
cp .env.example .env        # add sandbox keys only
docker compose up -d        # Postgres + payhook
make test                   # unit + integration tests
```

Send a test webhook:

```bash
./scripts/send-midtrans-sample.sh   # signs a sample payload with your sandbox key
```

## Configuration

| Variable | Description |
|---|---|
| `DATABASE_URL` | Postgres connection string |
| `FORWARD_URL` | Where verified events are delivered |
| `MIDTRANS_SERVER_KEY` | Midtrans sandbox server key |
| `XENDIT_CALLBACK_TOKEN` | Xendit webhook verification token |
| `STRIPE_WEBHOOK_SECRET` | Stripe endpoint signing secret (`whsec_...`) |
| `MAX_ATTEMPTS` | Forwarding attempts before dead-letter (default `8`) |

## Design choices

- **Standard library first:** `net/http` routing, `log/slog`, `database/sql`. The only dependency is the `pgx` Postgres driver.
- **Postgres as the queue:** an outbox table with `SKIP LOCKED` is enough at this scale and avoids running a separate broker.
- **No secrets in code or logs:** keys come from the environment; request bodies are logged without signature fields.

## Not in scope (yet)

Metrics endpoint, admin UI, multi-tenant keys, payment *creation* APIs. This project only receives and forwards.

## License

MIT
>>>>>>> 872063b (initial commit)
