package blockchain_health

import (
	"testing"
	"time"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker(3)

	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected initial state CircuitClosed, got %v", cb.GetState())
	}

	if !cb.CanExecute() {
		t.Error("Expected CanExecute=true for new circuit breaker")
	}

	if cb.GetFailureCount() != 0 {
		t.Errorf("Expected failure count=0, got %d", cb.GetFailureCount())
	}
}

func TestCircuitBreaker_FailureThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3)

	// Record 2 failures - should stay closed
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected state CircuitClosed after 2 failures, got %v", cb.GetState())
	}

	if !cb.CanExecute() {
		t.Error("Expected CanExecute=true after 2 failures (threshold=3)")
	}

	// Record 3rd failure - should open
	cb.RecordFailure()

	if cb.GetState() != CircuitOpen {
		t.Errorf("Expected state CircuitOpen after 3 failures, got %v", cb.GetState())
	}

	if cb.CanExecute() {
		t.Error("Expected CanExecute=false when circuit is open")
	}
}

func TestCircuitBreaker_SuccessReset(t *testing.T) {
	cb := NewCircuitBreaker(3)

	// Record failures
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.GetFailureCount() != 2 {
		t.Errorf("Expected failure count=2, got %d", cb.GetFailureCount())
	}

	// Record success - should reset failure count
	cb.RecordSuccess()

	if cb.GetFailureCount() != 0 {
		t.Errorf("Expected failure count=0 after success, got %d", cb.GetFailureCount())
	}

	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected state CircuitClosed after success, got %v", cb.GetState())
	}
}

func TestCircuitBreaker_HalfOpenTransition(t *testing.T) {
	cb := NewCircuitBreaker(1)

	// Trigger circuit open
	cb.RecordFailure()

	if cb.GetState() != CircuitOpen {
		t.Errorf("Expected state CircuitOpen, got %v", cb.GetState())
	}

	// Should not allow execution immediately
	if cb.CanExecute() {
		t.Error("Expected CanExecute=false immediately after opening")
	}

	// Wait for enough time to allow half-open (circuit breaker uses 60s timeout)
	// For testing, we'll need to manipulate the lastFailureTime
	// This is a simplified test - in practice you'd mock time or make timeout configurable
	time.Sleep(10 * time.Millisecond) // Small delay for testing

	// Note: This test would need the circuit breaker to have a configurable timeout
	// for proper testing. For now, we'll just verify the basic state transitions work.
}

func TestCircuitBreaker_HalfOpenSuccess(t *testing.T) {
	cb := NewCircuitBreaker(1)

	// Trigger circuit open
	cb.RecordFailure()

	// Manually transition to half-open for testing
	cb.state = CircuitHalfOpen

	if !cb.CanExecute() {
		t.Error("Expected CanExecute=true in half-open state")
	}

	// Success in half-open should close the circuit
	cb.RecordSuccess()

	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected state CircuitClosed after success in half-open, got %v", cb.GetState())
	}

	if cb.GetFailureCount() != 0 {
		t.Errorf("Expected failure count=0 after successful half-open, got %d", cb.GetFailureCount())
	}
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(1)

	// Manually set to half-open for testing
	cb.state = CircuitHalfOpen

	// Failure in half-open should go back to open
	cb.RecordFailure()

	if cb.GetState() != CircuitOpen {
		t.Errorf("Expected state CircuitOpen after failure in half-open, got %v", cb.GetState())
	}

	if cb.CanExecute() {
		t.Error("Expected CanExecute=false after failure in half-open")
	}
}

func TestCircuitBreaker_MultipleFailuresAndRecovery(t *testing.T) {
	cb := NewCircuitBreaker(2)

	// Scenario: fail -> fail -> open -> success -> closed
	cb.RecordFailure()
	if cb.GetState() != CircuitClosed {
		t.Error("Should stay closed after first failure")
	}

	cb.RecordFailure()
	if cb.GetState() != CircuitOpen {
		t.Error("Should open after hitting threshold")
	}

	// Simulate half-open state
	cb.state = CircuitHalfOpen

	// Success should close it
	cb.RecordSuccess()
	if cb.GetState() != CircuitClosed {
		t.Error("Should close after success in half-open")
	}

	// Should be able to execute again
	if !cb.CanExecute() {
		t.Error("Should be able to execute after recovery")
	}
}
