package elastic

import (
	"fmt"
	"strings"

	"github.com/opencode-sig/runtime-sdk/infra/internal/configutil"
)

const (
	defaultDialTimeout        = "3s"
	defaultRequestTimeout     = "5s"
	defaultMaxRetries         = 3
	defaultRetryBackoff       = "200ms"
	defaultSlowQueryThreshold = "500ms"
)

// Config describes Elasticsearch connection and request behavior.
//
// It only carries infrastructure configuration. Index naming, query DSL, and
// document schemas belong to service-owned application code.
type Config struct {
	Addresses      []string            `json:"addresses" yaml:"addresses"`
	Username       string              `json:"username,omitempty" yaml:"username,omitempty"`
	Password       string              `json:"password,omitempty" yaml:"password,omitempty"`
	CloudID        string              `json:"cloud_id,omitempty" yaml:"cloud_id,omitempty"`
	APIKey         string              `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	DialTimeout    string              `json:"dial_timeout,omitempty" yaml:"dial_timeout,omitempty"`
	RequestTimeout string              `json:"request_timeout,omitempty" yaml:"request_timeout,omitempty"`
	MaxRetries     int                 `json:"max_retries,omitempty" yaml:"max_retries,omitempty"`
	RetryBackoff   string              `json:"retry_backoff,omitempty" yaml:"retry_backoff,omitempty"`
	TLS            TLSConfig           `json:"tls,omitempty" yaml:"tls,omitempty"`
	Observability  ObservabilityConfig `json:"observability,omitempty" yaml:"observability,omitempty"`
}

// TLSConfig describes Elasticsearch TLS connection settings.
type TLSConfig struct {
	Enabled            bool   `json:"enabled" yaml:"enabled"`
	CAFile             string `json:"ca_file,omitempty" yaml:"ca_file,omitempty"`
	ServerName         string `json:"server_name,omitempty" yaml:"server_name,omitempty"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty" yaml:"insecure_skip_verify,omitempty"`
}

// ObservabilityConfig describes runtime thresholds used by callers for telemetry.
type ObservabilityConfig struct {
	SlowQueryThreshold string `json:"slow_query_threshold,omitempty" yaml:"slow_query_threshold,omitempty"`
}

// IsZero reports whether Elasticsearch config is unset.
func (c Config) IsZero() bool {
	return len(c.Addresses) == 0 &&
		c.Username == "" &&
		c.Password == "" &&
		c.CloudID == "" &&
		c.APIKey == "" &&
		c.DialTimeout == "" &&
		c.RequestTimeout == "" &&
		c.MaxRetries == 0 &&
		c.RetryBackoff == "" &&
		c.TLS.IsZero() &&
		c.Observability.IsZero()
}

// IsZero reports whether TLS config is unset.
func (c TLSConfig) IsZero() bool {
	return !c.Enabled &&
		c.CAFile == "" &&
		c.ServerName == "" &&
		!c.InsecureSkipVerify
}

// IsZero reports whether observability config is unset.
func (c ObservabilityConfig) IsZero() bool {
	return c.SlowQueryThreshold == ""
}

// Normalize returns an Elasticsearch config copy with defaults applied.
func (c Config) Normalize() Config {
	c.Addresses = cleanList(c.Addresses)
	if c.DialTimeout == "" {
		c.DialTimeout = defaultDialTimeout
	}
	if c.RequestTimeout == "" {
		c.RequestTimeout = defaultRequestTimeout
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = defaultMaxRetries
	}
	if c.RetryBackoff == "" {
		c.RetryBackoff = defaultRetryBackoff
	}
	c.Observability = c.Observability.Normalize()
	return c
}

// Normalize returns an observability config copy with defaults applied.
func (c ObservabilityConfig) Normalize() ObservabilityConfig {
	if c.SlowQueryThreshold == "" {
		c.SlowQueryThreshold = defaultSlowQueryThreshold
	}
	return c
}

// Validate checks whether Elasticsearch config satisfies production baseline constraints.
func (c Config) Validate() error {
	if c.IsZero() {
		return nil
	}
	c = c.Normalize()
	if len(c.Addresses) == 0 && strings.TrimSpace(c.CloudID) == "" {
		return fmt.Errorf("elastic addresses or cloud_id is required")
	}
	if strings.TrimSpace(c.APIKey) != "" && (strings.TrimSpace(c.Username) != "" || strings.TrimSpace(c.Password) != "") {
		return fmt.Errorf("elastic api_key must not be combined with username/password")
	}
	if strings.TrimSpace(c.Username) != "" && strings.TrimSpace(c.Password) == "" {
		return fmt.Errorf("elastic password is required when username is set")
	}
	if strings.TrimSpace(c.Password) != "" && strings.TrimSpace(c.Username) == "" {
		return fmt.Errorf("elastic username is required when password is set")
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("elastic max_retries must be non-negative")
	}
	if _, err := configutil.PositiveDuration("elastic dial_timeout", c.DialTimeout); err != nil {
		return err
	}
	if _, err := configutil.PositiveDuration("elastic request_timeout", c.RequestTimeout); err != nil {
		return err
	}
	if _, err := configutil.PositiveDuration("elastic retry_backoff", c.RetryBackoff); err != nil {
		return err
	}
	if _, err := configutil.PositiveDuration("elastic slow_query_threshold", c.Observability.SlowQueryThreshold); err != nil {
		return err
	}
	return nil
}

func cleanList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
