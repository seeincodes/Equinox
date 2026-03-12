package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var matchDryRun bool

var matchCmd = &cobra.Command{
	Use:   "match",
	Short: "Detect equivalent markets across venues",
	Long:  "Runs Stage 1 (rule-based) and Stage 2 (embedding similarity) equivalence detection across all cross-venue market pairs.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if matchDryRun {
			fmt.Println("Running equivalence detection (dry run — no persistence)...")
		} else {
			fmt.Println("Running equivalence detection...")
		}
		// TODO: implement equivalence detection
		return nil
	},
}

func init() {
	matchCmd.Flags().BoolVar(&matchDryRun, "dry-run", false, "Run detection without persisting results")
}
