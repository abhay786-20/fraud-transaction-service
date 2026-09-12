// Package worker holds background processes — currently just the outbox
// worker — as opposed to internal/handler, which only ever runs in
// response to an HTTP request.
package worker

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	"github.com/abhay786-20/fraud-transaction-service/internal/repository"
	"github.com/abhay786-20/fraud-transaction-service/pkg/kafka"
)

// OutboxWorker polls outbox_events for unpublished rows and publishes
// them to Kafka, marking each one published only after Kafka confirms
// the write.
type OutboxWorker struct {
	outboxRepo repository.OutboxRepository
	producer   *kafka.Producer
	log        *zap.Logger
	interval   time.Duration
	batchSize  int
}

func NewOutboxWorker(outboxRepo repository.OutboxRepository, producer *kafka.Producer, log *zap.Logger) *OutboxWorker {
	return &OutboxWorker{
		outboxRepo: outboxRepo,
		producer:   producer,
		log:        log,
		interval:   2 * time.Second,
		batchSize:  10,
	}
}

// Run polls forever until ctx is cancelled. Meant to be started in its
// own goroutine once, at startup — see cmd/main.go.
func (w *OutboxWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.log.Info("outbox worker started", zap.Duration("interval", w.interval))

	for {
		select {
		case <-ctx.Done():
			w.log.Info("outbox worker stopping")
			return
		case <-ticker.C:
			w.publishPending(ctx)
		}
	}
}

func (w *OutboxWorker) publishPending(ctx context.Context) {
	events, err := w.outboxRepo.GetUnpublished(ctx, w.batchSize)
	if err != nil {
		w.log.Error("outbox worker: fetching unpublished events failed", zap.Error(err))
		return
	}

	for _, event := range events {
		// Only decoding enough of the payload to get the partition key —
		// the worker doesn't need to understand the rest of the event's
		// shape, it just forwards event.Payload as-is to Kafka.
		var partial struct {
			SenderID string `json:"sender_id"`
		}
		if err := json.Unmarshal(event.Payload, &partial); err != nil {
			w.log.Error("outbox worker: decoding payload failed",
				zap.String("event_id", event.ID), zap.Error(err))
			continue // one bad row shouldn't block the rest of the batch
		}

		if err := w.producer.Publish(ctx, partial.SenderID, event.Payload); err != nil {
			w.log.Error("outbox worker: publishing to kafka failed",
				zap.String("event_id", event.ID), zap.Error(err))
			continue // stays unpublished; picked up again next tick
		}

		if err := w.outboxRepo.MarkPublished(ctx, event.ID); err != nil {
			w.log.Error("outbox worker: marking event published failed",
				zap.String("event_id", event.ID), zap.Error(err))
			continue
		}

		w.log.Info("outbox event published",
			zap.String("event_id", event.ID),
			zap.String("event_type", event.EventType),
			zap.String("aggregate_id", event.AggregateID),
		)
	}
}
