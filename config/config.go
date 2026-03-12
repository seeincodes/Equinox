package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	// Kalshi
	KalshiBaseURL string `mapstructure:"KALSHI_BASE_URL"`
	KalshiAPIKey  string `mapstructure:"KALSHI_API_KEY"`

	// Polymarket
	PolymarketGammaURL string `mapstructure:"POLYMARKET_GAMMA_URL"`
	PolymarketCLOBURL  string `mapstructure:"POLYMARKET_CLOB_URL"`

	// OpenAI
	OpenAIAPIKey string `mapstructure:"OPENAI_API_KEY"`

	// Database
	SQLiteDBPath string `mapstructure:"SQLITE_DB_PATH"`

	// Polling
	PollIntervalSeconds int `mapstructure:"POLL_INTERVAL_SECONDS"`

	// Equivalence thresholds
	EmbeddingSimilarityHigh float64 `mapstructure:"EMBEDDING_SIMILARITY_HIGH"`
	EmbeddingSimilarityLow  float64 `mapstructure:"EMBEDDING_SIMILARITY_LOW"`
	JaccardThreshold        float64 `mapstructure:"JACCARD_THRESHOLD"`
	LevenshteinThreshold    float64 `mapstructure:"LEVENSHTEIN_THRESHOLD"`
	ResolutionWindowHours   int     `mapstructure:"RESOLUTION_WINDOW_HOURS"`

	// Routing weights
	WeightPriceQuality       float64 `mapstructure:"WEIGHT_PRICE_QUALITY"`
	WeightLiquidity          float64 `mapstructure:"WEIGHT_LIQUIDITY"`
	WeightSpreadQuality      float64 `mapstructure:"WEIGHT_SPREAD_QUALITY"`
	WeightMarketStatus       float64 `mapstructure:"WEIGHT_MARKET_STATUS"`
	StalenessLiquidityHaircut float64 `mapstructure:"STALENESS_LIQUIDITY_HAIRCUT"`

	// Logging
	LogLevel  string `mapstructure:"LOG_LEVEL"`
	LogFormat string `mapstructure:"LOG_FORMAT"`
}

func Load() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")

	// Read .env file (optional — env vars take precedence)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Only warn, don't fail — env vars may be set directly
			slog.Warn("could not read .env file", "error", err)
		}
	}

	// Env vars override .env file
	viper.AutomaticEnv()

	// Defaults
	viper.SetDefault("KALSHI_BASE_URL", "https://demo-api.kalshi.co/trade-api/v2")
	viper.SetDefault("POLYMARKET_GAMMA_URL", "https://gamma-api.polymarket.com")
	viper.SetDefault("POLYMARKET_CLOB_URL", "https://clob.polymarket.com")
	viper.SetDefault("SQLITE_DB_PATH", "./equinox.db")
	viper.SetDefault("POLL_INTERVAL_SECONDS", 60)
	viper.SetDefault("EMBEDDING_SIMILARITY_HIGH", 0.92)
	viper.SetDefault("EMBEDDING_SIMILARITY_LOW", 0.78)
	viper.SetDefault("JACCARD_THRESHOLD", 0.25)
	viper.SetDefault("LEVENSHTEIN_THRESHOLD", 0.40)
	viper.SetDefault("RESOLUTION_WINDOW_HOURS", 48)
	viper.SetDefault("WEIGHT_PRICE_QUALITY", 0.40)
	viper.SetDefault("WEIGHT_LIQUIDITY", 0.35)
	viper.SetDefault("WEIGHT_SPREAD_QUALITY", 0.15)
	viper.SetDefault("WEIGHT_MARKET_STATUS", 0.10)
	viper.SetDefault("STALENESS_LIQUIDITY_HAIRCUT", 0.20)
	viper.SetDefault("LOG_LEVEL", "INFO")
	viper.SetDefault("LOG_FORMAT", "json")

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}

// InitLogger sets up slog based on config.
func InitLogger(cfg *Config) {
	var level slog.Level
	switch strings.ToUpper(cfg.LogLevel) {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if strings.ToLower(cfg.LogFormat) == "text" {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}

	slog.SetDefault(slog.New(handler))
}
