package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
)

type CheckFunc func(ctx context.Context) error

type Checker struct {
	mu     sync.RWMutex
	checks map[string]CheckFunc
}

type Response struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// New creates a health check aggregator.
func New() *Checker {
	return &Checker{checks: make(map[string]CheckFunc)}
}

// Add registers a named health check.
//
// Empty names and nil checks are ignored to avoid panics from bad call sites.
func (c *Checker) Add(name string, fn CheckFunc) {
	if name == "" || fn == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks[name] = fn
}

// Check runs all registered health checks and returns the combined result.
//
// It copies the check map before execution so slow probes do not hold the lock.
func (c *Checker) Check(ctx context.Context) Response {
	c.mu.RLock()
	checks := make(map[string]CheckFunc, len(c.checks))
	for name, fn := range c.checks {
		checks[name] = fn
	}
	c.mu.RUnlock()

	resp := Response{Status: "ok", Checks: make(map[string]string, len(checks))}
	for name, fn := range checks {
		if err := fn(ctx); err != nil {
			resp.Status = "degraded"
			resp.Checks[name] = err.Error()
			continue
		}
		resp.Checks[name] = "ok"
	}
	return resp
}

// Handler returns the standard /healthz HTTP handler.
//
// Any failed check returns 503 with failed check names and messages.
func (c *Checker) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := c.Check(r.Context())
		w.Header().Set("Content-Type", "application/json")
		if resp.Status != "ok" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}
