package config

import (
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Verify defaults from PRD
	if cfg.KalshiBaseURL != "https://demo-api.kalshi.co/trade-api/v2" {
		t.Errorf("KalshiBaseURL = %q, want sandbox URL", cfg.KalshiBaseURL)
	}
	if cfg.PolymarketGammaURL != "https://gamma-api.polymarket.com" {
		t.Errorf("PolymarketGammaURL = %q", cfg.PolymarketGammaURL)
	}
	if cfg.PolymarketCLOBURL != "https://clob.polymarket.com" {
		t.Errorf("PolymarketCLOBURL = %q", cfg.PolymarketCLOBURL)
	}
	if cfg.SQLiteDBPath != "./equinox.db" {
		t.Errorf("SQLiteDBPath = %q", cfg.SQLiteDBPath)
	}
	if cfg.PollIntervalSeconds != 60 {
		t.Errorf("PollIntervalSeconds = %d, want 60", cfg.PollIntervalSeconds)
	}

	// Equivalence thresholds
	if cfg.EmbeddingSimilarityHigh != 0.92 {
		t.Errorf("EmbeddingSimilarityHigh = %f, want 0.92", cfg.EmbeddingSimilarityHigh)
	}
	if cfg.EmbeddingSimilarityLow != 0.78 {
		t.Errorf("EmbeddingSimilarityLow = %f, want 0.78", cfg.EmbeddingSimilarityLow)
	}
	if cfg.JaccardThreshold != 0.25 {
		t.Errorf("JaccardThreshold = %f, want 0.25", cfg.JaccardThreshold)
	}
	if cfg.LevenshteinThreshold != 0.40 {
		t.Errorf("LevenshteinThreshold = %f, want 0.40", cfg.LevenshteinThreshold)
	}
	if cfg.ResolutionWindowHours != 48 {
		t.Errorf("ResolutionWindowHours = %d, want 48", cfg.ResolutionWindowHours)
	}

	// Routing weights should sum to 1.0
	sum := cfg.WeightPriceQuality + cfg.WeightLiquidity + cfg.WeightSpreadQuality + cfg.WeightMarketStatus
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("routing weights sum = %f, want 1.0", sum)
	}

	if cfg.StalenessLiquidityHaircut != 0.20 {
		t.Errorf("StalenessLiquidityHaircut = %f, want 0.20", cfg.StalenessLiquidityHaircut)
	}

	// Logging
	if cfg.LogLevel != "INFO" {
		t.Errorf("LogLevel = %q, want INFO", cfg.LogLevel)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want json", cfg.LogFormat)
	}
}

func TestInitLogger(t *testing.T) {
	// Verify InitLogger doesn't panic for each format/level combo
	cases := []struct {
		level  string
		format string
	}{
		{"DEBUG", "json"},
		{"INFO", "json"},
		{"WARN", "text"},
		{"ERROR", "text"},
		{"INVALID", "json"},
	}
	for _, tc := range cases {
		t.Run(tc.level+"_"+tc.format, func(t *testing.T) {
			cfg := &Config{LogLevel: tc.level, LogFormat: tc.format}
			InitLogger(cfg) // should not panic
		})
	}
}
