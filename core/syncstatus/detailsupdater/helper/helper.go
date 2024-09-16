package helper

import (
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func InjectsSyncDetails(details *types.Struct, status domain.ObjectSyncStatus, syncError domain.SyncError) *types.Struct {
	details = pbtypes.EnsureStructInited(details)
	if pbtypes.Get(details, bundle.RelationKeySyncStatus.String()) == nil {
		details.Fields[bundle.RelationKeySyncStatus.String()] = pbtypes.Int64(int64(status))
	}
	if pbtypes.Get(details, bundle.RelationKeySyncDate.String()) == nil {
		details.Fields[bundle.RelationKeySyncDate.String()] = pbtypes.Int64(time.Now().Unix())
	}
	if pbtypes.Get(details, bundle.RelationKeySyncError.String()) == nil {
		details.Fields[bundle.RelationKeySyncError.String()] = pbtypes.Int64(int64(syncError))
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
