package helper

import (
	"time"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func InjectsSyncDetails(details *domain.Details, status domain.ObjectSyncStatus, syncError domain.SyncError) *domain.Details {
	if details == nil {
		details = domain.NewDetails()
	}
	if !details.Has(bundle.RelationKeySyncStatus) {
		details.Set(bundle.RelationKeySyncStatus, pbtypes.Int64(int64(status)))
	}
	if !details.Has(bundle.RelationKeySyncDate) {
		details.Set(bundle.RelationKeySyncDate, pbtypes.Int64(time.Now().Unix()))
	}
	if !details.Has(bundle.RelationKeySyncError) {
		details.Set(bundle.RelationKeySyncError, pbtypes.Int64(int64(syncError)))
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
	}
}
