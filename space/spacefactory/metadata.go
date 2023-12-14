package spacefactory

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
	symKeyProto, err := symKey.Marshall()
	if err != nil {
		return nil, err
	}
	metadata := &model.Metadata{
		Payload: &model.MetadataPayloadOfIdentity{
			Identity: &model.MetadataPayloadIdentityPayload{
				ProfileSymKey: symKeyProto,
			},
		},
	}
	return metadata.Marshal()
}

func deriveAccountEncKey(accKey crypto.PrivKey) (crypto.SymKey, error) {
	raw, err := accKey.Raw()
	if err != nil {
		return nil, err
	}
	return crypto.DeriveSymmetricKey(raw, anytypeMetadataPath)
}
