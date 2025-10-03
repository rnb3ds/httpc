package circuitbreaker

import (
	"sync"
)

// Manager manages multiple circuit breakers for different hosts
type Manager struct {
	breakers sync.Map // map[string]*CircuitBreaker
	config   *Config
	mu       sync.RWMutex
}

// NewManager creates a new circuit breaker manager
func NewManager(config *Config) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	return &Manager{
		config: config,
	}
}

// GetBreaker returns the circuit breaker for the given host
// Creates a new one if it doesn't exist
func (m *Manager) GetBreaker(host string) *CircuitBreaker {
	if breaker, ok := m.breakers.Load(host); ok {
		return breaker.(*CircuitBreaker)
	}

	// Create new breaker
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring lock
	if breaker, ok := m.breakers.Load(host); ok {
		return breaker.(*CircuitBreaker)
	}

	breaker := New(host, m.config)
	m.breakers.Store(host, breaker)
	return breaker
}

// Execute runs the function through the circuit breaker for the given host
func (m *Manager) Execute(host string, fn func() error) error {
	breaker := m.GetBreaker(host)
	return breaker.Execute(fn)
}

// Reset resets the circuit breaker for the given host
func (m *Manager) Reset(host string) {
	if breaker, ok := m.breakers.Load(host); ok {
		breaker.(*CircuitBreaker).Reset()
	}
}

// Trip trips the circuit breaker for the given host
func (m *Manager) Trip(host string) {
	breaker := m.GetBreaker(host)
	breaker.Trip()
}

// GetState returns the state of the circuit breaker for the given host
func (m *Manager) GetState(host string) State {
	if breaker, ok := m.breakers.Load(host); ok {
		return breaker.(*CircuitBreaker).State()
	}
	return StateClosed
}

// GetCounts returns the counts for the circuit breaker for the given host
func (m *Manager) GetCounts(host string) Counts {
	if breaker, ok := m.breakers.Load(host); ok {
		return breaker.(*CircuitBreaker).Counts()
	}
	return Counts{}
}

// GetAllStates returns the states of all circuit breakers
func (m *Manager) GetAllStates() map[string]State {
	states := make(map[string]State)
	m.breakers.Range(func(key, value interface{}) bool {
		host := key.(string)
		breaker := value.(*CircuitBreaker)
		states[host] = breaker.State()
		return true
	})
	return states
}

// ResetAll resets all circuit breakers
func (m *Manager) ResetAll() {
	m.breakers.Range(func(key, value interface{}) bool {
		breaker := value.(*CircuitBreaker)
		breaker.Reset()
		return true
	})
}

