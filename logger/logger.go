package logger

import (
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	defaultLogDir     = "./logs"
	defaultFilePrefix = "runtime"
	defaultMaxAgeDays = 7
)

type Config struct {
	ServiceName     string `json:"service_name" yaml:"service_name"`
	FilePrefix      string `json:"file_prefix" yaml:"file_prefix"`
	Level           string `json:"level" yaml:"level"`
	StacktraceLevel string `json:"stacktrace_level" yaml:"stacktrace_level"`
	Format          string `json:"format" yaml:"format"`
	EnableStdout    bool   `json:"enable_stdout" yaml:"enable_stdout"`
	EnableFile      bool   `json:"enable_file" yaml:"enable_file"`
	Caller          bool   `json:"caller" yaml:"caller"`
	LogDir          string `json:"log_dir" yaml:"log_dir"`
	MaxAgeDays      int    `json:"max_age_days" yaml:"max_age_days"`
	FileZone        string `json:"file_zone" yaml:"file_zone"`
}

// New creates a zap logger with the default production configuration.
//
// It is primarily intended for early startup logs before full config is loaded.
func New(service string) (*zap.Logger, error) {
	return NewWithConfig(Config{
		ServiceName:     service,
		FilePrefix:      service,
		Level:           "info",
		StacktraceLevel: "error",
		Format:          "json",
		EnableStdout:    true,
		Caller:          true,
	})
}

// NewWithConfig creates a zap logger from SDK logger config.
//
// It supports stdout, file output, JSON/console encoding, caller, and
// stacktrace level configuration.
func NewWithConfig(config Config) (*zap.Logger, error) {
	level, err := parseLevel(config.Level, zapcore.InfoLevel)
	if err != nil {
		return nil, fmt.Errorf("logger level: %w", err)
	}
	stacktraceLevel, err := parseLevel(config.StacktraceLevel, zapcore.ErrorLevel)
	if err != nil {
		return nil, fmt.Errorf("logger stacktrace level: %w", err)
	}

	encoding := strings.ToLower(strings.TrimSpace(config.Format))
	if encoding == "" {
		encoding = "json"
	}
	if encoding != "json" && encoding != "console" {
		return nil, fmt.Errorf("unsupported logger format %q", config.Format)
	}

	core, err := newCore(config, encoding, level)
	if err != nil {
		return nil, err
	}
	options := []zap.Option{
		zap.AddStacktrace(stacktraceLevel),
		zap.ErrorOutput(zapcore.Lock(os.Stderr)),
		zap.Fields(zap.String("service", firstNonEmpty(config.ServiceName, "app"))),
	}
	if config.Caller {
		options = append(options, zap.AddCaller())
	}
	return zap.New(core, options...), nil
}

// newCore creates a zapcore.Core from stdout and file output settings.
func newCore(config Config, encoding string, level zapcore.Level) (zapcore.Core, error) {
	writers := make([]zapcore.WriteSyncer, 0, 2)
	if config.EnableStdout {
		writers = append(writers, zapcore.Lock(os.Stdout))
	}
	if config.EnableFile {
		location, err := loadLocation(config.FileZone)
		if err != nil {
			return nil, err
		}
		writer, err := newDailyWriter(dailyWriterConfig{
			Dir:        firstNonEmpty(config.LogDir, defaultLogDir),
			Prefix:     firstNonEmpty(config.FilePrefix, config.ServiceName, defaultFilePrefix),
			Location:   location,
			MaxAgeDays: normalizeMaxAgeDays(config.MaxAgeDays),
			Now:        time.Now,
		})
		if err != nil {
			return nil, err
		}
		writers = append(writers, writer)
	}
	if len(writers) == 0 {
		writers = append(writers, zapcore.Lock(os.Stdout))
	}
	return zapcore.NewCore(newEncoder(encoding), zapcore.NewMultiWriteSyncer(writers...), level), nil
}

// newEncoder creates the shared zap encoder.
func newEncoder(encoding string) zapcore.Encoder {
	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncodeDuration = zapcore.StringDurationEncoder
	cfg.EncodeLevel = zapcore.LowercaseLevelEncoder
	cfg.EncodeCaller = zapcore.ShortCallerEncoder
	if encoding == "console" {
		return zapcore.NewConsoleEncoder(cfg)
	}
	return zapcore.NewJSONEncoder(cfg)
}

// loadLocation loads the time zone used by daily file rotation.
func loadLocation(value string) (*time.Location, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "local") {
		return time.Local, nil
	}
	location, err := time.LoadLocation(value)
	if err != nil {
		return nil, fmt.Errorf("logger file_zone: %w", err)
	}
	return location, nil
}

// normalizeMaxAgeDays returns the log file retention window in days.
func normalizeMaxAgeDays(value int) int {
	if value <= 0 {
		return defaultMaxAgeDays
	}
	return value
}

// parseLevel parses a zap level and uses fallback for empty values.
func parseLevel(value string, fallback zapcore.Level) (zapcore.Level, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(strings.ToLower(value))); err != nil {
		return fallback, err
	}
	return level, nil
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
