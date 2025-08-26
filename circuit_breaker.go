package blockchain_health

import (
	"time"
)

// NewCircuitBreaker creates a new circuit breaker with the specified failure threshold
func NewCircuitBreaker(failureThreshold int) *CircuitBreaker {
	return &CircuitBreaker{
		failureThreshold: failureThreshold,
		state:            CircuitClosed,
	}
}

// CanExecute returns true if the circuit breaker allows execution
func (cb *CircuitBreaker) CanExecute() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if enough time has passed to try half-open
		if time.Since(cb.lastFailureTime) > 60*time.Second {
			cb.mutex.RUnlock()
			cb.mutex.Lock()
			// Double-check after acquiring write lock
			if cb.state == CircuitOpen && time.Since(cb.lastFailureTime) > 60*time.Second {
				cb.state = CircuitHalfOpen
			}
			cb.mutex.Unlock()
			cb.mutex.RLock()
			return cb.state == CircuitHalfOpen
		}
		return false
	case CircuitHalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	switch cb.state {
	case CircuitHalfOpen:
		// Success in half-open state moves to closed
		cb.state = CircuitClosed
		cb.failureCount = 0
	case CircuitClosed:
		// Reset failure count on success
		cb.failureCount = 0
	}
}

// RecordFailure records a failed operation
func (cb *CircuitBreaker) RecordFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitClosed:
		if cb.failureCount >= cb.failureThreshold {
			cb.state = CircuitOpen
		}
	case CircuitHalfOpen:
		// Any failure in half-open state goes back to open
		cb.state = CircuitOpen
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// GetFailureCount returns the current failure count
func (cb *CircuitBreaker) GetFailureCount() int {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.failureCount
}
