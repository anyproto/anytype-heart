package metricsid

import (
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/strkey"

	"github.com/anyproto/anytype-heart/util/privkey"
)

const (
	metricsVersionByte    strkey.VersionByte = 0xce
	MetricsDerivationPath                    = "m/99999'/0'"
)

func encodeMetricsId(pubKey crypto.PubKey) (string, error) {
	raw, err := pubKey.Raw()
	if err != nil {
		return "", err
	}
	return strkey.Encode(metricsVersionByte, raw)
}

func DeriveMetricsId(privKey crypto.PrivKey) (string, error) {
	key, err := privkey.DeriveFromPrivKey(MetricsDerivationPath, privKey)
	if err != nil {
		return "", err
	}
	return encodeMetricsId(key.GetPublic())
}
