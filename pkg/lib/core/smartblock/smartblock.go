package smartblock

import (
	"errors"
	"fmt"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type SmartBlockType uint64

const (
	SmartBlockTypeAccountOld = SmartBlockType(model.SmartBlockType_AccountOld)

	SmartBlockTypePage                = SmartBlockType(model.SmartBlockType_Page)
	SmartBlockTypeProfilePage         = SmartBlockType(model.SmartBlockType_ProfilePage)
	SmartBlockTypeHome                = SmartBlockType(model.SmartBlockType_Home)
	SmartBlockTypeArchive             = SmartBlockType(model.SmartBlockType_Archive)
	SmartBlockTypeSet                 = SmartBlockType(model.SmartBlockType_Set)
	SmartBlockTypeObjectType          = SmartBlockType(model.SmartBlockType_STObjectType)
	SmartBlockTypeFile                = SmartBlockType(model.SmartBlockType_File)
	SmartblockTypeMarketplaceType     = SmartBlockType(model.SmartBlockType_MarketplaceType)
	SmartblockTypeMarketplaceRelation = SmartBlockType(model.SmartBlockType_MarketplaceRelation)
	SmartblockTypeMarketplaceTemplate = SmartBlockType(model.SmartBlockType_MarketplaceTemplate)
	SmartBlockTypeTemplate            = SmartBlockType(model.SmartBlockType_Template)
	SmartBlockTypeBundledTemplate     = SmartBlockType(model.SmartBlockType_BundledTemplate)
	SmartBlockTypeBundledRelation     = SmartBlockType(model.SmartBlockType_BundledRelation)
	SmartBlockTypeSubObject           = SmartBlockType(model.SmartBlockType_SubObject)
	SmartBlockTypeBundledObjectType   = SmartBlockType(model.SmartBlockType_BundledObjectType)
	SmartBlockTypeAnytypeProfile      = SmartBlockType(model.SmartBlockType_AnytypeProfile)
	SmartBlockTypeDate                = SmartBlockType(model.SmartBlockType_Date)
	SmartBlockTypeBreadcrumbs         = SmartBlockType(model.SmartBlockType_Breadcrumbs)
	SmartBlockTypeWorkspaceOld        = SmartBlockType(model.SmartBlockType_WorkspaceOld) // deprecated thread-based workspaces
	SmartBlockTypeWorkspace           = SmartBlockType(model.SmartBlockType_Workspace)
	SmartBlockTypeWidget              = SmartBlockType(model.SmartBlockType_Widget)
)

var ErrNoSuchSmartblock = errors.New("this id does not relate to any smartblock type")

func SmartBlockTypeFromID(id string) (SmartBlockType, error) {
	if strings.HasPrefix(id, addr.BundledRelationURLPrefix) {
		return SmartBlockTypeBundledRelation, nil
	}

	if strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) {
		return SmartBlockTypeBundledObjectType, nil
	}

	if len(strings.Split(id, addr.SubObjectCollectionIdSeparator)) == 2 {
		return SmartBlockTypeSubObject, nil
	}

	// workaround for options that have no prefix
	// todo: remove this after migration to the new records format
	if bson.IsObjectIdHex(id) {
		return SmartBlockTypeSubObject, nil
	}

	if strings.HasPrefix(id, addr.AnytypeProfileId) {
		return SmartBlockTypeProfilePage, nil
	}
	if strings.HasPrefix(id, addr.VirtualPrefix) {
		sbt, err := addr.ExtractVirtualSourceType(id)
		if err != nil {
			return 0, err
		}
		return SmartBlockType(sbt), nil
	}
	if strings.HasPrefix(id, addr.DatePrefix) {
		return SmartBlockTypeDate, nil
	}

	c, err := cid.Decode(id)
	if err != nil {
		return SmartBlockTypePage, err
	}
	// TODO: discard this fragile condition as soon as we will move to the multiaddr with prefix
	if c.Prefix().Codec == cid.DagProtobuf && c.Prefix().MhType == multihash.SHA2_256 {
		return SmartBlockTypeFile, nil
	}
	if c.Prefix().Codec == cid.DagCBOR {
		return SmartBlockTypePage, nil
	}

	return SmartBlockTypePage, ErrNoSuchSmartblock
}

func SmartBlockTypeFromThreadID(tid thread.ID) (SmartBlockType, error) {
	panic("should not be used")
}

// Panics in case of incorrect sb type!
func (sbt SmartBlockType) ToProto() model.SmartBlockType {
	return model.SmartBlockType(sbt)
}

func (sbt SmartBlockType) Valid() (err error) {
	if _, ok := model.SmartBlockType_name[int32(sbt)]; ok {
		return nil
	}
	return fmt.Errorf("unknown smartblock type")
}

func (sbt SmartBlockType) IsOneOf(sbts ...SmartBlockType) bool {
	for _, t := range sbts {
		if t == sbt {
			return true
		}
	}
	return false
}

// Indexable determines if the object of specific type need to be proceeded by the indexer in order to appear in sets
func (sbt SmartBlockType) Indexable() (details, outgoingLinks bool) {
	switch sbt {
	case SmartblockTypeMarketplaceType, SmartblockTypeMarketplaceRelation,
		SmartblockTypeMarketplaceTemplate, SmartBlockTypeDate, SmartBlockTypeBreadcrumbs, SmartBlockTypeAccountOld, SmartBlockTypeWorkspaceOld:
		return false, false
	case SmartBlockTypeArchive, SmartBlockTypeHome:
		return false, true
	default:
		return true, true
	}
}
