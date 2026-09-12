// Package kafka wraps segmentio/kafka-go, so the rest of the codebase
// depends on this package, not on the third-party library directly.
package kafka

import (
	"context"

	"github.com/segmentio/kafka-go"
)

// Producer publishes messages to a single Kafka topic.
type Producer struct {
	writer *kafka.Writer
}

// NewProducer builds a producer for the given topic, connecting to the
// given broker address(es).
func NewProducer(brokers []string, topic string) *Producer {
	return &Producer{
		writer: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        topic,
			Balancer:     &kafka.Hash{}, // same key -> same partition, always
			RequiredAcks: kafka.RequireAll,
		},
	}
}

// Publish sends one message, keyed by key — e.g. the sender's user_id, so
// all of one user's events land on the same partition and stay ordered
// relative to each other.
func (p *Producer) Publish(ctx context.Context, key string, value []byte) error {
	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: value,
	})
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
