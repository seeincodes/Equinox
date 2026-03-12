package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var ingestVenue string

var ingestCmd = &cobra.Command{
	Use:   "ingest",
	Short: "Fetch raw market data from venue APIs",
	Long:  "Fetches market metadata and pricing from Kalshi and/or Polymarket and stores raw responses in SQLite.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Ingesting markets...")
		// TODO: implement ingest pipeline
		return nil
	},
}

func init() {
	ingestCmd.Flags().StringVar(&ingestVenue, "venue", "", "Filter by venue: kalshi or polymarket (default: both)")
}
