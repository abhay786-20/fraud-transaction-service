# fraud-transaction-service

Handles transaction creation, wallet balances, and reliable event
publishing (Outbox Pattern + Kafka) for the fraud detection platform.

## What it actually does today

- Create a transaction — `sender_id` always comes from the caller's
  validated JWT, **never** the request body (same rule as everywhere else
  in this platform); `receiver_id` is verified to be a real, active
  account via a real-time service-to-service call to `fraud-auth-service`
  before anything is written
- Wallets — explicit create/top-up/view API (no auto-provisioning on
  signup); a transaction atomically debits the sender and credits the
  receiver, race-free (a single conditional `UPDATE` enforces sufficient
  balance, not a separate check-then-write), all in the same database
  transaction as the transaction row and outbox event — one atomic unit,
  or none of it happens
- Outbox Pattern, both halves: atomic storage (transaction + wallet
  writes + outbox event, one DB transaction) and reliable delivery (a
  background worker polls unpublished events every 2s and publishes them
  to Kafka, partitioned by `sender_id` so one user's events stay ordered)
- Graceful shutdown — real `SIGTERM`/`SIGINT` handling; the outbox worker
  and HTTP server both stop cleanly, never killed mid-publish
- `/health` (liveness) and `/ready` (readiness — pings Postgres)

## Tech stack

Go · Gin · `sqlx` + `lib/pq` (PostgreSQL) · `segmentio/kafka-go` ·
`golang-jwt/jwt/v5` (verification only — this service never issues
tokens) · `google/uuid` · `zap` · `godotenv` · `golang-migrate` (via
Docker, not a local install) · `air` (hot reload, dev only)

## Project layout

```
cmd/                    entrypoint — wiring, graceful shutdown
internal/
  authclient/            HTTP client for fraud-auth-service's internal API
  config/                env var loading + validation
  db/                    Postgres connection pool
  dto/                   HTTP request/response shapes
  handler/               HTTP layer — Gin handlers
  middleware/            JWT validation (verify-only)
  models/                structs mirroring the database
  repository/            the only layer that runs SQL
  router/                route registration
  service/               business logic
  worker/                the outbox worker background process
migrations/             versioned schema changes (golang-migrate)
pkg/
  constants/             env var key names
  env/                   generic env var parsing helpers
  kafka/                 producer wrapper (segmentio/kafka-go)
  logger/                zap logger construction
  token/                 JWT verification (Claims + Parse only)
```

## API

| Method | Path | Auth | Notes |
|---|---|---|---|
| `GET` | `/health` | none | liveness |
| `GET` | `/ready` | none | readiness — pings Postgres |
| `POST` | `/transactions` | JWT | `sender_id` from JWT; debits sender, credits receiver, atomically |
| `POST` | `/wallets` | JWT | creates a wallet for the caller, balance `0` |
| `GET` | `/wallets/me` | JWT | view the caller's own balance |
| `POST` | `/wallets/topup` | JWT | add funds to the caller's own wallet |

## Cross-service dependencies

- **Verifies JWTs issued by `fraud-auth-service`**, using a secret that
  must match `AUTH_JWT_SECRET` there exactly (`TRANSACTION_JWT_SECRET`
  here — shared secret, different env var name, kept in sync manually).
- **Calls `fraud-auth-service`'s `/internal/*` API** to verify a
  transaction's receiver is real and active, authenticated with a
  separate shared key (`TRANSACTION_INTERNAL_API_KEY` here, must match
  `AUTH_INTERNAL_API_KEY` there).

Both are real synchronous dependencies — this service cannot create a
transaction if `fraud-auth-service` is unreachable, a deliberate trade-off
(see `fraud-platform-infra/docs/coding-plan.md` §5) in exchange for never
accepting an unverified receiver.

## Running it locally

1. Bring up shared infra (Postgres + Kafka, from `fraud-platform-infra`):
   ```bash
   cd ../fraud-platform-infra && make postgres && make kafka
   ```
2. Copy `.env.example` to `.env` and fill in real values — `TRANSACTION_JWT_SECRET`
   and `TRANSACTION_INTERNAL_API_KEY` must match `fraud-auth-service`'s
   `.env` exactly.
3. Apply migrations:
   ```bash
   make migrate-up
   ```
4. Run with hot reload:
   ```bash
   air
   ```
5. `fraud-auth-service` must also be running for receiver verification and
   JWT issuance to work.

## Testing

```bash
go test ./... -v
```
`internal/service` has table-driven tests against fake, in-memory
`TransactionRepository`, `WalletRepository`, and `UserVerifier` — no real
database, Kafka, or network call needed to run them.

## Building the production image

```bash
docker build -t fraud-transaction-service .
```
Multi-stage build — the final image is just the compiled binary on
`alpine`, not the Go toolchain (~42MB total).

## Database

Owns its own dedicated PostgreSQL database, `fraud_transaction_db` —
tables: `transactions`, `outbox_events`, `wallets`. Isolated from every
other service's database at the engine level (see
`fraud-platform-infra`'s `create-multiple-dbs.sh`).

## Not built yet

- Asymmetric JWT signing (RS256) — the shared-secret approach works but
  means any service holding the secret could also forge tokens, not just
  verify them
- Rate limiting
- Admin-facing endpoints to list/search transactions (the admin dashboard
  will need these)
