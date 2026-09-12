package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/abhay786-20/fraud-transaction-service/internal/models"
)

// OutboxRepository is a separate interface from TransactionRepository on
// purpose — it's used by the outbox worker (a background process reading
// outbox_events), a different responsibility from serving HTTP requests
// about transactions, even though both ultimately talk to the same
// database.
type OutboxRepository interface {
	GetUnpublished(ctx context.Context, limit int) ([]models.OutboxEvent, error)
	MarkPublished(ctx context.Context, id string) error
}

type postgresOutboxRepository struct {
	db  *sqlx.DB
	log *zap.Logger
}

func NewOutboxRepository(db *sqlx.DB, log *zap.Logger) OutboxRepository {
	return &postgresOutboxRepository{db: db, log: log}
}

func (r *postgresOutboxRepository) GetUnpublished(ctx context.Context, limit int) ([]models.OutboxEvent, error) {
	start := time.Now()
	const query = `
		SELECT * FROM outbox_events
		WHERE published_at IS NULL
		ORDER BY created_at ASC
		LIMIT $1`

	var events []models.OutboxEvent
	err := r.db.SelectContext(ctx, &events, query, limit)
	r.log.Debug("repository query",
		zap.String("op", "outbox.GetUnpublished"),
		zap.Duration("duration", time.Since(start)),
		zap.Error(err),
	)
	if err != nil {
		return nil, fmt.Errorf("getting unpublished events: %w", err)
	}
	return events, nil
}

func (r *postgresOutboxRepository) MarkPublished(ctx context.Context, id string) error {
	start := time.Now()
	const query = `UPDATE outbox_events SET published_at = now() WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, id)
	r.log.Debug("repository query",
		zap.String("op", "outbox.MarkPublished"),
		zap.Duration("duration", time.Since(start)),
		zap.Error(err),
	)
	if err != nil {
		return fmt.Errorf("marking event published: %w", err)
	}
	return nil
}
