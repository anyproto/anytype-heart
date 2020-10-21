package core

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/stretchr/testify/require"
)

func createAccount(t require.TestingT) Service {
	mnemonic, err := WalletGenerateMnemonic(12)
	fmt.Printf("mnemonic: %s\n", mnemonic)
	require.NoError(t, err)

	account, err := WalletAccountAt(mnemonic, 0, "")
	fmt.Printf("account 0: %s\n", account.Address())

	require.NoError(t, err)
	rootPath, err := ioutil.TempDir(os.TempDir(), "anytype_*")
	require.NoError(t, err)

	rawSeed, err := account.Raw()
	require.NoError(t, err)

	err = WalletInitRepo(rootPath, rawSeed)
	require.NoError(t, err)

	var opts = []ServiceOption{WithRootPathAndAccount(rootPath, account.Address())}

	if os.Getenv("ANYTYPE_TEST_OFFLINE") == "1" {
		opts = append(opts, WithOfflineMode(true))
		opts = append(opts, WithoutCafe())
	}

	anytype, err := New(opts...)
	require.NoError(t, err)

	return anytype
}
