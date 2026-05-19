// Package apperror provides lightweight error wrapping with source location.
//
// The import path is apperror to avoid colliding with the predeclared Go error type
// while keeping the package purpose explicit.
package apperror

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// Frame records the source location where an error was created or wrapped.
type Frame struct {
	File     string
	Line     int
	Function string
}

// Error wraps an optional cause with a message and the caller source location.
type Error struct {
	message string
	cause   error
	frame   Frame
}

// New creates an application error with the caller source location.
func New(message string) error {
	return &Error{
		message: normalizeMessage(message),
		frame:   callerFrame(1),
	}
}

// Errorf creates an application error with a formatted message and caller source location.
func Errorf(format string, args ...any) error {
	return &Error{
		message: normalizeMessage(fmt.Sprintf(format, args...)),
		frame:   callerFrame(1),
	}
}

// Wrap wraps err with message and records the file name and line at the wrap call site.
//
// A nil err returns nil so callers can use:
//
//	if err != nil {
//	    return apperror.Wrap(err, "read config")
//	}
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return &Error{
		message: normalizeMessage(message),
		cause:   err,
		frame:   callerFrame(1),
	}
}

// Wrapf wraps err with a formatted message and records the wrap call site.
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return &Error{
		message: normalizeMessage(fmt.Sprintf(format, args...)),
		cause:   err,
		frame:   callerFrame(1),
	}
}

// Error returns a human-readable error string containing message, cause and source location.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	location := e.frame.String()
	switch {
	case e.cause != nil && e.message != "":
		return fmt.Sprintf("%s: %v (%s)", e.message, e.cause, location)
	case e.cause != nil:
		return fmt.Sprintf("%v (%s)", e.cause, location)
	case e.message != "":
		return fmt.Sprintf("%s (%s)", e.message, location)
	default:
		return location
	}
}

// Unwrap returns the wrapped cause so errors.Is and errors.As continue to work.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Frame returns the captured source location.
func (e *Error) Frame() Frame {
	if e == nil {
		return Frame{}
	}
	return e.frame
}

// Message returns the wrapping message without source location or cause text.
func (e *Error) Message() string {
	if e == nil {
		return ""
	}
	return e.message
}

// FrameOf returns the first source frame found in an error chain.
func FrameOf(err error) (Frame, bool) {
	var wrapped *Error
	if errors.As(err, &wrapped) {
		return wrapped.Frame(), true
	}
	return Frame{}, false
}

// String formats the frame as file:line.
func (f Frame) String() string {
	if f.File == "" || f.Line <= 0 {
		return "unknown:0"
	}
	return fmt.Sprintf("%s:%d", f.File, f.Line)
}

// callerFrame returns the source location of the public helper caller.
func callerFrame(skip int) Frame {
	pc, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return Frame{}
	}
	frame := Frame{
		File: filepath.Base(file),
		Line: line,
	}
	if fn := runtime.FuncForPC(pc); fn != nil {
		frame.Function = fn.Name()
	}
	return frame
}

// normalizeMessage keeps empty messages from producing awkward error strings.
func normalizeMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "error"
	}
	return message
}
