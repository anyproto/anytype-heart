package core

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/stretchr/testify/require"
)

func createAccount(t require.TestingT) Service {
	mnemonic, err := WalletGenerateMnemonic(12)
	fmt.Printf("mnemonic: %s\n", mnemonic)
	require.NoError(t, err)

	account, err := WalletAccountAt(mnemonic, 0, "")
	fmt.Printf("account 0: %s\n", account.Address())

	require.NoError(t, err)
	rootPath := filepath.Join(os.TempDir(), "anytype")

	rawSeed, err := account.Raw()
	require.NoError(t, err)

	err = WalletInitRepo(rootPath, rawSeed)
	require.NoError(t, err)

	anytype, err := New(rootPath, account.Address())
	require.NoError(t, err)

	return anytype
}
