package main

import (
	"fmt"
	"os"

	"github.com/automa-saga/logx"
	"github.com/spf13/cobra"
)

func main() {
	err := logx.Initialize(logx.LoggingConfig{
		Level:          "debug",
		ConsoleLogging: true,
	})
	if err != nil {
		fmt.Printf("Error initializing logger: %v\n", err)
		os.Exit(1)
	}

	// Initialize the root command
	var rootCmd = &cobra.Command{
		Use:   "fetch-prs",
		Short: "Fetch pull requests from GitHub repositories",
		Long:  `A CLI tool to fetch and manage pull requests from GitHub repositories based on date range and other criteria.`,
	}

	rootCmd.AddCommand(fetchCmd)

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		logx.As().Error().Err(err).Msg("Command execution failed")
		os.Exit(1)
	}
}
