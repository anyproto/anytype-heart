package metricsid

import (
	"testing"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/strkey"
	"github.com/stretchr/testify/require"
)

func TestMetrics(t *testing.T) {
	privKey, _, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)
	res, err := DeriveMetricsId(privKey)
	require.NoError(t, err)
	decoded, err := strkey.Decode(metricsVersionByte, res)
	require.NoError(t, err)
	_, err = crypto.NewSigningEd25519PubKeyFromBytes(decoded)
	require.NoError(t, err)
}
