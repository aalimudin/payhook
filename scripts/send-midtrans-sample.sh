#!/usr/bin/env bash
# Sends a signed sandbox Midtrans notification. Needs MIDTRANS_SERVER_KEY (sandbox only).
set -euo pipefail
URL=${URL:-http://localhost:8080/webhooks/midtrans}
ORDER=${ORDER:-order-$RANDOM} CODE=200 AMOUNT=${AMOUNT:-10000.00}
SIG=$(printf '%s' "$ORDER$CODE$AMOUNT$MIDTRANS_SERVER_KEY" | openssl dgst -sha512 -hex | awk '{print $NF}')
curl -sS -i -X POST "$URL" -H 'Content-Type: application/json' -d "{\"transaction_id\":\"tx-$ORDER\",\"transaction_status\":\"settlement\",\"order_id\":\"$ORDER\",\"status_code\":\"$CODE\",\"gross_amount\":\"$AMOUNT\",\"signature_key\":\"$SIG\"}"
