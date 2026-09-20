# fraud-transaction-service — API flow

Direct-to-service (`localhost:8082`), no gateway. **fraud-auth-service must
also be running** — this service verifies a receiver exists (and resolves
identities for the admin dashboard) by calling it directly, and every
route here needs a JWT that only fraud-auth-service issues. See
`docs/postman/fraud-transaction-service.postman_collection.json` for the
same flow with auto-chaining if you'd rather use Postman.

## 0. Setup — create a sender and receiver (via fraud-auth-service, port 8081)

```bash
curl -s -X POST http://localhost:8081/signup -H "Content-Type: application/json" \
  -d '{"name":"Sender One","email":"sender@example.com","password":"password123"}'
curl -s -X POST http://localhost:8081/signup -H "Content-Type: application/json" \
  -d '{"name":"Receiver One","email":"receiver@example.com","password":"password123"}'

SENDER_TOKEN=$(curl -s -X POST http://localhost:8081/login -H "Content-Type: application/json" \
  -d '{"email":"sender@example.com","password":"password123"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")
RECEIVER_ID=$(curl -s -X POST http://localhost:8081/login -H "Content-Type: application/json" \
  -d '{"email":"receiver@example.com","password":"password123"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['user']['id'])")
ADMIN_TOKEN=$(curl -s -X POST http://localhost:8081/login -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"<your AUTH_SEED_ADMIN_PASSWORD>"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")
```

## 1. Wallets

```bash
# Every wallet route acts on the CALLER's own account — no user_id field exists on any of these bodies.
curl -s -X POST http://localhost:8082/wallets -H "Authorization: Bearer $SENDER_TOKEN"

curl -s -X POST http://localhost:8082/wallets/topup \
  -H "Authorization: Bearer $SENDER_TOKEN" -H "Content-Type: application/json" \
  -d '{"amount":"1000.00"}'

curl -s http://localhost:8082/wallets/me -H "Authorization: Bearer $SENDER_TOKEN"
```

The receiver needs a wallet too before a transaction can credit it:

```bash
RECEIVER_TOKEN=$(curl -s -X POST http://localhost:8081/login -H "Content-Type: application/json" \
  -d '{"email":"receiver@example.com","password":"password123"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")
curl -s -X POST http://localhost:8082/wallets -H "Authorization: Bearer $RECEIVER_TOKEN"
```

## 2. Transactions

```bash
# 250000 is deliberately >200,000 AND a round multiple of 10,000 — if
# fraud-engine-service is running too, this trips both the large_amount and
# round_amount rules (score 50, "high" risk) and you'll see it land in that
# service's GET /scores and trigger a real fraud-alert email.
TXN_ID=$(curl -s -X POST http://localhost:8082/transactions \
  -H "Authorization: Bearer $SENDER_TOKEN" -H "Content-Type: application/json" \
  -d "{\"receiver_id\":\"$RECEIVER_ID\",\"amount\":\"250000\"}" | python3 -c "import sys,json;print(json.load(sys.stdin)['id'])")

# List — admin-only. status/search/user_ids are all optional and compose;
# search (txn ID substring) and user_ids (sender OR receiver match) are OR'd
# together as ONE search concept, while status is a separate AND filter.
curl -s "http://localhost:8082/transactions?limit=20&offset=0" -H "Authorization: Bearer $ADMIN_TOKEN"
curl -s "http://localhost:8082/transactions?status=completed" -H "Authorization: Bearer $ADMIN_TOKEN"
curl -s "http://localhost:8082/transactions?search=$TXN_ID" -H "Authorization: Bearer $ADMIN_TOKEN"

# Flag — admin-only, only eligible from status=completed. Publishes to the
# SAME "fraud-alerts" Kafka topic the automatic scorer uses.
curl -s -X POST http://localhost:8082/transactions/$TXN_ID/flag \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{"reason":"Sender wallet had a suspicious rapid top-up right before this transfer"}'

# Unflag — admin-only, only eligible from status=flagged. Sends a distinct
# "reviewed and cleared" email instead of a fraud-alert one.
curl -s -X POST http://localhost:8082/transactions/$TXN_ID/unflag \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{"reason":"Confirmed legitimate with the sender over a support call"}'
```

## 3. Admin wallet management

```bash
SENDER_ID=$(curl -s -X POST http://localhost:8081/login -H "Content-Type: application/json" \
  -d '{"email":"sender@example.com","password":"password123"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['user']['id'])")

# Bulk status for a whole page of user IDs in one call — what the admin
# dashboard's Users table calls, instead of one GET per row.
curl -s "http://localhost:8082/wallets?user_ids=$SENDER_ID,$RECEIVER_ID" -H "Authorization: Bearer $ADMIN_TOKEN"

# Disable — freezes it. Debit/Credit/AddBalance all reject it until re-enabled.
curl -s -X PATCH http://localhost:8082/wallets/$SENDER_ID \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{"is_enabled": false}'

# Confirm the freeze — this now 403s:
curl -s -w "\nHTTP %{http_code}\n" -X POST http://localhost:8082/wallets/topup \
  -H "Authorization: Bearer $SENDER_TOKEN" -H "Content-Type: application/json" -d '{"amount":"1"}'

# Re-enable
curl -s -X PATCH http://localhost:8082/wallets/$SENDER_ID \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{"is_enabled": true}'
```
