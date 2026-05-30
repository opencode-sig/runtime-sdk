package servicekit

import (
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestNewGenerationNormalizesService(t *testing.T) {
	generation := NewGeneration(" gateway ")
	if !strings.HasPrefix(generation, "gateway-") {
		t.Fatalf("generation = %q, want gateway-*", generation)
	}
	if _, err := strconv.ParseInt(strings.TrimPrefix(generation, "gateway-"), 10, 64); err != nil {
		t.Fatalf("generation suffix should be numeric: %v", err)
	}
}

func TestNewGenerationUsesDefaultService(t *testing.T) {
	generation := NewGeneration(" ")
	if !strings.HasPrefix(generation, "dataplane-") {
		t.Fatalf("generation = %q, want dataplane-*", generation)
	}
}

func TestNewGenerationConcurrentUniqueness(t *testing.T) {
	const count = 512
	generations := make(chan string, count)

	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			generations <- NewGeneration("gateway")
		}()
	}
	wg.Wait()
	close(generations)

	seen := make(map[string]struct{}, count)
	for generation := range generations {
		if !strings.HasPrefix(generation, "gateway-") {
			t.Fatalf("generation = %q, want gateway-*", generation)
		}
		if _, exists := seen[generation]; exists {
			t.Fatalf("duplicate generation %q", generation)
		}
		seen[generation] = struct{}{}
	}
	if len(seen) != count {
		t.Fatalf("generation count = %d, want %d", len(seen), count)
	}
}
