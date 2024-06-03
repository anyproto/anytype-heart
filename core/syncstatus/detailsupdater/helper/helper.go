package helper

import (
	"time"

	"github.com/gogo/protobuf/types"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func IsSyncRelationRequired(sbType smartblock.SmartBlockType) bool {
	smartblockTypes := SyncRelationsSmartblockTypes()
	return slices.Contains(smartblockTypes, sbType)
}

func InjectsSyncDetails(details *types.Struct, status domain.SyncStatus) {
	if details == nil || details.Fields == nil {
		details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	if pbtypes.Get(details, bundle.RelationKeySyncStatus.String()) == nil {
		details.Fields[bundle.RelationKeySyncStatus.String()] = pbtypes.Int64(int64(status))
	}
	if pbtypes.Get(details, bundle.RelationKeySyncDate.String()) == nil {
		details.Fields[bundle.RelationKeySyncDate.String()] = pbtypes.Int64(time.Now().Unix())
	}
	if pbtypes.Get(details, bundle.RelationKeySyncError.String()) == nil {
		details.Fields[bundle.RelationKeySyncError.String()] = pbtypes.Int64(int64(domain.Null))
	}
}

func SyncRelationsSmartblockTypes() []smartblock.SmartBlockType {
	return []smartblock.SmartBlockType{
		smartblock.SmartBlockTypeObjectType,
		smartblock.SmartBlockTypeRelation,
		smartblock.SmartBlockTypeRelationOption,
		smartblock.SmartBlockTypeFileObject,

		smartblock.SmartBlockTypePage,
		smartblock.SmartBlockTypeTemplate,
		smartblock.SmartBlockTypeSpaceView,
		smartblock.SmartBlockTypeProfilePage,
	}
}
