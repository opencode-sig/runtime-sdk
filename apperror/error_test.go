package apperror

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestWrapRecordsCallerFileAndLine(t *testing.T) {
	base := os.ErrNotExist
	_, file, line, _ := runtime.Caller(0)
	err := Wrap(base, "open file")

	if !errors.Is(err, base) {
		t.Fatal("wrapped error should preserve errors.Is")
	}
	frame, ok := FrameOf(err)
	if !ok {
		t.Fatal("expected frame")
	}
	if frame.File != "error_test.go" {
		t.Fatalf("file = %q", frame.File)
	}
	if frame.Line != line+1 {
		t.Fatalf("line = %d, want %d from %s", frame.Line, line+1, file)
	}
	if !strings.Contains(err.Error(), "error_test.go:") {
		t.Fatalf("error missing source location: %s", err)
	}
}

func TestWrapNilReturnsNil(t *testing.T) {
	if err := Wrap(nil, "noop"); err != nil {
		t.Fatalf("wrap nil = %v", err)
	}
	if err := Wrapf(nil, "noop %s", "x"); err != nil {
		t.Fatalf("wrapf nil = %v", err)
	}
}

func TestNewRecordsCallerLocation(t *testing.T) {
	_, _, line, _ := runtime.Caller(0)
	err := New("created")

	var wrapped *Error
	if !errors.As(err, &wrapped) {
		t.Fatal("expected *Error")
	}
	frame := wrapped.Frame()
	if frame.File != "error_test.go" {
		t.Fatalf("file = %q", frame.File)
	}
	if frame.Line != line+1 {
		t.Fatalf("line = %d, want %d", frame.Line, line+1)
	}
	if wrapped.Message() != "created" {
		t.Fatalf("message = %q", wrapped.Message())
	}
}

func TestWrapfFormatsMessage(t *testing.T) {
	err := Wrapf(os.ErrPermission, "write %s", "config")
	if !strings.Contains(err.Error(), "write config") {
		t.Fatalf("error missing formatted message: %s", err)
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatal("wrapped error should preserve cause")
	}
}
