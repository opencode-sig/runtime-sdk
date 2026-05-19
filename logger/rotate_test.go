package logger

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDailyWriterRotatesByDay(t *testing.T) {
	dir := t.TempDir()
	current := time.Date(2026, 5, 17, 23, 59, 0, 0, time.UTC)
	writer, err := newDailyWriter(dailyWriterConfig{
		Dir:        dir,
		Prefix:     "app",
		Location:   time.UTC,
		MaxAgeDays: 7,
		Now: func() time.Time {
			return current
		},
	})
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	defer writer.Close()

	if _, err := writer.Write([]byte("day1\n")); err != nil {
		t.Fatalf("write day1: %v", err)
	}
	current = time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	if _, err := writer.Write([]byte("day2\n")); err != nil {
		t.Fatalf("write day2: %v", err)
	}
	if err := writer.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	assertFileContains(t, filepath.Join(dir, "app.2026-05-17.log"), "day1")
	assertFileContains(t, filepath.Join(dir, "app.2026-05-18.log"), "day2")
}

func TestDailyWriterCleanupMaxAgeDays(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "app.2026-05-10.log"), "old")
	writeTestFile(t, filepath.Join(dir, "app.2026-05-11.log"), "keep")

	writer, err := newDailyWriter(dailyWriterConfig{
		Dir:        dir,
		Prefix:     "app",
		Location:   time.UTC,
		MaxAgeDays: 7,
		Now: func() time.Time {
			return time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	defer writer.Close()

	if _, err := os.Stat(filepath.Join(dir, "app.2026-05-10.log")); !os.IsNotExist(err) {
		t.Fatalf("old log still exists, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app.2026-05-11.log")); err != nil {
		t.Fatalf("kept log missing: %v", err)
	}
}

func TestDailyWriterConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	writer, err := newDailyWriter(dailyWriterConfig{
		Dir:        dir,
		Prefix:     "app",
		Location:   time.UTC,
		MaxAgeDays: 7,
		Now: func() time.Time {
			return time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	defer writer.Close()

	const goroutines = 100
	const writesPerGoroutine = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				if _, err := writer.Write([]byte("line\n")); err != nil {
					t.Errorf("write: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if err := writer.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "app.2026-05-17.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if got, want := strings.Count(string(data), "line\n"), goroutines*writesPerGoroutine; got != want {
		t.Fatalf("line count = %d, want %d", got, want)
	}
}

func assertFileContains(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("%s does not contain %q: %s", path, want, data)
	}
}

func writeTestFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
