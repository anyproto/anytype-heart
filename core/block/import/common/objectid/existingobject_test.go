package objectid

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

func TestExistingObject_GetIDAndPayload_MatchByIDOnlyWhenGetExisting(t *testing.T) {
	setup := func(t *testing.T) (*existingObject, *common.Snapshot) {
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
		return existing, sn
	}

	t.Run("getExisting false does not match by object id", func(t *testing.T) {
		existing, sn := setup(t)

		id, _, err := existing.GetIDAndPayload(context.Background(), "spaceId", sn, false)
		require.NoError(t, err)
		assert.Equal(t, "", id)
	})

	t.Run("getExisting true matches by object id", func(t *testing.T) {
		existing, sn := setup(t)

		id, _, err := existing.GetIDAndPayload(context.Background(), "spaceId", sn, true)
		require.NoError(t, err)
		assert.Equal(t, "obj1", id)
	})
}

func TestExistingObject_GetIDAndPayload_MatchBySnapshotIDWhenOldAnytypeIDMissing(t *testing.T) {
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
			SbType: coresb.SmartBlockTypeFileObject,
			Data: &common.StateSnapshot{
				Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId: domain.String("obj1"),
				}),
			},
		},
	}

	id, _, err := existing.GetIDAndPayload(context.Background(), "spaceId", sn, true)
	require.NoError(t, err)
	assert.Equal(t, "obj1", id)
}

func TestExistingObject_GetIDAndPayload_GetExistingPrefersObjectIDOverOldAnytypeID(t *testing.T) {
	sf := objectstore.NewStoreFixture(t)
	existing := newExistingObject(sf)

	// Simulate a prior import-created clone that references original id via oldAnytypeID.
	sf.AddObjects(t, "spaceId", []objectstore.TestObject{
		{
			bundle.RelationKeyId:      domain.String("original-file-object-id"),
			bundle.RelationKeySpaceId: domain.String("spaceId"),
		},
		{
			bundle.RelationKeyId:           domain.String("imported-clone-id"),
			bundle.RelationKeyOldAnytypeID: domain.String("original-file-object-id"),
			bundle.RelationKeySpaceId:      domain.String("spaceId"),
		},
	})

	sn := &common.Snapshot{
		Snapshot: &common.SnapshotModel{
			SbType: coresb.SmartBlockTypeFileObject,
			Data: &common.StateSnapshot{
				Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId:           domain.String("original-file-object-id"),
					bundle.RelationKeyOldAnytypeID: domain.String("original-file-object-id"),
				}),
			},
		},
	}

	id, _, err := existing.GetIDAndPayload(context.Background(), "spaceId", sn, true)
	require.NoError(t, err)
	assert.Equal(t, "original-file-object-id", id)
}
