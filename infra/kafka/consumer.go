package kafka

import (
	"context"
	"fmt"

	"github.com/opencode-sig/runtime-sdk/infra/internal/configutil"
	"strings"

	kafkago "github.com/segmentio/kafka-go"
)

// Consumer wraps kafka-go Reader with a stable infra boundary.
type Consumer struct {
	reader *kafkago.Reader
}

// NewConsumer creates a Kafka consumer for one topic.
func NewConsumer(cfg Config, topic string, groupID string) (*Consumer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg = cfg.Normalize()
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers is required")
	}
	if strings.TrimSpace(topic) == "" {
		return nil, fmt.Errorf("kafka consumer topic is required")
	}
	if groupID == "" {
		groupID = cfg.Consumer.GroupID
	}

	dialer, err := newDialer(cfg)
	if err != nil {
		return nil, err
	}
	return &Consumer{reader: kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:     cfg.Brokers,
		GroupID:     groupID,
		Topic:       topic,
		Dialer:      dialer,
		StartOffset: cfg.Consumer.StartOffset,
		MaxWait:     configutil.MustDuration(cfg.RequestTimeout),
	})}, nil
}

// FetchMessage fetches one Kafka message.
func (c *Consumer) FetchMessage(ctx context.Context) (kafkago.Message, error) {
	if c == nil || c.reader == nil {
		return kafkago.Message{}, nil
	}
	return c.reader.FetchMessage(ctx)
}

// CommitMessages commits consumed messages.
func (c *Consumer) CommitMessages(ctx context.Context, messages ...kafkago.Message) error {
	if c == nil || c.reader == nil || len(messages) == 0 {
		return nil
	}
	return c.reader.CommitMessages(ctx, messages...)
}

// Close closes the consumer.
func (c *Consumer) Close() error {
	if c == nil || c.reader == nil {
		return nil
	}
	return c.reader.Close()
}
