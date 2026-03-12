package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var explainGroup string

var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Show detailed breakdown of an equivalence group",
	Long:  "Displays member markets, match method, confidence score, flags, and rationale for a given equivalence group.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Explaining group: %s\n", explainGroup)
		// TODO: implement group explanation
		return nil
	},
}

func init() {
	explainCmd.Flags().StringVar(&explainGroup, "group", "", "Equivalence group ID (required)")
	explainCmd.MarkFlagRequired("group")
}
