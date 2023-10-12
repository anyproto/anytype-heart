package space

import (
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const anytypeMetadataPath = "m/SLIP-0021/anytype/account/metadata"

func deriveAccountMetadata(acc crypto.PrivKey) ([]byte, error) {
	symKey, err := deriveAccountEncKey(acc)
	if err != nil {
		return nil, err
	}
	rawSymKey, err := symKey.Raw()
	if err != nil {
		return nil, err
	}
	metadata := model.MetadataAccount{ProfileSymKey: rawSymKey}
	return metadata.Marshal()
}

func deriveAccountEncKey(accKey crypto.PrivKey) (crypto.SymKey, error) {
	raw, err := accKey.Raw()
	if err != nil {
		return nil, err
	}
	return crypto.DeriveSymmetricKey(raw, anytypeMetadataPath)
}
