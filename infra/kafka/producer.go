package kafka

import (
	"context"
	"fmt"

	"github.com/opencode-sig/runtime-sdk/infra/internal/configutil"

	kafkago "github.com/segmentio/kafka-go"
)

// Producer wraps kafka-go Writer with a stable infra boundary.
type Producer struct {
	writer *kafkago.Writer
}

// NewProducer creates a Kafka producer.
//
// The writer is topic-less by default. WriteMessages sets the provided topic on
// messages that do not already specify one.
func NewProducer(cfg Config) (*Producer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg = cfg.Normalize()
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers is required")
	}

	transport, err := newTransport(cfg)
	if err != nil {
		return nil, err
	}
	return &Producer{writer: &kafkago.Writer{
		Addr:         kafkago.TCP(cfg.Brokers...),
		Balancer:     &kafkago.LeastBytes{},
		MaxAttempts:  cfg.Producer.MaxAttempts,
		BatchSize:    cfg.Producer.BatchSize,
		BatchTimeout: configutil.MustDuration(cfg.Producer.BatchTimeout),
		ReadTimeout:  configutil.MustDuration(cfg.RequestTimeout),
		WriteTimeout: configutil.MustDuration(cfg.RequestTimeout),
		RequiredAcks: kafkago.RequiredAcks(cfg.Producer.RequiredAcks),
		Async:        cfg.Producer.Async,
		Compression:  kafkaCompression(cfg.Producer.Compression),
		Transport:    transport,
	}}, nil
}

// WriteMessages writes messages to topic.
func (p *Producer) WriteMessages(ctx context.Context, topic string, messages ...kafkago.Message) error {
	if p == nil || p.writer == nil || len(messages) == 0 {
		return nil
	}
	for i := range messages {
		if messages[i].Topic == "" {
			messages[i].Topic = topic
		}
	}
	return p.writer.WriteMessages(ctx, messages...)
}

// Close closes the producer.
func (p *Producer) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
