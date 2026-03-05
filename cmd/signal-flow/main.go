package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/rvald/signal-flow/cmd/signal-flow/cli"
)

func main() {
	// Load .env file (ignore error if it doesn't exist)
	if err := godotenv.Load(".env"); err != nil && !os.IsNotExist(err) {
		fmt.Println("Warning: Error loading .env file:", err)
	}
	// Create context that cancels on interrupt
	ctx, cancel := context.WithCancel(context.Background())

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		cancel()
	}()

	// Create and execute root command
	rootCmd := cli.NewRootCmd()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		cancel()
		os.Exit(1)
	}

	cancel() // Cleanup on successful exit
}
