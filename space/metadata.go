package space

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const anytypeMetadataPath = "m/SLIP-0021/anytype/account/metadata"

var deriveMetadata = deriveAccountMetadata

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

func getRepKey(spaceId string) (uint64, error) {
	sepIdx := strings.Index(spaceId, ".")
	if sepIdx == -1 {
		return 0, fmt.Errorf("space id is incorrect")
	}
	return strconv.ParseUint(spaceId[sepIdx+1:], 36, 64)
}
