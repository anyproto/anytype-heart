package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func createAccount(t *testing.T) Service {
	mnemonic, err := WalletGenerateMnemonic(12)
	require.NoError(t, err)

	account, err := WalletAccountAt(mnemonic,0,"")
	require.NoError(t, err)
	rootPath := filepath.Join(os.TempDir(), "anytype")
	err = WalletInitRepo(rootPath, account.Seed())
	require.NoError(t, err)

	anytype, err := New(rootPath, account.Address())
	require.NoError(t, err)

	return anytype
}
