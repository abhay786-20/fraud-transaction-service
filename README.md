# fraud-transaction-service

Handles transaction creation and reliable event publishing (Outbox
Pattern + Kafka) for the fraud detection platform.

Under active development — this README will be filled in properly once
the core vertical slice is built and tested, same as `fraud-auth-service`.

## Known v1 scope boundaries (deliberate, not overlooked)

Tracked in full in `fraud-platform-infra/docs/coding-plan.md` §5:

- **`receiver_id` is not verified to exist** — no foreign key is possible
  across service boundaries (different physical database from
  `fraud-auth-service`'s `users` table), and the real fix (a
  service-to-service call to verify it) isn't built yet.
- **No balance/wallet concept** — a transaction is recorded regardless of
  whether the sender could plausibly afford it.
- `sender_id` always comes from the caller's validated JWT, **never** the
  request body — this one's already enforced, not deferred.
