package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check health of all venue adapters and ingestion stats",
	Long:  "Reports the health status of each venue API, circuit breaker state, and database statistics.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Checking system status...")
		// TODO: implement status check
		return nil
	},
}
