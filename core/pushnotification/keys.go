package pushnotification

import (
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/strkey"

	"github.com/anyproto/anytype-heart/util/privkey"
)

const (
	spaceKeyPath = "m/99999'/1'"
	spaceVersion = 0xB5
	spacePath    = "m/SLIP-0021/anytype/space/key"
)

func deriveSpaceKey(firstMetadataKey crypto.PrivKey) (string, crypto.PrivKey, error) {
	key, err := privkey.DeriveFromPrivKey(spaceKeyPath, firstMetadataKey)
	if err != nil {
		return "", nil, err
	}
	rawKey, err := key.GetPublic().Raw()
	if err != nil {
		return "", nil, err
	}
	encodedKey, err := strkey.Encode(spaceVersion, rawKey)
	if err != nil {
		return "", nil, err
	}
	return encodedKey, key, nil
}

func deriveSymmetricKey(readKey crypto.SymKey) (crypto.SymKey, error) {
	raw, err := readKey.Raw()
	if err != nil {
		return nil, err
	}
	return crypto.DeriveSymmetricKey(raw, spacePath)
}
