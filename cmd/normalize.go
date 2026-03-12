package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var normalizeCmd = &cobra.Command{
	Use:   "normalize",
	Short: "Transform raw venue data into canonical market model",
	Long:  "Converts raw market data from all venues into the Canonical Market Model (CMM). Idempotent — safe to re-run.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Normalizing markets...")
		// TODO: implement normalization pipeline
		return nil
	},
}
