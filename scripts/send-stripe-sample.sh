#!/usr/bin/env bash
# Sends a signed sandbox Stripe event. Needs STRIPE_WEBHOOK_SECRET (sandbox only).
set -euo pipefail
URL=${URL:-http://localhost:8080/webhooks/stripe}
TS=$(date +%s)
BODY="{\"id\":\"evt_$RANDOM\",\"type\":\"payment_intent.succeeded\",\"data\":{\"object\":{\"id\":\"pi_1\",\"amount\":500,\"metadata\":{\"order_id\":\"order-$RANDOM\"}}}}"
SIG=$(printf '%s' "$TS.$BODY" | openssl dgst -sha256 -hmac "$STRIPE_WEBHOOK_SECRET" -hex | awk '{print $NF}')
curl -sS -i -X POST "$URL" -H "Stripe-Signature: t=$TS,v1=$SIG" -H 'Content-Type: application/json' -d "$BODY"
