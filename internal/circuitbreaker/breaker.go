package circuitbreaker

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// State represents the circuit breaker state
type State int32

const (
	// StateClosed means requests are allowed
	StateClosed State = iota
	// StateOpen means requests are blocked
	StateOpen
	// StateHalfOpen means limited requests are allowed to test recovery
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

var (
	// ErrCircuitOpen is returned when the circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")
	// ErrTooManyRequests is returned when too many requests are made in half-open state
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// Config defines circuit breaker configuration
type Config struct {
	// MaxRequests is the maximum number of requests allowed in half-open state
	MaxRequests uint32
	// Interval is the cyclic period in closed state to clear internal counts
	Interval time.Duration
	// Timeout is the period of open state before transitioning to half-open
	Timeout time.Duration
	// ReadyToTrip is called with counts to determine if the breaker should trip
	ReadyToTrip func(counts Counts) bool
	// OnStateChange is called whenever the state changes
	OnStateChange func(name string, from State, to State)
}

// Counts holds the statistics for the circuit breaker
type Counts struct {
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

// DefaultConfig returns a default circuit breaker configuration
func DefaultConfig() *Config {
	return &Config{
		MaxRequests: 5,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			// Trip if failure rate > 50% and at least 10 requests
			return counts.Requests >= 10 &&
				float64(counts.TotalFailures)/float64(counts.Requests) >= 0.5
		},
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	name          string
	config        *Config
	state         int32 // atomic State
	generation    uint64
	counts        Counts
	expiry        int64 // Unix timestamp in nanoseconds
	mu            sync.RWMutex
	stateChangeMu sync.Mutex
}

// New creates a new circuit breaker
func New(name string, config *Config) *CircuitBreaker {
	if config == nil {
		config = DefaultConfig()
	}

	cb := &CircuitBreaker{
		name:   name,
		config: config,
		state:  int32(StateClosed),
		expiry: time.Now().Add(config.Interval).UnixNano(),
	}

	return cb
}

// Name returns the circuit breaker name
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

// State returns the current state
func (cb *CircuitBreaker) State() State {
	return State(atomic.LoadInt32(&cb.state))
}

// Counts returns a copy of the current counts
func (cb *CircuitBreaker) Counts() Counts {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.counts
}

// Execute runs the given function if the circuit breaker allows it
func (cb *CircuitBreaker) Execute(fn func() error) error {
	generation, err := cb.beforeRequest()
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			cb.afterRequest(generation, false)
			panic(r)
		}
	}()

	err = fn()
	cb.afterRequest(generation, err == nil)
	return err
}

// beforeRequest checks if the request can proceed
func (cb *CircuitBreaker) beforeRequest() (uint64, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := State(atomic.LoadInt32(&cb.state))
	generation := cb.generation

	switch state {
	case StateClosed:
		// Check if we need to reset counts
		if cb.expiry <= time.Now().UnixNano() {
			cb.resetCounts()
			cb.expiry = time.Now().Add(cb.config.Interval).UnixNano()
		}
		cb.counts.Requests++
		return generation, nil

	case StateOpen:
		// Check if timeout has passed
		if cb.expiry <= time.Now().UnixNano() {
			cb.setState(StateHalfOpen)
			cb.resetCounts()
			cb.counts.Requests++
			return generation, nil
		}
		return generation, ErrCircuitOpen

	case StateHalfOpen:
		// Allow limited requests in half-open state
		if cb.counts.Requests >= cb.config.MaxRequests {
			return generation, ErrTooManyRequests
		}
		cb.counts.Requests++
		return generation, nil

	default:
		return generation, fmt.Errorf("unknown circuit breaker state: %v", state)
	}
}

// afterRequest records the result of the request
func (cb *CircuitBreaker) afterRequest(generation uint64, success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Ignore if generation has changed (state transition occurred)
	if generation != cb.generation {
		return
	}

	if success {
		cb.onSuccess()
	} else {
		cb.onFailure()
	}
}

// onSuccess handles a successful request
func (cb *CircuitBreaker) onSuccess() {
	cb.counts.TotalSuccesses++
	cb.counts.ConsecutiveSuccesses++
	cb.counts.ConsecutiveFailures = 0

	state := State(atomic.LoadInt32(&cb.state))

	if state == StateHalfOpen {
		// If we've had enough successful requests in half-open, close the circuit
		if cb.counts.ConsecutiveSuccesses >= cb.config.MaxRequests {
			cb.setState(StateClosed)
			cb.resetCounts()
			cb.expiry = time.Now().Add(cb.config.Interval).UnixNano()
		}
	}
}

// onFailure handles a failed request
func (cb *CircuitBreaker) onFailure() {
	cb.counts.TotalFailures++
	cb.counts.ConsecutiveFailures++
	cb.counts.ConsecutiveSuccesses = 0

	state := State(atomic.LoadInt32(&cb.state))

	switch state {
	case StateClosed:
		// Check if we should trip the breaker
		if cb.config.ReadyToTrip != nil && cb.config.ReadyToTrip(cb.counts) {
			cb.setState(StateOpen)
			cb.expiry = time.Now().Add(cb.config.Timeout).UnixNano()
		}

	case StateHalfOpen:
		// Any failure in half-open state reopens the circuit
		cb.setState(StateOpen)
		cb.expiry = time.Now().Add(cb.config.Timeout).UnixNano()
	}
}

// setState changes the circuit breaker state
func (cb *CircuitBreaker) setState(newState State) {
	cb.stateChangeMu.Lock()
	defer cb.stateChangeMu.Unlock()

	oldState := State(atomic.LoadInt32(&cb.state))
	if oldState == newState {
		return
	}

	atomic.StoreInt32(&cb.state, int32(newState))
	cb.generation++

	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(cb.name, oldState, newState)
	}
}

// resetCounts resets all counts to zero
func (cb *CircuitBreaker) resetCounts() {
	cb.counts = Counts{}
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.setState(StateClosed)
	cb.resetCounts()
	cb.expiry = time.Now().Add(cb.config.Interval).UnixNano()
}

// Trip manually trips the circuit breaker to open state
func (cb *CircuitBreaker) Trip() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.setState(StateOpen)
	cb.expiry = time.Now().Add(cb.config.Timeout).UnixNano()
}

