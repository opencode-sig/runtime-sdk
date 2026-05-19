package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const dailyLogDateLayout = "2006-01-02"

type dailyWriterConfig struct {
	Dir        string
	Prefix     string
	Location   *time.Location
	MaxAgeDays int
	Now        func() time.Time
}

// dailyWriter is a daily-rotating zapcore.WriteSyncer implementation.
//
// Write, Sync, and Close share one lock so concurrent writes and date changes
// cannot race with closing the previous file.
type dailyWriter struct {
	mu sync.Mutex

	dir        string
	prefix     string
	location   *time.Location
	maxAgeDays int
	now        func() time.Time

	file         *os.File
	currentDate  string
	nextRotation time.Time
}

// newDailyWriter creates a daily writer and opens today's log file immediately.
func newDailyWriter(config dailyWriterConfig) (*dailyWriter, error) {
	writer := &dailyWriter{
		dir:        firstNonEmpty(config.Dir, defaultLogDir),
		prefix:     cleanFilePrefix(firstNonEmpty(config.Prefix, defaultFilePrefix)),
		location:   config.Location,
		maxAgeDays: normalizeMaxAgeDays(config.MaxAgeDays),
		now:        config.Now,
	}
	if writer.location == nil {
		writer.location = time.Local
	}
	if writer.now == nil {
		writer.now = time.Now
	}
	if err := os.MkdirAll(writer.dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	if err := writer.rotateLocked(writer.now().In(writer.location)); err != nil {
		return nil, err
	}
	if err := writer.cleanupLocked(writer.now().In(writer.location)); err != nil {
		return nil, err
	}
	return writer, nil
}

// Write writes log bytes and rotates automatically when the date changes.
func (w *dailyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := w.now().In(w.location)
	if w.file == nil || !now.Before(w.nextRotation) {
		if err := w.rotateLocked(now); err != nil {
			return 0, err
		}
		if err := w.cleanupLocked(now); err != nil {
			return 0, err
		}
	}
	return w.file.Write(p)
}

// Sync flushes the current log file to disk.
func (w *dailyWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}
	return w.file.Sync()
}

// Close closes the current log file.
func (w *dailyWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// rotateLocked switches to the log file for now's date.
func (w *dailyWriter) rotateLocked(now time.Time) error {
	date := now.Format(dailyLogDateLayout)
	if w.file != nil && w.currentDate == date {
		return nil
	}
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return fmt.Errorf("close log file: %w", err)
		}
		w.file = nil
	}

	file, err := os.OpenFile(w.logPath(date), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	w.file = file
	w.currentDate = date
	w.nextRotation = nextDay(now)
	return nil
}

// cleanupLocked deletes log files older than the maxAgeDays retention window.
func (w *dailyWriter) cleanupLocked(now time.Time) error {
	cutoff := startOfDay(now).AddDate(0, 0, -w.maxAgeDays+1)
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return fmt.Errorf("read log dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		date, ok := w.dateFromFileName(entry.Name())
		if !ok || !date.Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(w.dir, entry.Name())); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove old log file: %w", err)
		}
	}
	return nil
}

// logPath returns the log file path for a date.
func (w *dailyWriter) logPath(date string) string {
	return filepath.Join(w.dir, w.prefix+"."+date+".log")
}

// dateFromFileName parses the date from a file name with the current prefix.
func (w *dailyWriter) dateFromFileName(name string) (time.Time, bool) {
	prefix := w.prefix + "."
	suffix := ".log"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return time.Time{}, false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
	date, err := time.ParseInLocation(dailyLogDateLayout, value, w.location)
	if err != nil {
		return time.Time{}, false
	}
	return date, true
}

// cleanFilePrefix prevents file_prefix path separators from escaping log_dir.
func cleanFilePrefix(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultFilePrefix
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_")
	return replacer.Replace(value)
}

// startOfDay returns midnight for now's date in its location.
func startOfDay(now time.Time) time.Time {
	year, month, day := now.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, now.Location())
}

// nextDay returns midnight of the next day in now's location.
func nextDay(now time.Time) time.Time {
	return startOfDay(now).AddDate(0, 0, 1)
}
