package main

import (
	"fmt"
	"github.com/spf13/cobra"
)

var CliCmd = &cobra.Command{
	Use:   "cli",
	Short: "CLI utilities",
	Long:  `Various CLI utilities for cafe operations.`,
}

func init() {
	// subcommands
	CliCmd.AddCommand(migrateCmd)
	CliCmd.AddCommand(cafeCmd)
	// local flags
}

func main() {
	if err := CliCmd.Execute(); err != nil {
		fmt.Printf("failed to execute: %s", err.Error())
	}
}
