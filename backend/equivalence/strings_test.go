package equivalence

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		expected float64
		delta    float64
	}{
		{"identical", "will bitcoin exceed 100k", "will bitcoin exceed 100k", 1.0, 0.001},
		{"no overlap", "cat dog", "fish bird", 0.0, 0.001},
		{"partial overlap", "will bitcoin exceed 100k", "will bitcoin hit 100k", 0.6, 0.1},
		{"empty both", "", "", 1.0, 0.001},
		{"empty one", "test", "", 0.0, 0.001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JaccardSimilarity(tt.a, tt.b)
			assert.InDelta(t, tt.expected, result, tt.delta)
		})
	}
}

func TestNormalizedLevenshtein(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		expected float64
		delta    float64
	}{
		{"identical", "bitcoin", "bitcoin", 1.0, 0.001},
		{"completely different", "abc", "xyz", 0.0, 0.001},
		{"similar", "bitcoin", "bitcoins", 0.875, 0.001},
		{"empty both", "", "", 1.0, 0.001},
		{"empty one", "test", "", 0.0, 0.001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizedLevenshtein(tt.a, tt.b)
			assert.InDelta(t, tt.expected, result, tt.delta)
		})
	}
}

func TestLevenshtein(t *testing.T) {
	assert.Equal(t, 0, levenshtein("kitten", "kitten"))
	assert.Equal(t, 3, levenshtein("kitten", "sitting"))
	assert.Equal(t, 5, levenshtein("", "hello"))
	assert.Equal(t, 5, levenshtein("hello", ""))
}
