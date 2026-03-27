package adapters

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"equinox/models"
)

// CircuitState represents the state of the circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // normal operation
	CircuitOpen                         // rejecting requests
	CircuitHalfOpen                     // probing with a single request
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "CLOSED"
	case CircuitOpen:
		return "OPEN"
	case CircuitHalfOpen:
		return "HALF-OPEN"
	default:
		return "UNKNOWN"
	}
}

const (
	circuitOpenThreshold = 5
	circuitCooldown      = 60 * time.Second
)

// CircuitBreaker implements a three-state circuit breaker per venue.
type CircuitBreaker struct {
	mu                sync.Mutex
	venue             models.Venue
	state             CircuitState
	consecutiveErrors int
	lastFailure       time.Time
	lastError         error
}

func NewCircuitBreaker(venue models.Venue) *CircuitBreaker {
	return &CircuitBreaker{
		venue: venue,
		state: CircuitClosed,
	}
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.checkCooldown()
	return cb.state
}

// Allow checks if a request is allowed. Returns an error if the circuit is open.
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.checkCooldown()

	switch cb.state {
	case CircuitClosed:
		return nil
	case CircuitHalfOpen:
		return nil
	case CircuitOpen:
		return &AdapterError{
			Venue:      cb.venue,
			Type:       T6Suspension,
			Attempts:   0,
			LastError:  fmt.Errorf("circuit breaker OPEN: %v", cb.lastError),
			OccurredAt: time.Now(),
		}
	}
	return nil
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitHalfOpen {
		slog.Info("circuit breaker closed", "venue", cb.venue)
	}
	cb.state = CircuitClosed
	cb.consecutiveErrors = 0
}

// RecordFailure records a failed request and potentially opens the circuit.
func (cb *CircuitBreaker) RecordFailure(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveErrors++
	cb.lastFailure = time.Now()
	cb.lastError = err

	if cb.consecutiveErrors >= circuitOpenThreshold && cb.state != CircuitOpen {
		cb.state = CircuitOpen
		slog.Error("circuit breaker opened",
			"venue", cb.venue,
			"consecutive_errors", cb.consecutiveErrors,
			"last_error", err,
		)
	}
}

// checkCooldown transitions from OPEN to HALF-OPEN after cooldown. Must hold mu.
func (cb *CircuitBreaker) checkCooldown() {
	if cb.state == CircuitOpen && time.Since(cb.lastFailure) >= circuitCooldown {
		cb.state = CircuitHalfOpen
		slog.Info("circuit breaker half-open, probing", "venue", cb.venue)
	}
}
