package privkey

import (
	"bytes"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/go-slip10"
)

func DeriveFromPrivKey(path string, privKey crypto.PrivKey) (key crypto.PrivKey, err error) {
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
