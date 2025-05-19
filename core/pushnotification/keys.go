package pushnotification

import (
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/util/privkey"
)

const (
	spaceKeyPath = "m/99999'/1'"
	spacePath    = "m/SLIP-0021/anytype/space/key"
)

func deriveSpaceKey(firstMetadataKey crypto.PrivKey) (crypto.PrivKey, error) {
	key, err := privkey.DeriveFromPrivKey(spaceKeyPath, firstMetadataKey)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func deriveSymmetricKey(readKey crypto.SymKey) (crypto.SymKey, error) {
	raw, err := readKey.Raw()
	if err != nil {
		return nil, err
	}
	return crypto.DeriveSymmetricKey(raw, spacePath)
}
