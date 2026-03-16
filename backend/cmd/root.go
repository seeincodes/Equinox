package cmd

import (
	"equinox/config"
	"log/slog"

	"github.com/spf13/cobra"
)

var cfg *config.Config

var rootCmd = &cobra.Command{
	Use:   "equinox",
	Short: "Cross-venue prediction market aggregation & routing",
	Long:  "Equinox connects to Kalshi and Polymarket, normalizes markets into a canonical model, detects equivalent markets across venues, and simulates routing decisions.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load()
		if err != nil {
			return err
		}
		config.InitLogger(cfg)
		slog.Debug("config loaded")
		return nil
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(ingestCmd)
	rootCmd.AddCommand(normalizeCmd)
	rootCmd.AddCommand(matchCmd)
	rootCmd.AddCommand(routeCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(explainCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(userCmd)
}
