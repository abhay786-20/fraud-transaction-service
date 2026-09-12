package models

import "time"

// OutboxEvent mirrors a row in the outbox_events table — the actual
// Outbox Pattern mechanism. PublishedAt is a pointer because it's
// nullable: nil means "not yet published to Kafka."
type OutboxEvent struct {
	ID            string     `db:"id"`
	AggregateType string     `db:"aggregate_type"`
	AggregateID   string     `db:"aggregate_id"`
	EventType     string     `db:"event_type"`
	Payload       []byte     `db:"payload"` // raw JSON bytes — see note in repository
	PublishedAt   *time.Time `db:"published_at"`
	CreatedAt     time.Time  `db:"created_at"`
}
