package main

import (
	"context"
	app2 "github.com/anytypeio/go-anytype-middleware/app"
	wallet2 "github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe"
	core2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	"github.com/anytypeio/go-anytype-middleware/util/console"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
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
		var (
			appMnemonic string
			appAccount wallet.Keypair
			accountsToFind []string
			err error
		)

		if mnemonic != "" {
			for i:=0; i<10; i++ {
				ac, err := 	core2.WalletAccountAt(mnemonic, i, "")
				if err != nil {
					console.Fatal("failed to get account from provided mnemonic: %s", err.Error())
					return
				}

				accountsToFind = append(accountsToFind, ac.Address())
			}
		} else if account != "" {
			accountsToFind = []string{account}
		} else {
			console.Fatal("no mnemonic or account provided")
			return
		}
		// create temp wallet in order to do requests to cafe
		appMnemonic, err = core2.WalletGenerateMnemonic(12)
		appAccount, err = core2.WalletAccountAt(appMnemonic, 0, "")

		rootPath, err := ioutil.TempDir(os.TempDir(), "anytype_*")
		app := new(app2.App)
		app.Register(wallet2.NewWithRepoPathAndKeys(rootPath, appAccount, nil))
		app.Register(cafe.New())
		at := core2.New()
		app.Register(at)
		err = app.Start()
		if err != nil {
			console.Fatal("failed to start anytype: %s", err.Error())
			return
		}
		var found bool
		var ch = make(chan core2.Profile)
		closeCh := make(chan struct{})
		go func() {
			defer close(closeCh)
			select {
			case profile, ok := <-ch:
				if !ok {
					return
				}
				found = true
				console.Success("got profile: id=%s name=%s", profile.AccountAddr, profile.Name)
			}
		}()
		err = at.FindProfilesByAccountIDs(context.Background(), accountsToFind, ch)
		if err != nil {
			console.Fatal("failed to query cafe: " + err.Error())
		}
		<-closeCh
		if !found {
			console.Fatal("no accounts found on cafe")
		}
	},
}

func init() {
	// subcommands
	cafeCmd.AddCommand(findProfiles)
	findProfiles.PersistentFlags().StringVarP(&mnemonic, "mnemonic", "", "", "mnemonic to find profiles on")
	findProfiles.PersistentFlags().StringVarP(&account, "account", "a", "", "account to find profiles on")
}