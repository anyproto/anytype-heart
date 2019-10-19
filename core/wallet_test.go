package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_WalletCreate(t *testing.T) {
	mw := createWallet(t)

	err := mw.Stop()
	require.NoError(t, err, "failed to stop mw")
}

func Test_WalletRecover(t *testing.T) {
	mw := recoverWallet(t, "input blame switch simple fatigue fragile grab goose unusual identify abuse use")

	err := mw.Stop()
	require.NoError(t, err, "failed to stop mw")
}
