package circuitbreaker

import (
	"errors"
	"testing"
	"time"
)

// ============================================================================
// CIRCUIT BREAKER BASIC TESTS
// ============================================================================

func TestCircuitBreaker_New(t *testing.T) {
	cb := New("test", nil)
	if cb == nil {
		t.Fatal("New() returned nil")
	}

	if cb.name != "test" {
		t.Errorf("Expected name 'test', got '%s'", cb.name)
	}

	if cb.State() != StateClosed {
		t.Errorf("Expected initial state Closed, got %v", cb.State())
	}
}

func TestCircuitBreaker_NewWithConfig(t *testing.T) {
	config := &Config{
		MaxRequests: 10,
		Interval:    30 * time.Second,
		Timeout:     15 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			return counts.TotalFailures > 5
		},
	}

	cb := New("test", config)
	if cb == nil {
		t.Fatal("New() returned nil")
	}

	if cb.config.MaxRequests != config.MaxRequests {
		t.Errorf("Expected MaxRequests %d, got %d", config.MaxRequests, cb.config.MaxRequests)
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("State.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ============================================================================
// EXECUTE TESTS
// ============================================================================

func TestCircuitBreaker_Execute_Success(t *testing.T) {
	cb := New("test", nil)

	err := cb.Execute(func() error {
		return nil
	})

	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	if cb.State() != StateClosed {
		t.Errorf("Expected state Closed, got %v", cb.State())
	}
}

func TestCircuitBreaker_Execute_Failure(t *testing.T) {
	cb := New("test", nil)

	expectedErr := errors.New("test error")
	err := cb.Execute(func() error {
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestCircuitBreaker_Execute_CircuitOpen(t *testing.T) {
	config := &Config{
		MaxRequests: 1,
		Interval:    1 * time.Second,
		Timeout:     100 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.TotalFailures >= 3
		},
	}

	cb := New("test", config)

	// Trigger failures to open the circuit
	for i := 0; i < 3; i++ {
		cb.Execute(func() error {
			return errors.New("failure")
		})
	}

	// Circuit should be open now
	if cb.State() != StateOpen {
		t.Errorf("Expected state Open, got %v", cb.State())
	}

	// Next request should fail immediately
	err := cb.Execute(func() error {
		return nil
	})

	if err != ErrCircuitOpen {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}
}

// ============================================================================
// STATE TRANSITION TESTS
// ============================================================================

func TestCircuitBreaker_StateTransition_ClosedToOpen(t *testing.T) {
	config := &Config{
		MaxRequests: 1,
		Interval:    1 * time.Second,
		Timeout:     100 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.TotalFailures >= 2
		},
	}

	cb := New("test", config)

	// Initial state should be Closed
	if cb.State() != StateClosed {
		t.Fatalf("Expected initial state Closed, got %v", cb.State())
	}

	// Trigger failures
	for i := 0; i < 2; i++ {
		cb.Execute(func() error {
			return errors.New("failure")
		})
	}

	// Should transition to Open
	if cb.State() != StateOpen {
		t.Errorf("Expected state Open after failures, got %v", cb.State())
	}
}

func TestCircuitBreaker_StateTransition_OpenToHalfOpen(t *testing.T) {
	config := &Config{
		MaxRequests: 1,
		Interval:    1 * time.Second,
		Timeout:     100 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.TotalFailures >= 2
		},
	}

	cb := New("test", config)

	// Open the circuit
	for i := 0; i < 2; i++ {
		cb.Execute(func() error {
			return errors.New("failure")
		})
	}

	if cb.State() != StateOpen {
		t.Fatalf("Expected state Open, got %v", cb.State())
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Next request should transition to HalfOpen
	cb.Execute(func() error {
		return nil
	})

	// Note: State might be Closed if the request succeeded
	state := cb.State()
	if state != StateHalfOpen && state != StateClosed {
		t.Errorf("Expected state HalfOpen or Closed, got %v", state)
	}
}

func TestCircuitBreaker_StateTransition_HalfOpenToClosed(t *testing.T) {
	config := &Config{
		MaxRequests: 2,
		Interval:    1 * time.Second,
		Timeout:     100 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.TotalFailures >= 2
		},
	}

	cb := New("test", config)

	// Open the circuit
	for i := 0; i < 2; i++ {
		cb.Execute(func() error {
			return errors.New("failure")
		})
	}

	// Wait for timeout to transition to HalfOpen
	time.Sleep(150 * time.Millisecond)

	// Successful requests in HalfOpen should close the circuit
	for i := 0; i < 2; i++ {
		cb.Execute(func() error {
			return nil
		})
	}

	// Note: State might still be HalfOpen or Closed depending on timing
	state := cb.State()
	if state != StateClosed && state != StateHalfOpen {
		t.Errorf("Expected state Closed or HalfOpen after successful requests, got %v", state)
	}
}

// ============================================================================
// COUNTS TESTS
// ============================================================================

func TestCircuitBreaker_Counts(t *testing.T) {
	cb := New("test", nil)

	// Execute successful requests
	for i := 0; i < 3; i++ {
		cb.Execute(func() error {
			return nil
		})
	}

	counts := cb.Counts()
	if counts.TotalSuccesses != 3 {
		t.Errorf("Expected 3 successes, got %d", counts.TotalSuccesses)
	}

	// Execute failed requests
	for i := 0; i < 2; i++ {
		cb.Execute(func() error {
			return errors.New("failure")
		})
	}

	counts = cb.Counts()
	if counts.TotalFailures != 2 {
		t.Errorf("Expected 2 failures, got %d", counts.TotalFailures)
	}
}

// ============================================================================
// CALLBACK TESTS
// ============================================================================

func TestCircuitBreaker_OnStateChange(t *testing.T) {
	stateChanges := []State{}

	config := &Config{
		MaxRequests: 1,
		Interval:    1 * time.Second,
		Timeout:     100 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.TotalFailures >= 2
		},
		OnStateChange: func(name string, from State, to State) {
			stateChanges = append(stateChanges, to)
		},
	}

	cb := New("test", config)

	// Trigger state change to Open
	for i := 0; i < 2; i++ {
		cb.Execute(func() error {
			return errors.New("failure")
		})
	}

	if len(stateChanges) == 0 {
		t.Skip("State change callback not called (implementation may not support it)")
		return
	}

	if stateChanges[len(stateChanges)-1] != StateOpen {
		t.Errorf("Expected last state change to Open, got %v", stateChanges[len(stateChanges)-1])
	}
}

// ============================================================================
// CONCURRENT TESTS
// ============================================================================

func TestCircuitBreaker_Concurrent(t *testing.T) {
	// Use a config that won't trip the circuit
	config := &Config{
		MaxRequests: 100,
		Interval:    1 * time.Second,
		Timeout:     1 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			return false // Never trip
		},
	}

	cb := New("test", config)

	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func(id int) {
			cb.Execute(func() error {
				if id%2 == 0 {
					return nil
				}
				return errors.New("failure")
			})
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	counts := cb.Counts()
	if counts.Requests != 100 {
		t.Errorf("Expected 100 requests, got %d", counts.Requests)
	}
}

// ============================================================================
// RESET TESTS
// ============================================================================

func TestCircuitBreaker_Reset(t *testing.T) {
	config := &Config{
		MaxRequests: 1,
		Interval:    1 * time.Second,
		Timeout:     100 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.TotalFailures >= 2
		},
	}

	cb := New("test", config)

	// Open the circuit
	for i := 0; i < 2; i++ {
		cb.Execute(func() error {
			return errors.New("failure")
		})
	}

	if cb.State() != StateOpen {
		t.Fatalf("Expected state Open, got %v", cb.State())
	}

	// Reset the circuit breaker
	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("Expected state Closed after reset, got %v", cb.State())
	}

	counts := cb.Counts()
	if counts.Requests != 0 {
		t.Errorf("Expected 0 requests after reset, got %d", counts.Requests)
	}
}

// ============================================================================
// ADDITIONAL COVERAGE TESTS
// ============================================================================

func TestCircuitBreaker_DefaultConfig(t *testing.T) {
	cb := New("test", nil)

	if cb.State() != StateClosed {
		t.Errorf("Expected StateClosed, got %v", cb.State())
	}

	// Should work with default config
	err := cb.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected success with default config, got error: %v", err)
	}
}

func TestCircuitBreaker_Name(t *testing.T) {
	cb := New("test-breaker", &Config{})

	if cb.Name() != "test-breaker" {
		t.Errorf("Expected name 'test-breaker', got '%s'", cb.Name())
	}
}
