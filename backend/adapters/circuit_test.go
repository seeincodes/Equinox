package adapters

import (
	"fmt"
	"testing"
	"time"

	"equinox/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_StartsInClosedState(t *testing.T) {
	cb := NewCircuitBreaker(models.VenueKalshi)
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestCircuitBreaker_AllowsWhenClosed(t *testing.T) {
	cb := NewCircuitBreaker(models.VenueKalshi)
	assert.NoError(t, cb.Allow())
}

func TestCircuitBreaker_OpensAfter5Failures(t *testing.T) {
	cb := NewCircuitBreaker(models.VenueKalshi)

	for i := 0; i < 5; i++ {
		cb.RecordFailure(fmt.Errorf("fail %d", i))
	}

	assert.Equal(t, CircuitOpen, cb.State())
	err := cb.Allow()
	require.Error(t, err)

	var adErr *AdapterError
	require.ErrorAs(t, err, &adErr)
	assert.Equal(t, T6Suspension, adErr.Type)
}

func TestCircuitBreaker_DoesNotOpenBefore5Failures(t *testing.T) {
	cb := NewCircuitBreaker(models.VenueKalshi)

	for i := 0; i < 4; i++ {
		cb.RecordFailure(fmt.Errorf("fail %d", i))
	}

	assert.Equal(t, CircuitClosed, cb.State())
	assert.NoError(t, cb.Allow())
}

func TestCircuitBreaker_ResetsOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(models.VenueKalshi)

	for i := 0; i < 3; i++ {
		cb.RecordFailure(fmt.Errorf("fail %d", i))
	}
	cb.RecordSuccess()

	assert.Equal(t, CircuitClosed, cb.State())
	assert.Equal(t, 0, cb.consecutiveErrors)
}

func TestCircuitBreaker_HalfOpenAfterCooldown(t *testing.T) {
	cb := NewCircuitBreaker(models.VenueKalshi)

	for i := 0; i < 5; i++ {
		cb.RecordFailure(fmt.Errorf("fail %d", i))
	}
	assert.Equal(t, CircuitOpen, cb.State())

	// Simulate cooldown elapsed
	cb.mu.Lock()
	cb.lastFailure = time.Now().Add(-61 * time.Second)
	cb.mu.Unlock()

	assert.Equal(t, CircuitHalfOpen, cb.State())
	assert.NoError(t, cb.Allow())
}

func TestCircuitBreaker_HalfOpenToClosedOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(models.VenueKalshi)

	for i := 0; i < 5; i++ {
		cb.RecordFailure(fmt.Errorf("fail %d", i))
	}

	cb.mu.Lock()
	cb.lastFailure = time.Now().Add(-61 * time.Second)
	cb.mu.Unlock()

	assert.Equal(t, CircuitHalfOpen, cb.State())
	cb.RecordSuccess()
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestCircuitBreaker_HalfOpenToOpenOnFailure(t *testing.T) {
	cb := NewCircuitBreaker(models.VenueKalshi)

	for i := 0; i < 5; i++ {
		cb.RecordFailure(fmt.Errorf("fail %d", i))
	}

	cb.mu.Lock()
	cb.lastFailure = time.Now().Add(-61 * time.Second)
	cb.mu.Unlock()

	assert.Equal(t, CircuitHalfOpen, cb.State())
	cb.RecordFailure(fmt.Errorf("probe failed"))
	assert.Equal(t, CircuitOpen, cb.State())
}

func TestCircuitState_String(t *testing.T) {
	assert.Equal(t, "CLOSED", CircuitClosed.String())
	assert.Equal(t, "OPEN", CircuitOpen.String())
	assert.Equal(t, "HALF-OPEN", CircuitHalfOpen.String())
}
