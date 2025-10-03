package helper

import (
	"time"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

func InjectsSyncDetails(details *domain.Details, status domain.ObjectSyncStatus, syncError domain.SyncError) *domain.Details {
	if details == nil {
		details = domain.NewDetails()
	}
	if !details.Has(bundle.RelationKeySyncStatus) {
		details.SetInt64(bundle.RelationKeySyncStatus, int64(status))
	}
	if !details.Has(bundle.RelationKeySyncDate) {
		details.SetInt64(bundle.RelationKeySyncDate, time.Now().Unix())
	}
	if !details.Has(bundle.RelationKeySyncError) {
		details.SetInt64(bundle.RelationKeySyncError, int64(syncError))
	}
	return details
}

func SyncRelationsSmartblockTypes() []smartblock.SmartBlockType {
	return []smartblock.SmartBlockType{
		smartblock.SmartBlockTypeObjectType,
		smartblock.SmartBlockTypeRelation,
		smartblock.SmartBlockTypeRelationOption,
		smartblock.SmartBlockTypeFileObject,

		smartblock.SmartBlockTypePage,
		smartblock.SmartBlockTypeTemplate,
		smartblock.SmartBlockTypeProfilePage,
		smartblock.SmartBlockTypeChatDerivedObject,
		smartblock.SmartBlockTypeChatObjectDeprecated,
		smartblock.SmartBlockTypeSpaceView,
	}
}
