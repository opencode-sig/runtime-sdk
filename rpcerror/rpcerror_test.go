package rpcerror

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBusinessCode(t *testing.T) {
	err := InvalidArgument(10001, "user id is required")
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected grpc status error: %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("grpc code = %s", st.Code())
	}
	if st.Message() != "user id is required" {
		t.Fatalf("message = %q", st.Message())
	}

	code, ok := BusinessCode(st)
	if !ok {
		t.Fatal("expected business code")
	}
	if code != 10001 {
		t.Fatalf("business code = %d", code)
	}
}

func TestBusinessNormalizesInvalidInput(t *testing.T) {
	err := Business(0, " ")
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected grpc status error: %v", err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Fatalf("grpc code = %s", st.Code())
	}
	if st.Message() != "business error" {
		t.Fatalf("message = %q", st.Message())
	}

	code, ok := BusinessCode(st)
	if !ok {
		t.Fatal("expected business code")
	}
	if code != UnknownBusinessCode {
		t.Fatalf("business code = %d", code)
	}
}

func TestBusinessCodeReturnsFalseForPlainStatus(t *testing.T) {
	st := status.New(codes.InvalidArgument, "plain error")
	if code, ok := BusinessCode(st); ok {
		t.Fatalf("business code = %d", code)
	}
}
