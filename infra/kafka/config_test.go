package kafka

import (
	"context"
	"testing"
)

func TestConfigValidateAllowsZeroConfig(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatalf("validate zero config: %v", err)
	}
}

func TestConfigValidateAllowsZeroOptionalValues(t *testing.T) {
	cfg := Config{
		Brokers: []string{"127.0.0.1:9092"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestConfigNormalizeDefaultsOptionalValues(t *testing.T) {
	cfg := Config{
		Brokers: []string{"127.0.0.1:9092"},
	}

	normalized := cfg.Normalize()
	if normalized.ClientID != "app" {
		t.Fatalf("client id = %q", normalized.ClientID)
	}
	if normalized.DialTimeout != "3s" {
		t.Fatalf("dial timeout = %q", normalized.DialTimeout)
	}
	if normalized.RequestTimeout != "5s" {
		t.Fatalf("request timeout = %q", normalized.RequestTimeout)
	}
	if normalized.Topic.NumPartitions != 3 {
		t.Fatalf("topic num partitions = %d", normalized.Topic.NumPartitions)
	}
	if normalized.Topic.ReplicationFactor != 1 {
		t.Fatalf("topic replication factor = %d", normalized.Topic.ReplicationFactor)
	}
	if normalized.Producer.RequiredAcks != -1 {
		t.Fatalf("producer required acks = %d", normalized.Producer.RequiredAcks)
	}
	if normalized.Producer.MaxAttempts != 3 {
		t.Fatalf("producer max attempts = %d", normalized.Producer.MaxAttempts)
	}
	if normalized.Producer.Compression != CompressionSnappy {
		t.Fatalf("producer compression = %q", normalized.Producer.Compression)
	}
	if normalized.Producer.BatchTimeout != "100ms" {
		t.Fatalf("producer batch timeout = %q", normalized.Producer.BatchTimeout)
	}
	if normalized.Consumer.StartOffset != -1 {
		t.Fatalf("consumer start offset = %d", normalized.Consumer.StartOffset)
	}
	if normalized.Consumer.Process.MaxRetries != 3 {
		t.Fatalf("consumer process max retries = %d", normalized.Consumer.Process.MaxRetries)
	}
	if normalized.Consumer.Process.RetryBackoff != "200ms" {
		t.Fatalf("consumer process retry backoff = %q", normalized.Consumer.Process.RetryBackoff)
	}
}

func TestConfigValidateRejectsInvalidCompression(t *testing.T) {
	cfg := Config{
		Brokers: []string{"127.0.0.1:9092"},
		Producer: ProducerConfig{
			Compression: "zip",
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid compression error")
	}
}

func TestConfigValidateRejectsNegativeBatchSize(t *testing.T) {
	cfg := Config{
		Brokers: []string{"127.0.0.1:9092"},
		Producer: ProducerConfig{
			BatchSize: -1,
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected negative batch size error")
	}
}

func TestConfigValidateRejectsSASLMissingCredentials(t *testing.T) {
	cfg := Config{
		Brokers: []string{"127.0.0.1:9092"},
		SASL: SASLConfig{
			Enabled:   true,
			Mechanism: SASLMechanismPlain,
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing SASL credentials error")
	}
}

func TestConfigValidateRejectsInvalidRetryBackoff(t *testing.T) {
	cfg := Config{
		Brokers: []string{"127.0.0.1:9092"},
		Consumer: ConsumerConfig{
			Process: ProcessConfig{
				RetryBackoff: "bad",
			},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid retry backoff error")
	}
}

func TestConfigValidateRejectsInvalidDialTimeout(t *testing.T) {
	cfg := Config{
		Brokers:     []string{"127.0.0.1:9092"},
		DialTimeout: "bad",
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid dial timeout error")
	}
}

func TestNewProducerRejectsZeroConfig(t *testing.T) {
	if _, err := NewProducer(Config{}); err == nil {
		t.Fatal("expected missing kafka brokers error")
	}
}

func TestNewProducerMapsConfig(t *testing.T) {
	producer, err := NewProducer(Config{
		Brokers: []string{"127.0.0.1:9092"},
		Producer: ProducerConfig{
			RequiredAcks: 1,
			MaxAttempts:  2,
			Compression:  CompressionGZIP,
			BatchSize:    10,
		},
	})
	if err != nil {
		t.Fatalf("new producer: %v", err)
	}
	defer producer.Close()

	if producer.writer.MaxAttempts != 2 {
		t.Fatalf("max attempts = %d", producer.writer.MaxAttempts)
	}
	if int(producer.writer.RequiredAcks) != 1 {
		t.Fatalf("required acks = %d", producer.writer.RequiredAcks)
	}
	if producer.writer.BatchSize != 10 {
		t.Fatalf("batch size = %d", producer.writer.BatchSize)
	}
}

func TestNewConsumerRejectsMissingTopic(t *testing.T) {
	_, err := NewConsumer(Config{Brokers: []string{"127.0.0.1:9092"}}, "", "")
	if err == nil {
		t.Fatal("expected missing topic error")
	}
}

func TestCheckRejectsZeroConfig(t *testing.T) {
	if err := Check(context.Background(), Config{}); err == nil {
		t.Fatal("expected missing kafka brokers error")
	}
}
