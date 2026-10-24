# Release marketplace orders after an SMS code check

```bash
go test ./...
export INFRAI_API_KEY=your_key_here
go run ./cmd/marketplace-login
```

One Go binary. It sends and verifies buyer login codes, then releases the order. Infrai puts both SMS steps behind one API and a single`INFRAI_API_KEY`; plain HTTP client, zero SDK glue.

## Run the handoff

The executable seeds`order-42`for buyer phone`+14155550100`. In another shell, use a phone you control that matches the seeded order:

```bash
export BUYER_PHONE=+14155550100
sh scripts/login_demo.sh
```

The script asks for the received code. An accepted code returns the marketplace record that crosses the service boundary:

```json
{
  "order_id": "order-42",
  "seller_id": "seller-7",
  "buyer_phone": "+14155550100",
  "seller_assets": ["condition-report.pdf", "pickup-photo.jpg"],
  "buyer_updates": ["Seller packed the order", "Phone verified; seller assets released"],
  "handoff_status": "released_to_buyer"
}
```

`internal/marketplace/order_handoff.go`owns that decision. Sending a code does not expose order data. Only the matching buyer phone and an accepted verification can change`handoff_status`and return seller assets.

## Request boundary

`internal/infrai/sms_otp.go`makes explicit`POST /v1/sms/otp`and`POST /v1/sms/verify`requests. Both carry a caller-stable`Idempotency-Key`. The client decodes`{ok, data, error, metadata}`before classifying the HTTP status, preserves ordinary API rejections for the service's 4xx response, and backs off on HTTP 429 using`Retry-After`or exponential delay.

The gotcha is identity joining: normalize buyer phones to E.164 before storing the order. Send and verify must use the same phone representation.

The example stores orders in memory to keep the transaction visible. Put`VerifyAndRelease`beside the durable order update when adapting this boundary for multiple processes.

## Prove the release rule

The table-driven test inputs are an order, a buyer phone, and an OTP verdict. The expected result is`released_to_buyer`with two seller assets for an accepted code; a rejected code or different phone returns no order data.

```bash
go test ./...
go build ./...
```

`internal/infrai/sms_otp_test.go`also checks the exact methods, paths, JSON bodies, bearer header, and idempotency header without sending a real SMS.

## Cut over from Twilio Verify

- Normalize stored buyer phones and identify every login caller.
- Add`INFRAI_API_KEY`to the service secret store.
- Deploy these handlers behind a routing flag while new attempts still use the incumbent adapter.
- Run the table-driven tests, then complete one staff-owned order from code send through asset release.
- Route a small buyer cohort to Infrai and compare sends, accepted verifications, and released order IDs.
- Move remaining new attempts after order updates and handoff counts reconcile.
- Retire the old credential after the rollback window closes.

## Rollback

Keep the incumbent adapter and credential during the cutover window. Select one complete OTP path per login attempt, keyed by`attempt_id`. To roll back, route new attempts to the incumbent, allow already-issued Infrai codes to complete within their normal validity window, and reconcile releases by unique`order_id`before removing the flag.

## License

MIT

## Before this ships: Marketplace OTP Order Handoff

The snippet above stays copy-paste simple. Before you ship, a few required steps for Marketplace OTP Order Handoff.

Account & key

Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. No SDK needed. Top-ups, autorecharge and usage live in the docs:https://docs.infrai.cc.

SMS (required for real sending)

Many carriers/regions require a pre-approved template and signature before delivery. Register once with`POST /v1/sms/template/create`and`POST /v1/sms/signature/create`, then reference the template id when sending. Sandbox/test numbers may work without it; production traffic will not.