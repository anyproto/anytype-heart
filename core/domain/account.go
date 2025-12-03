package domain

import (
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const anytypeMetadataPath = "m/SLIP-0021/anytype/account/metadata"

func DeriveAccountMetadata(acc crypto.PrivKey) (*model.Metadata, crypto.SymKey, error) {
	symKey, err := deriveAccountEncKey(acc)
	if err != nil {
		return nil, nil, err
	}
	symKeyProto, err := symKey.Marshall()
	if err != nil {
		return nil, nil, err
	}
	return &model.Metadata{
		Payload: &model.MetadataPayloadOfIdentity{
			Identity: &model.MetadataPayloadIdentityPayload{
				ProfileSymKey: symKeyProto,
			},
		},
	}, symKey, nil
}

func deriveAccountEncKey(accKey crypto.PrivKey) (crypto.SymKey, error) {
	raw, err := accKey.Raw()
	if err != nil {
		return nil, err
	}
	return crypto.DeriveSymmetricKey(raw, anytypeMetadataPath)
}
