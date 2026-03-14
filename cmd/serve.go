package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"equinox/api"
	"equinox/store"

	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the REST API server",
	Long:  "Starts an HTTP server exposing market data, equivalence groups, routing, and health endpoints.",
	RunE:  runServe,
}

var serveAddr string

func init() {
	serveCmd.Flags().StringVar(&serveAddr, "addr", ":8080", "Address to listen on (e.g., :8080)")
}

func runServe(cmd *cobra.Command, args []string) error {
	db, err := store.New(cfg.SQLiteDBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	srv := api.NewServer(db, cfg, serveAddr)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		slog.Info("shutting down API server")
		if err := srv.Shutdown(context.Background()); err != nil {
			slog.Error("server shutdown error", "error", err)
		}
	}()

	slog.Info("starting API server", "addr", serveAddr)
	if err := srv.ListenAndServe(); err != nil && err.Error() != "http: Server closed" {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}
