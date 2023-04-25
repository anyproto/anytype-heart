package main

import (
	"github.com/spf13/cobra"
)

var cafeCmd = &cobra.Command{
	Use:   "cafe",
	Short: "Cafe-specific commands",
}

var (
	mnemonic string
	account  string
)

var findProfiles = &cobra.Command{
	Use:   "findprofiles",
	Short: "Find profiles by mnemonic or accountId",
	Run: func(c *cobra.Command, args []string) {
		return
	},
}

func init() {
	// subcommands
	cafeCmd.AddCommand(findProfiles)
	findProfiles.PersistentFlags().StringVarP(&mnemonic, "mnemonic", "", "", "mnemonic to find profiles on")
	findProfiles.PersistentFlags().StringVarP(&account, "account", "a", "", "account to find profiles on")
}
