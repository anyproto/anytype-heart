package smartblock

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/textileio/go-threads/core/thread"

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
	SmartBlockTypeCollection          = SmartBlockType(model.SmartBlockType_Collection)
)

var ErrNoSuchSmartblock = errors.New("this id does not relate to any smartblock type")

func PatchSmartBlockType(id string, sbt SmartBlockType) (string, error) {
	tid, err := thread.Decode(id)
	if err != nil {
		return id, err
	}
	rawid := []byte(tid.KeyString())
	ver, n := binary.Uvarint(rawid)
	variant, n2 := binary.Uvarint(rawid[n:])
	_, n3 := binary.Uvarint(rawid[n+n2:])
	finalN := n + n2 + n3
	buf := make([]byte, 3*binary.MaxVarintLen64+len(rawid)-finalN)
	n = binary.PutUvarint(buf, ver)
	n += binary.PutUvarint(buf[n:], variant)
	n += binary.PutUvarint(buf[n:], uint64(sbt))
	copy(buf[n:], rawid[finalN:])
	if tid, err = thread.Cast(buf[:n+len(rawid)-finalN]); err != nil {
		return id, err
	}
	return tid.String(), nil
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
