package health

import (
	"context"
	"errors"
	"testing"
)

func TestCheckerStatus(t *testing.T) {
	checker := New()
	checker.Add("ok", func(ctx context.Context) error { return nil })
	checker.Add("bad", func(ctx context.Context) error { return errors.New("failed") })

	resp := checker.Check(context.Background())
	if resp.Status != "degraded" {
		t.Fatalf("got status %q, want degraded", resp.Status)
	}
	if resp.Checks["ok"] != "ok" || resp.Checks["bad"] != "failed" {
		t.Fatalf("unexpected checks: %#v", resp.Checks)
	}
}
