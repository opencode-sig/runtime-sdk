package kafka

import (
	"fmt"
	"strings"

	"github.com/opencode-sig/runtime-sdk/infra/internal/configutil"
)

const (
	CompressionNone   = "none"
	CompressionGZIP   = "gzip"
	CompressionSnappy = "snappy"
	CompressionLZ4    = "lz4"
	CompressionZSTD   = "zstd"

	SASLMechanismPlain       = "plain"
	SASLMechanismSCRAMSHA256 = "scram-sha-256"
	SASLMechanismSCRAMSHA512 = "scram-sha-512"

	defaultClientID          = "app"
	defaultDialTimeout       = "3s"
	defaultRequestTimeout    = "5s"
	defaultNumPartitions     = 3
	defaultReplicationFactor = 1
	defaultRequiredAcks      = -1
	defaultMaxAttempts       = 3
	defaultCompression       = CompressionSnappy
	defaultBatchTimeout      = "100ms"
	defaultStartOffset       = -1
	defaultProcessMaxRetries = 3
	defaultProcessBackoff    = "200ms"
)

// Config describes Kafka connection, topic, producer, and consumer configuration.
//
// It only carries infrastructure configuration. NewProducer and NewConsumer
// create clients without forcing connectivity.
type Config struct {
	Brokers        []string       `json:"brokers" yaml:"brokers"`
	ClientID       string         `json:"client_id" yaml:"client_id"`
	DialTimeout    string         `json:"dial_timeout,omitempty" yaml:"dial_timeout,omitempty"`
	RequestTimeout string         `json:"request_timeout,omitempty" yaml:"request_timeout,omitempty"`
	Topic          TopicConfig    `json:"topic" yaml:"topic"`
	Producer       ProducerConfig `json:"producer" yaml:"producer"`
	Consumer       ConsumerConfig `json:"consumer" yaml:"consumer"`
	TLS            TLSConfig      `json:"tls,omitempty" yaml:"tls,omitempty"`
	SASL           SASLConfig     `json:"sasl,omitempty" yaml:"sasl,omitempty"`
}

// TopicConfig describes topic auto-creation policy.
type TopicConfig struct {
	AutoCreate        bool `json:"auto_create" yaml:"auto_create"`
	NumPartitions     int  `json:"num_partitions" yaml:"num_partitions"`
	ReplicationFactor int  `json:"replication_factor" yaml:"replication_factor"`
}

// ProducerConfig describes default Kafka producer behavior.
type ProducerConfig struct {
	Async        bool   `json:"async" yaml:"async"`
	RequiredAcks int    `json:"required_acks" yaml:"required_acks"`
	MaxAttempts  int    `json:"max_attempts" yaml:"max_attempts"`
	Compression  string `json:"compression" yaml:"compression"`
	BatchSize    int    `json:"batch_size,omitempty" yaml:"batch_size,omitempty"`
	BatchTimeout string `json:"batch_timeout,omitempty" yaml:"batch_timeout,omitempty"`
}

// ConsumerConfig describes default Kafka consumer behavior.
type ConsumerConfig struct {
	GroupID     string        `json:"group_id,omitempty" yaml:"group_id,omitempty"`
	StartOffset int64         `json:"start_offset" yaml:"start_offset"`
	Process     ProcessConfig `json:"process" yaml:"process"`
}

// ProcessConfig describes retry and fallback policy for one message.
type ProcessConfig struct {
	MaxRetries      int    `json:"max_retries" yaml:"max_retries"`
	RetryBackoff    string `json:"retry_backoff" yaml:"retry_backoff"`
	DLQTopic        string `json:"dlq_topic" yaml:"dlq_topic"`
	CommitOnFailure bool   `json:"commit_on_failure" yaml:"commit_on_failure"`
}

// TLSConfig describes Kafka TLS connection settings.
type TLSConfig struct {
	Enabled            bool   `json:"enabled" yaml:"enabled"`
	ServerName         string `json:"server_name,omitempty" yaml:"server_name,omitempty"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty" yaml:"insecure_skip_verify,omitempty"`
}

// SASLConfig describes Kafka SASL authentication settings.
type SASLConfig struct {
	Enabled   bool   `json:"enabled" yaml:"enabled"`
	Mechanism string `json:"mechanism,omitempty" yaml:"mechanism,omitempty"`
	Username  string `json:"username,omitempty" yaml:"username,omitempty"`
	Password  string `json:"password,omitempty" yaml:"password,omitempty"`
}

// IsZero reports whether Kafka config is unset.
func (c Config) IsZero() bool {
	return len(c.Brokers) == 0 &&
		c.ClientID == "" &&
		c.DialTimeout == "" &&
		c.RequestTimeout == "" &&
		c.Topic.IsZero() &&
		c.Producer.IsZero() &&
		c.Consumer.IsZero() &&
		!c.TLS.Enabled &&
		!c.SASL.Enabled
}

// IsZero reports whether topic config is unset.
func (c TopicConfig) IsZero() bool {
	return !c.AutoCreate && c.NumPartitions == 0 && c.ReplicationFactor == 0
}

// IsZero reports whether producer config is unset.
func (c ProducerConfig) IsZero() bool {
	return !c.Async &&
		c.RequiredAcks == 0 &&
		c.MaxAttempts == 0 &&
		c.Compression == "" &&
		c.BatchSize == 0 &&
		c.BatchTimeout == ""
}

// IsZero reports whether consumer config is unset.
func (c ConsumerConfig) IsZero() bool {
	return c.GroupID == "" && c.StartOffset == 0 && c.Process.IsZero()
}

// IsZero reports whether process config is unset.
func (c ProcessConfig) IsZero() bool {
	return c.MaxRetries == 0 &&
		c.RetryBackoff == "" &&
		c.DLQTopic == "" &&
		!c.CommitOnFailure
}

// Normalize returns a Kafka config copy with defaults applied.
func (c Config) Normalize() Config {
	if c.ClientID == "" {
		c.ClientID = defaultClientID
	}
	if c.DialTimeout == "" {
		c.DialTimeout = defaultDialTimeout
	}
	if c.RequestTimeout == "" {
		c.RequestTimeout = defaultRequestTimeout
	}
	c.Topic = c.Topic.Normalize()
	c.Producer = c.Producer.Normalize()
	c.Consumer = c.Consumer.Normalize()
	if c.SASL.Enabled && c.SASL.Mechanism == "" {
		c.SASL.Mechanism = SASLMechanismPlain
	}
	return c
}

// Normalize returns a topic config copy with defaults applied.
func (c TopicConfig) Normalize() TopicConfig {
	if c.NumPartitions == 0 {
		c.NumPartitions = defaultNumPartitions
	}
	if c.ReplicationFactor == 0 {
		c.ReplicationFactor = defaultReplicationFactor
	}
	return c
}

// Normalize returns a producer config copy with defaults applied.
func (c ProducerConfig) Normalize() ProducerConfig {
	if c.RequiredAcks == 0 {
		c.RequiredAcks = defaultRequiredAcks
	}
	if c.MaxAttempts == 0 {
		c.MaxAttempts = defaultMaxAttempts
	}
	if c.Compression == "" {
		c.Compression = defaultCompression
	}
	if c.BatchTimeout == "" {
		c.BatchTimeout = defaultBatchTimeout
	}
	return c
}

// Normalize returns a consumer config copy with defaults applied.
func (c ConsumerConfig) Normalize() ConsumerConfig {
	if c.StartOffset == 0 {
		c.StartOffset = defaultStartOffset
	}
	c.Process = c.Process.Normalize()
	return c
}

// Normalize returns a process config copy with defaults applied.
func (c ProcessConfig) Normalize() ProcessConfig {
	if c.MaxRetries == 0 {
		c.MaxRetries = defaultProcessMaxRetries
	}
	if c.RetryBackoff == "" {
		c.RetryBackoff = defaultProcessBackoff
	}
	return c
}

// Validate checks whether Kafka config satisfies production baseline constraints.
func (c Config) Validate() error {
	if c.IsZero() {
		return nil
	}
	c = c.Normalize()
	if len(c.Brokers) == 0 {
		return fmt.Errorf("kafka brokers is required")
	}
	if strings.TrimSpace(c.ClientID) == "" {
		return fmt.Errorf("kafka client_id is required")
	}
	if _, err := configutil.PositiveDuration("kafka dial_timeout", c.DialTimeout); err != nil {
		return err
	}
	if _, err := configutil.PositiveDuration("kafka request_timeout", c.RequestTimeout); err != nil {
		return err
	}
	if err := c.Topic.Validate(); err != nil {
		return err
	}
	if err := c.Producer.Validate(); err != nil {
		return err
	}
	if err := c.Consumer.Validate(); err != nil {
		return err
	}
	if err := c.SASL.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate checks topic config.
func (c TopicConfig) Validate() error {
	c = c.Normalize()
	if c.NumPartitions <= 0 {
		return fmt.Errorf("kafka topic num_partitions must be positive")
	}
	if c.ReplicationFactor <= 0 {
		return fmt.Errorf("kafka topic replication_factor must be positive")
	}
	return nil
}

// Validate checks producer config.
func (c ProducerConfig) Validate() error {
	c = c.Normalize()
	if c.MaxAttempts <= 0 {
		return fmt.Errorf("kafka producer max_attempts must be positive")
	}
	switch strings.ToLower(strings.TrimSpace(c.Compression)) {
	case CompressionNone, CompressionGZIP, CompressionSnappy, CompressionLZ4, CompressionZSTD:
	default:
		return fmt.Errorf("unsupported kafka producer compression %q", c.Compression)
	}
	if c.BatchSize < 0 {
		return fmt.Errorf("kafka producer batch_size must be non-negative")
	}
	if _, err := configutil.PositiveDuration("kafka producer batch_timeout", c.BatchTimeout); err != nil {
		return err
	}
	return nil
}

// Validate checks consumer config.
func (c ConsumerConfig) Validate() error {
	c = c.Normalize()
	return c.Process.Validate()
}

// Validate checks process config.
func (c ProcessConfig) Validate() error {
	c = c.Normalize()
	if c.MaxRetries < 0 {
		return fmt.Errorf("kafka consumer process max_retries must be non-negative")
	}
	if _, err := configutil.PositiveDuration("kafka consumer process retry_backoff", c.RetryBackoff); err != nil {
		return err
	}
	return nil
}

// Validate checks SASL authentication config.
func (c SASLConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(c.Mechanism)) {
	case SASLMechanismPlain, SASLMechanismSCRAMSHA256, SASLMechanismSCRAMSHA512:
	default:
		return fmt.Errorf("unsupported kafka sasl mechanism %q", c.Mechanism)
	}
	if c.Username == "" || c.Password == "" {
		return fmt.Errorf("kafka sasl username and password are required")
	}
	return nil
}
