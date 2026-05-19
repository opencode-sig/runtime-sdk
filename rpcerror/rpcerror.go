package rpcerror

import (
	"strconv"
	"strings"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// UnknownBusinessCode is used when a business error code is invalid.
	UnknownBusinessCode = 1

	businessReason      = "BUSINESS_ERROR"
	businessCodeKey     = "business_code"
	businessErrorDomain = "business"
)

// Business creates a generic business error.
//
// It uses FailedPrecondition by default for business failures that are not
// invalid arguments, missing resources, or conflicts.
func Business(code int, message string) error {
	return WithStatus(codes.FailedPrecondition, code, message)
}

// InvalidArgument creates an invalid argument business error.
func InvalidArgument(code int, message string) error {
	return WithStatus(codes.InvalidArgument, code, message)
}

// NotFound creates a missing resource business error.
func NotFound(code int, message string) error {
	return WithStatus(codes.NotFound, code, message)
}

// Conflict creates a resource conflict business error.
func Conflict(code int, message string) error {
	return WithStatus(codes.AlreadyExists, code, message)
}

// WithStatus creates a gRPC status error with a business code.
//
// The business code is stored in ErrorInfo metadata so a Gateway can copy it
// into its HTTP response envelope.
func WithStatus(grpcCode codes.Code, code int, message string) error {
	code = normalizeBusinessCode(code)
	message = normalizeMessage(message)

	st := status.New(grpcCode, message)
	st, err := st.WithDetails(&errdetails.ErrorInfo{
		Reason: businessReason,
		Domain: businessErrorDomain,
		Metadata: map[string]string{
			businessCodeKey: strconv.Itoa(code),
		},
	})
	if err != nil {
		return status.Error(grpcCode, message)
	}
	return st.Err()
}

// BusinessCode extracts the business code from gRPC status details.
//
// It returns false when the status does not contain the SDK ErrorInfo detail.
func BusinessCode(st *status.Status) (int, bool) {
	if st == nil {
		return 0, false
	}
	for _, detail := range st.Details() {
		info, ok := detail.(*errdetails.ErrorInfo)
		if !ok || info.GetReason() != businessReason {
			continue
		}
		code, err := strconv.Atoi(info.GetMetadata()[businessCodeKey])
		if err != nil || code <= 0 {
			continue
		}
		return code, true
	}
	return 0, false
}

// normalizeBusinessCode ensures business codes are always positive.
func normalizeBusinessCode(code int) int {
	if code <= 0 {
		return UnknownBusinessCode
	}
	return code
}

// normalizeMessage ensures the business error message is safe to expose.
func normalizeMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "business error"
	}
	return message
}
