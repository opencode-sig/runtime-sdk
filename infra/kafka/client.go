package kafka

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"

	"github.com/opencode-sig/runtime-sdk/infra/internal/configutil"

	kafkago "github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

func newDialer(cfg Config) (*kafkago.Dialer, error) {
	mechanism, err := newSASLMechanism(cfg.SASL)
	if err != nil {
		return nil, err
	}
	return &kafkago.Dialer{
		ClientID:      cfg.ClientID,
		Timeout:       configutil.MustDuration(cfg.DialTimeout),
		DualStack:     true,
		TLS:           newTLSConfig(cfg.TLS),
		SASLMechanism: mechanism,
	}, nil
}

func newTransport(cfg Config) (*kafkago.Transport, error) {
	mechanism, err := newSASLMechanism(cfg.SASL)
	if err != nil {
		return nil, err
	}
	return &kafkago.Transport{
		Dial: (&net.Dialer{
			Timeout:   configutil.MustDuration(cfg.DialTimeout),
			DualStack: true,
		}).DialContext,
		DialTimeout: configutil.MustDuration(cfg.DialTimeout),
		ClientID:    cfg.ClientID,
		TLS:         newTLSConfig(cfg.TLS),
		SASL:        mechanism,
	}, nil
}

func newTLSConfig(cfg TLSConfig) *tls.Config {
	if !cfg.Enabled {
		return nil
	}
	return &tls.Config{
		ServerName:         cfg.ServerName,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}
}

func newSASLMechanism(cfg SASLConfig) (sasl.Mechanism, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Mechanism)) {
	case SASLMechanismPlain:
		return plain.Mechanism{Username: cfg.Username, Password: cfg.Password}, nil
	case SASLMechanismSCRAMSHA256:
		return scram.Mechanism(scram.SHA256, cfg.Username, cfg.Password)
	case SASLMechanismSCRAMSHA512:
		return scram.Mechanism(scram.SHA512, cfg.Username, cfg.Password)
	default:
		return nil, fmt.Errorf("unsupported kafka sasl mechanism %q", cfg.Mechanism)
	}
}

func kafkaCompression(value string) kafkago.Compression {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CompressionGZIP:
		return kafkago.Gzip
	case CompressionSnappy:
		return kafkago.Snappy
	case CompressionLZ4:
		return kafkago.Lz4
	case CompressionZSTD:
		return kafkago.Zstd
	default:
		return 0
	}
}

// Check verifies Kafka connectivity by dialing the first configured broker.
func Check(ctx context.Context, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	cfg = cfg.Normalize()
	if len(cfg.Brokers) == 0 {
		return fmt.Errorf("kafka brokers is required")
	}
	dialer, err := newDialer(cfg)
	if err != nil {
		return err
	}
	conn, err := dialer.DialContext(ctx, "tcp", cfg.Brokers[0])
	if err != nil {
		return err
	}
	return conn.Close()
}
