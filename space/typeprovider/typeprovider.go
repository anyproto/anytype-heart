package typeprovider

import (
	"fmt"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"

	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const CName = "space.typeprovider"

var log = logging.Logger(CName)

type SmartBlockTypeProvider interface {
	Type(spaceID string, id string) (smartblock.SmartBlockType, error)
	RegisterStaticType(id string, tp smartblock.SmartBlockType)
}

func SmartblockTypeFromID(id string) (smartblock.SmartBlockType, error) {
	if strings.HasPrefix(id, addr.BundledRelationURLPrefix) {
		return smartblock.SmartBlockTypeBundledRelation, nil
	}

	if strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) {
		return smartblock.SmartBlockTypeBundledObjectType, nil
	}

	if len(strings.Split(id, addr.SubObjectCollectionIdSeparator)) == 2 {
		return smartblock.SmartBlockTypeSubObject, nil
	}

	// workaround for options that have no prefix
	// todo: remove this after migration to the new records format
	if bson.IsObjectIdHex(id) {
		return smartblock.SmartBlockTypeSubObject, nil
	}

	if strings.HasPrefix(id, addr.AnytypeProfileId) {
		return smartblock.SmartBlockTypeProfilePage, nil
	}
	if strings.HasPrefix(id, addr.VirtualPrefix) {
		sbt, err := addr.ExtractVirtualSourceType(id)
		if err != nil {
			return 0, err
		}
		return smartblock.SmartBlockType(sbt), nil
	}
	if strings.HasPrefix(id, addr.DatePrefix) {
		return smartblock.SmartBlockTypeDate, nil
	}

	if strings.HasPrefix(id, addr.MissingObject) {
		return smartblock.SmartBlockTypeMissingObject, nil
	}

	c, err := cid.Decode(id)
	if err != nil {
		return smartblock.SmartBlockTypePage,
			fmt.Errorf("failed to determine smartblock type, objectID: %s, err: %s", id, err.Error())
	}
	// TODO: discard this fragile condition as soon as we will move to the multiaddr with prefix
	if c.Prefix().Codec == cid.DagProtobuf && c.Prefix().MhType == multihash.SHA2_256 {
		return smartblock.SmartBlockTypeFile, nil
	}
	if c.Prefix().Codec == cid.DagCBOR {
		return smartblock.SmartBlockTypePage, nil
	}

	return smartblock.SmartBlockTypePage, smartblock.ErrNoSuchSmartblock
}
