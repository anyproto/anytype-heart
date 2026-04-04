package objectid

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

func TestExistingObject_GetIDAndPayload_MatchByIDOnlyWhenGetExisting(t *testing.T) {
	sf := objectstore.NewStoreFixture(t)
	existing := newExistingObject(sf)

	sf.AddObjects(t, "spaceId", []objectstore.TestObject{
		{
			bundle.RelationKeyId:      domain.String("obj1"),
			bundle.RelationKeyName:    domain.String("name"),
			bundle.RelationKeySpaceId: domain.String("spaceId"),
		},
	})

	sn := &common.Snapshot{
		Snapshot: &common.SnapshotModel{
			SbType: coresb.SmartBlockTypePage,
			Data: &common.StateSnapshot{
				Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyOldAnytypeID: domain.String("obj1"),
				}),
			},
		},
	}

	id, _, err := existing.GetIDAndPayload(context.Background(), "spaceId", sn, false)
	assert.NoError(t, err)
	assert.Equal(t, "", id)

	id, _, err = existing.GetIDAndPayload(context.Background(), "spaceId", sn, true)
	assert.NoError(t, err)
	assert.Equal(t, "obj1", id)
}
