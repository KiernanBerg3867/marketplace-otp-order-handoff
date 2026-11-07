#!/bin/sh
set -eu

: "${BUYER_PHONE:?set BUYER_PHONE to the order buyer's E.164 phone}"

attempt_id="order-42-$(date +%s)"
curl -sS -X POST http://localhost:8080/login/code \
  -H 'Content-Type: application/json' \
  -d "{\"order_id\":\"order-42\",\"phone\":\"$BUYER_PHONE\",\"attempt_id\":\"$attempt_id\"}"
printf '\nCode: '
read -r code
curl -sS -X POST http://localhost:8080/login/verify \
  -H 'Content-Type: application/json' \
  -d "{\"order_id\":\"order-42\",\"phone\":\"$BUYER_PHONE\",\"code\":\"$code\",\"attempt_id\":\"$attempt_id\"}"
printf '\n'
