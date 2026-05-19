package component

import (
	"context"
	"errors"
	"testing"
)

func TestCloseComponentClosesOnce(t *testing.T) {
	calls := 0
	component := NewCloseComponent(func() error {
		calls++
		return nil
	})

	if err := component.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := component.Health(context.Background()); err != nil {
		t.Fatalf("health: %v", err)
	}
	if err := component.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := component.Stop(context.Background()); err != nil {
		t.Fatalf("second stop: %v", err)
	}
	if calls != 1 {
		t.Fatalf("close calls = %d", calls)
	}
	if err := component.Health(context.Background()); err == nil {
		t.Fatal("closed component should not be healthy")
	}
}

func TestCloseComponentRequiresCloseFunc(t *testing.T) {
	component := NewCloseComponent(nil)
	err := component.Start(context.Background())
	if err == nil {
		t.Fatal("expected start error")
	}
}

func TestCloseComponentReturnsCloseError(t *testing.T) {
	closeErr := errors.New("close failed")
	component := NewCloseComponent(func() error {
		return closeErr
	})

	if err := component.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := component.Stop(context.Background()); !errors.Is(err, closeErr) {
		t.Fatalf("stop error = %v", err)
	}
}
