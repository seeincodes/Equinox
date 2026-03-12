package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	routeMarket string
	routeSide   string
	routeSize   int
)

var routeCmd = &cobra.Command{
	Use:   "route",
	Short: "Simulate a routing decision for an order",
	Long:  "Scores all venues in the equivalence group for the given market and produces a RoutingDecision with human-readable rationale. SimulatedOnly — no real orders placed.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Routing: %s %s size=%d\n", routeSide, routeMarket, routeSize)
		// TODO: implement routing engine
		return nil
	},
}

func init() {
	routeCmd.Flags().StringVar(&routeMarket, "market", "", "Canonical market ID (required)")
	routeCmd.Flags().StringVar(&routeSide, "side", "", "YES or NO (required)")
	routeCmd.Flags().IntVar(&routeSize, "size", 0, "Order size in contracts (required)")
	routeCmd.MarkFlagRequired("market")
	routeCmd.MarkFlagRequired("side")
	routeCmd.MarkFlagRequired("size")
}
