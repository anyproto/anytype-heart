package smartblock

import (
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type SmartBlockType uint64

const (
	SmartBlockTypeAccountOld = SmartBlockType(model.SmartBlockType_AccountOld)

	SmartBlockTypePage                 = SmartBlockType(model.SmartBlockType_Page)
	SmartBlockTypeProfilePage          = SmartBlockType(model.SmartBlockType_ProfilePage)
	SmartBlockTypeHome                 = SmartBlockType(model.SmartBlockType_Home)
	SmartBlockTypeArchive              = SmartBlockType(model.SmartBlockType_Archive)
	SmartBlockTypeFile                 = SmartBlockType(model.SmartBlockType_File) // Deprecated
	SmartBlockTypeFileObject           = SmartBlockType(model.SmartBlockType_FileObject)
	SmartBlockTypeTemplate             = SmartBlockType(model.SmartBlockType_Template)
	SmartBlockTypeBundledTemplate      = SmartBlockType(model.SmartBlockType_BundledTemplate)
	SmartBlockTypeBundledRelation      = SmartBlockType(model.SmartBlockType_BundledRelation)
	SmartBlockTypeSubObject            = SmartBlockType(model.SmartBlockType_SubObject)
	SmartBlockTypeBundledObjectType    = SmartBlockType(model.SmartBlockType_BundledObjectType)
	SmartBlockTypeParticipant          = SmartBlockType(model.SmartBlockType_Participant)
	SmartBlockTypeAnytypeProfile       = SmartBlockType(model.SmartBlockType_AnytypeProfile)
	SmartBlockTypeDate                 = SmartBlockType(model.SmartBlockType_Date)
	SmartBlockTypeIdentity             = SmartBlockType(model.SmartBlockType_Identity)
	SmartBlockTypeChatObjectDeprecated = SmartBlockType(model.SmartBlockType_ChatObjectDeprecated) // deprecated. Container for any-store based chats
	SmartBlockTypeChatDerivedObject    = SmartBlockType(model.SmartBlockType_ChatDerivedObject)    // Any-store based object for chat
	SmartBlockTypeAccountObject        = SmartBlockType(model.SmartBlockType_AccountObject)

	SmartBlockTypeWorkspace      = SmartBlockType(model.SmartBlockType_Workspace)
	SmartBlockTypeWidget         = SmartBlockType(model.SmartBlockType_Widget)
	SmartBlockTypeRelation       = SmartBlockType(model.SmartBlockType_STRelation)
	SmartBlockTypeObjectType     = SmartBlockType(model.SmartBlockType_STType)
	SmartBlockTypeSpaceView      = SmartBlockType(model.SmartBlockType_SpaceView)
	SmartBlockTypeRelationOption = SmartBlockType(model.SmartBlockType_STRelationOption)

	SmartBlockTypeMissingObject      = SmartBlockType(model.SmartBlockType_MissingObject)
	SmartBlockTypeNotificationObject = SmartBlockType(model.SmartBlockType_NotificationObject)
	SmartBlockTypeDevicesObject      = SmartBlockType(model.SmartBlockType_DevicesObject)
)

var ErrNoSuchSmartblock = errors.New("this id does not relate to any smartblock type")

func (sbt SmartBlockType) String() string {
	return sbt.ToProto().String()
}

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
func (sbt SmartBlockType) Indexable() (fulltext, details, outgoingLinks bool) {
	switch sbt {
	case SmartBlockTypeDate, SmartBlockTypeAccountOld, SmartBlockTypeNotificationObject, SmartBlockTypeDevicesObject:
		return false, false, false
	case SmartBlockTypeWidget, SmartBlockTypeArchive, SmartBlockTypeHome:
		return false, true, false
	case SmartBlockTypeWorkspace,
		SmartBlockTypeAccountObject,
		SmartBlockTypeChatObjectDeprecated,
		SmartBlockTypeSpaceView:
		return false, true, true
	default:
		return true, true, true
	}
}
