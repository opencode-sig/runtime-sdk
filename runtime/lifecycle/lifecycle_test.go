package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

type component struct {
	started   bool
	stopped   bool
	startErr  error
	stopErr   error
	healthErr error
	order     *[]string
	name      string
}

func (c *component) Start(ctx context.Context) error {
	c.started = true
	return c.startErr
}

func (c *component) Stop(ctx context.Context) error {
	c.stopped = true
	if c.order != nil {
		*c.order = append(*c.order, c.name)
	}
	return c.stopErr
}

func (c *component) Health(ctx context.Context) error {
	return c.healthErr
}

func TestRuntimeStartStopHealth(t *testing.T) {
	rt := New("test")
	c := &component{}
	if err := rt.Add("component", c); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := rt.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := rt.Health(context.Background()); err != nil {
		t.Fatalf("health: %v", err)
	}
	if err := rt.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if !c.started || !c.stopped {
		t.Fatalf("component lifecycle mismatch: started=%v stopped=%v", c.started, c.stopped)
	}
}

func TestRuntimeRejectsDuplicateComponents(t *testing.T) {
	rt := New("test")
	if err := rt.Add("component", &component{}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := rt.Add("component", &component{}); err == nil {
		t.Fatal("expected duplicate component error")
	}
}

func TestRuntimeRejectsAddWhileRunning(t *testing.T) {
	rt := New("test")
	if err := rt.Add("one", &component{}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := rt.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := rt.Add("two", &component{}); err == nil {
		t.Fatal("expected add while running error")
	}
}

func TestRuntimeStopsStartedComponentsOnStartFailure(t *testing.T) {
	startErr := errors.New("boom")
	var stopOrder []string
	rt := New("test")
	first := &component{name: "first", order: &stopOrder}
	second := &component{name: "second", startErr: startErr, order: &stopOrder}

	if err := rt.Add("first", first); err != nil {
		t.Fatalf("add first: %v", err)
	}
	if err := rt.Add("second", second); err != nil {
		t.Fatalf("add second: %v", err)
	}

	err := rt.Start(context.Background())
	if !errors.Is(err, startErr) {
		t.Fatalf("expected start error, got %v", err)
	}
	if !first.stopped {
		t.Fatal("first component should be stopped after start failure")
	}
	if len(stopOrder) != 1 || stopOrder[0] != "first" {
		t.Fatalf("stop order = %#v", stopOrder)
	}
	if err := rt.Health(context.Background()); err == nil {
		t.Fatal("failed runtime should not be healthy")
	}
}

func TestRuntimeStopsInReverseOrder(t *testing.T) {
	var stopOrder []string
	rt := New("test")
	for _, name := range []string{"one", "two", "three"} {
		if err := rt.Add(name, &component{name: name, order: &stopOrder}); err != nil {
			t.Fatalf("add %s: %v", name, err)
		}
	}
	if err := rt.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := rt.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}

	want := []string{"three", "two", "one"}
	if !reflect.DeepEqual(stopOrder, want) {
		t.Fatalf("stop order = %#v, want %#v", stopOrder, want)
	}
}

func TestRuntimeHealthReportsComponentName(t *testing.T) {
	healthErr := errors.New("not ready")
	rt := New("test")
	if err := rt.Add("db", &component{healthErr: healthErr}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := rt.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	err := rt.Health(context.Background())
	if !errors.Is(err, healthErr) {
		t.Fatalf("health error = %v", err)
	}
	if got := fmt.Sprint(err); got != "health db: not ready" {
		t.Fatalf("health message = %q", got)
	}
}
