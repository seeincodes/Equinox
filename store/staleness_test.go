package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectStalePrices_NoStaleMarkets(t *testing.T) {
	dir := t.TempDir()
	db, err := New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	flagged, err := db.DetectStalePrices()
	require.NoError(t, err)
	assert.Equal(t, 0, flagged)
}

func TestDetectVenueStaleness_NoData(t *testing.T) {
	dir := t.TempDir()
	db, err := New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	stale := db.DetectVenueStaleness("KALSHI", 60*time.Second)
	assert.True(t, stale, "no data should be considered stale")
}

func TestDetectVenueStaleness_RecentData(t *testing.T) {
	dir := t.TempDir()
	db, err := New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Conn().Exec(
		`INSERT INTO raw_markets (venue, native_id, raw_payload, ingested_at) VALUES (?, ?, ?, ?)`,
		"KALSHI", "TEST-1", `{"ticker":"TEST-1"}`, time.Now().UTC().Format(time.RFC3339),
	)
	require.NoError(t, err)

	stale := db.DetectVenueStaleness("KALSHI", 60*time.Second)
	assert.False(t, stale, "recent data should not be stale")
}

func TestDetectVenueStaleness_OldData(t *testing.T) {
	dir := t.TempDir()
	db, err := New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	oldTime := time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	_, err = db.Conn().Exec(
		`INSERT INTO raw_markets (venue, native_id, raw_payload, ingested_at) VALUES (?, ?, ?, ?)`,
		"KALSHI", "TEST-1", `{"ticker":"TEST-1"}`, oldTime,
	)
	require.NoError(t, err)

	stale := db.DetectVenueStaleness("KALSHI", 60*time.Second)
	assert.True(t, stale, "data older than 2× poll interval should be stale")
}

func TestIsPriceUnchanged_InsufficientCycles(t *testing.T) {
	dir := t.TempDir()
	db, err := New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	for i := 0; i < 3; i++ {
		_, err = db.Conn().Exec(
			`INSERT INTO raw_markets (venue, native_id, raw_payload, ingested_at) VALUES (?, ?, ?, ?)`,
			"KALSHI", "TEST-1", `{"ticker":"TEST-1","yes_bid":65}`,
			time.Now().Add(time.Duration(-i)*time.Minute).UTC().Format(time.RFC3339),
		)
		require.NoError(t, err)
	}

	stale, err := db.isPriceUnchanged("KALSHI", "TEST-1", 5)
	require.NoError(t, err)
	assert.False(t, stale, "fewer than 5 cycles should not be stale")
}

func TestIsPriceUnchanged_AllSame(t *testing.T) {
	dir := t.TempDir()
	db, err := New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	for i := 0; i < 5; i++ {
		_, err = db.Conn().Exec(
			`INSERT INTO raw_markets (venue, native_id, raw_payload, ingested_at) VALUES (?, ?, ?, ?)`,
			"KALSHI", "TEST-1", `{"ticker":"TEST-1","yes_bid":65}`,
			time.Now().Add(time.Duration(-i)*time.Minute).UTC().Format(time.RFC3339),
		)
		require.NoError(t, err)
	}

	stale, err := db.isPriceUnchanged("KALSHI", "TEST-1", 5)
	require.NoError(t, err)
	assert.True(t, stale, "5 identical payloads should be stale")
}

func TestIsPriceUnchanged_Different(t *testing.T) {
	dir := t.TempDir()
	db, err := New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	for i := 0; i < 5; i++ {
		payload := fmt.Sprintf(`{"ticker":"TEST-1","yes_bid":%d}`, 60+i)
		_, err = db.Conn().Exec(
			`INSERT INTO raw_markets (venue, native_id, raw_payload, ingested_at) VALUES (?, ?, ?, ?)`,
			"KALSHI", "TEST-1", payload,
			time.Now().Add(time.Duration(-i)*time.Minute).UTC().Format(time.RFC3339),
		)
		require.NoError(t, err)
	}

	stale, err := db.isPriceUnchanged("KALSHI", "TEST-1", 5)
	require.NoError(t, err)
	assert.False(t, stale, "varying payloads should not be stale")
}
