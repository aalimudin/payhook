#!/usr/bin/env bash
# Sends a sandbox Xendit invoice callback. Needs XENDIT_CALLBACK_TOKEN (sandbox only).
set -euo pipefail
URL=${URL:-http://localhost:8080/webhooks/xendit}
curl -sS -i -X POST "$URL" -H "X-Callback-Token: $XENDIT_CALLBACK_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"id\":\"inv-$RANDOM\",\"external_id\":\"order-$RANDOM\",\"status\":\"PAID\",\"amount\":15000}"
