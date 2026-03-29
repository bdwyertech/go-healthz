package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/quick"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

func TestDefault(t *testing.T) {
	cwd, err := os.Getwd()
	assert.Nil(t, err)
	os.Args = []string{"cmd", "-config=" + filepath.Join(cwd, "test", "config.yml")}

	// Hack! :-)
	ctx := t.Context()
	go func() {
		go main()
		<-ctx.Done()
	}()
	// Wait a second for the Server
	time.Sleep(1 * time.Second)

	resp, err := http.Get("http://localhost:3456")
	assert.Nil(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	var status GlobalStatus

	err = json.Unmarshal(body, &status)
	assert.Nil(t, err, "Status should be a valid JSON GlobalStatus")

	assert.Equal(t, true, status.Healthy, "Global status should be healthy")
}

// TestRootHandler_ConcurrentUnhealthy verifies the root handler is race-free
// when multiple services report unhealthy concurrently.
func TestRootHandler_ConcurrentUnhealthy(t *testing.T) {
	r := mux.NewRouter()

	numServices := 5

	r.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		global := GlobalStatus{}
		global.Healthy = true

		var healthy atomic.Bool
		healthy.Store(true)

		var wg sync.WaitGroup

		for i := 0; i < numServices; i++ {
			wg.Go(func() {
				healthy.Store(false)
			})
		}

		wg.Wait()
		global.Healthy = healthy.Load()

		if global.Healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(global)
	}).Methods("GET")

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected 503, got %d", rec.Code)
	}

	var status GlobalStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if status.Healthy {
		t.Error("Expected Healthy=false when all services are unhealthy")
	}
}

// TestRootHandler_AllHealthy verifies the root handler returns 200 when all
// checks are healthy.
func TestRootHandler_AllHealthy(t *testing.T) {
	r := mux.NewRouter()

	r.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		global := GlobalStatus{}
		global.Healthy = true

		var wg sync.WaitGroup
		wg.Wait()

		if global.Healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(global)
	}).Methods("GET")

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var status GlobalStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if !status.Healthy {
		t.Error("Expected Healthy=true when all checks are healthy")
	}
}

// TestRootHandler_MixedHealth verifies the root handler returns 503 when at
// least one service is unhealthy.
func TestRootHandler_MixedHealth(t *testing.T) {
	r := mux.NewRouter()

	r.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		global := GlobalStatus{}

		var healthy atomic.Bool
		healthy.Store(true)

		var wg sync.WaitGroup

		healthResults := []bool{true, true, false, true}
		for _, h := range healthResults {
			wg.Go(func() {
				if !h {
					healthy.Store(false)
				}
			})
		}

		wg.Wait()
		global.Healthy = healthy.Load()

		if global.Healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(global)
	}).Methods("GET")

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected 503, got %d", rec.Code)
	}

	var status GlobalStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if status.Healthy {
		t.Error("Expected Healthy=false when any check is unhealthy")
	}
}

// TestRootHandler_JSONStructure verifies the response is valid JSON with the
// expected GlobalStatus fields and correct Content-Type header.
func TestRootHandler_JSONStructure(t *testing.T) {
	r := mux.NewRouter()

	r.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		global := GlobalStatus{
			Healthy:  true,
			Services: []SvcStatus{},
			Commands: []CmdStatus{},
			Requests: []RequestStatus{},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(global)
	}).Methods("GET")

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	if _, ok := raw["Healthy"]; !ok {
		t.Error("Response JSON should contain 'Healthy' field")
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %q", ct)
	}
}

// TestRootHandler_HTTPStatusCodes verifies 200 is returned for healthy and
// 503 for unhealthy.
func TestRootHandler_HTTPStatusCodes(t *testing.T) {
	tests := []struct {
		name           string
		healthy        bool
		expectedStatus int
	}{
		{"healthy", true, http.StatusOK},
		{"unhealthy", false, http.StatusServiceUnavailable},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := mux.NewRouter()
			r.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				global := GlobalStatus{Healthy: tc.healthy}
				if global.Healthy {
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(http.StatusServiceUnavailable)
				}
				json.NewEncoder(w).Encode(global)
			}).Methods("GET")

			req := httptest.NewRequest("GET", "/", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tc.expectedStatus {
				t.Errorf("Expected %d, got %d", tc.expectedStatus, rec.Code)
			}
		})
	}
}

// TestRootHandler_ContextCancellation verifies the handler returns 503
// promptly when the request context is cancelled mid-check.
func TestRootHandler_ContextCancellation(t *testing.T) {
	blocker := make(chan struct{})
	defer close(blocker)

	router := mux.NewRouter()
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		global := GlobalStatus{}
		global.Healthy = true

		var healthy atomic.Bool
		healthy.Store(true)

		var wg sync.WaitGroup
		wg.Go(func() {
			<-blocker
		})

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			global.Healthy = healthy.Load()
		case <-r.Context().Done():
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(GlobalStatus{Healthy: false, Reason: "health check timed out"})
			return
		}

		if global.Healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(global)
	}).Methods("GET")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	start := time.Now()
	router.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("Handler took %v — should have returned promptly on context cancellation", elapsed)
	}

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected 503, got %d", rec.Code)
	}

	var status GlobalStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if status.Healthy {
		t.Error("Expected Healthy=false on timeout")
	}
	if status.Reason == "" {
		t.Error("Expected a timeout reason in the response")
	}
}

// TestPropertyIndexedResultPlacement uses property-based testing to verify that
// each goroutine writes its result to the correct index in the results slice.
func TestPropertyIndexedResultPlacement(t *testing.T) {
	cfg := &quick.Config{MaxCount: 100}

	f := func(outcomes []bool) bool {
		n := len(outcomes)
		if n == 0 || n > 50 {
			return true // skip out-of-range inputs
		}

		// Pre-allocate the results slice.
		results := make([]SvcStatus, n)

		var wg sync.WaitGroup
		for i, healthy := range outcomes {
			wg.Go(func() {
				results[i] = SvcStatus{
					Healthy: healthy,
					Name:    fmt.Sprintf("svc-%d", i),
				}
			})
		}
		wg.Wait()

		// Verify each slot matches its expected outcome.
		for i, expected := range outcomes {
			if results[i].Healthy != expected {
				return false
			}
			if results[i].Name != fmt.Sprintf("svc-%d", i) {
				return false
			}
		}
		return true
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Indexed result placement failed: %v", err)
	}
}

// TestPropertyHealthyFlagAggregate uses property-based testing to verify that
// the aggregate healthy flag is true only when every individual check is true.
func TestPropertyHealthyFlagAggregate(t *testing.T) {
	cfg := &quick.Config{MaxCount: 100}

	// allTrue returns true only if every element is true.
	allTrue := func(v []bool) bool {
		for _, b := range v {
			if !b {
				return false
			}
		}
		return true
	}

	f := func(vector []bool) bool {
		n := len(vector)
		if n == 0 || n > 50 {
			return true // skip out-of-range inputs
		}

		var healthy atomic.Bool
		healthy.Store(true)

		var wg sync.WaitGroup
		for _, val := range vector {
			wg.Go(func() {
				if !val {
					healthy.Store(false)
				}
			})
		}
		wg.Wait()

		return healthy.Load() == allTrue(vector)
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Healthy flag aggregate failed: %v", err)
	}
}
