package metricsid

import (
	"bytes"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/strkey"
	"github.com/anyproto/go-slip10"
)

const (
	metricsVersionByte    strkey.VersionByte = 0xce
	MetricsDerivationPath                    = "m/99999'/0'/0'"
)

func deriveFromPrivKey(path string, privKey crypto.PrivKey) (key crypto.PrivKey, err error) {
	rawBytes, err := privKey.Raw()
	if err != nil {
		return nil, err
	}
	node, err := slip10.DeriveForPath(path, rawBytes)
	if err != nil {
		return nil, err
	}
	return genKey(node)
}

func genKey(node slip10.Node) (key crypto.PrivKey, err error) {
	reader := bytes.NewReader(node.RawSeed())
	key, _, err = crypto.GenerateEd25519Key(reader)
	return
}

func encodeMetricsId(pubKey crypto.PubKey) (string, error) {
	raw, err := pubKey.Raw()
	if err != nil {
		return "", err
	}
	return strkey.Encode(metricsVersionByte, raw)
}

func DeriveMetricsId(privKey crypto.PrivKey) (string, error) {
	key, err := deriveFromPrivKey(MetricsDerivationPath, privKey)
	if err != nil {
		return "", err
	}
	return encodeMetricsId(key.GetPublic())
}
