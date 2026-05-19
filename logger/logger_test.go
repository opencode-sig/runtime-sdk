package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewWithConfigWritesFile(t *testing.T) {
	dir := t.TempDir()
	log, err := NewWithConfig(Config{
		ServiceName:     "test-service",
		FilePrefix:      "test",
		Level:           "info",
		StacktraceLevel: "error",
		Format:          "json",
		EnableFile:      true,
		LogDir:          dir,
		Caller:          true,
		FileZone:        "UTC",
	})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	log.Info("hello")
	_ = log.Sync()

	data, err := os.ReadFile(filepath.Join(dir, "test."+time.Now().UTC().Format(dailyLogDateLayout)+".log"))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), `"service":"test-service"`) {
		t.Fatalf("log line missing service field: %s", data)
	}
	if !strings.Contains(string(data), `"msg":"hello"`) {
		t.Fatalf("log line missing message: %s", data)
	}
	if !strings.Contains(string(data), `"caller":`) {
		t.Fatalf("log line missing zap caller: %s", data)
	}
	for _, field := range []string{`"caller_file":`, `"caller_func":`, `"caller_line":`} {
		if strings.Contains(string(data), field) {
			t.Fatalf("log line contains custom caller field %s: %s", field, data)
		}
	}
}

func TestNewWithConfigRejectsBadFormat(t *testing.T) {
	_, err := NewWithConfig(Config{Format: "xml", EnableStdout: true})
	if err == nil {
		t.Fatal("expected bad format error")
	}
}

func TestNewWithConfigDefaultsToStdout(t *testing.T) {
	log, err := NewWithConfig(Config{Format: "json"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	_ = log.Sync()
}

func TestNewWithConfigRejectsBadFileZone(t *testing.T) {
	_, err := NewWithConfig(Config{
		Format:     "json",
		EnableFile: true,
		FileZone:   "bad-zone",
	})
	if err == nil {
		t.Fatal("expected bad file zone error")
	}
}
