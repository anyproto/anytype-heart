package objectid

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestFileObject_GetIDAndPayload_KeepExistingIDOnReplace(t *testing.T) {
	sf := objectstore.NewStoreFixture(t)
	spaceID := "spaceId"
	oldAnytypeID := "old-file-object-id"
	existingID := "existing-file-object-id"

	sf.AddObjects(t, spaceID, []objectstore.TestObject{
		{
			bundle.RelationKeyId:           domain.String(existingID),
			bundle.RelationKeyOldAnytypeID: domain.String(oldAnytypeID),
			bundle.RelationKeySpaceId:      domain.String(spaceID),
			bundle.RelationKeyResolvedLayout: domain.Int64(
				int64(model.ObjectType_file),
			),
		},
	})

	sn := &common.Snapshot{
		Id: oldAnytypeID,
		Snapshot: &common.SnapshotModel{
			SbType: coresb.SmartBlockTypeFileObject,
			Data: &common.StateSnapshot{
				Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId:           domain.String(oldAnytypeID),
					bundle.RelationKeyOldAnytypeID: domain.String(oldAnytypeID),
					bundle.RelationKeySource:       domain.String("/tmp/backup-file.png"),
				}),
			},
		},
	}

	f := &fileObject{
		treeObject:   newTreeObject(newExistingObject(sf), nil),
		blockService: nil,
	}

	gotID, _, err := f.GetIDAndPayload(
		context.Background(),
		spaceID,
		sn,
		time.Now(),
		true,
		objectorigin.Import(model.Import_Pb),
	)

	require.NoError(t, err)
	assert.Equal(t, existingID, gotID)
}

