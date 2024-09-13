package lastused

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestSetLastUsedDateForInitialType(t *testing.T) {
	isLastUsedDateGreater := func(details1, details2 *types.Struct) bool {
		return pbtypes.GetInt64(details1, bundle.RelationKeyLastUsedDate.String()) > pbtypes.GetInt64(details2, bundle.RelationKeyLastUsedDate.String())
	}

	t.Run("object types are sorted by lastUsedDate in correct order", func(t *testing.T) {
		// given
		ots := []string{
			bundle.TypeKeySet.BundledURL(),
			bundle.TypeKeyNote.BundledURL(),
			bundle.TypeKeyCollection.BundledURL(),
			bundle.TypeKeyTask.BundledURL(),
			bundle.TypeKeyPage.BundledURL(),
			bundle.TypeKeyDiaryEntry.BundledURL(),
			bundle.TypeKeyAudio.BundledURL(),
		}
		rand.Shuffle(len(ots), func(i, j int) {
			ots[i], ots[j] = ots[j], ots[i]
		})
		detailMap := map[string]*types.Struct{}

		// when
		for _, id := range ots {
			details := &types.Struct{Fields: make(map[string]*types.Value)}
			SetLastUsedDateForInitialObjectType(id, details)
			detailMap[id] = details
		}

		// then
		assert.True(t, isLastUsedDateGreater(detailMap[bundle.TypeKeyNote.BundledURL()], detailMap[bundle.TypeKeyPage.BundledURL()]))
		assert.True(t, isLastUsedDateGreater(detailMap[bundle.TypeKeyPage.BundledURL()], detailMap[bundle.TypeKeyTask.BundledURL()]))
		assert.True(t, isLastUsedDateGreater(detailMap[bundle.TypeKeyTask.BundledURL()], detailMap[bundle.TypeKeySet.BundledURL()]))
		assert.True(t, isLastUsedDateGreater(detailMap[bundle.TypeKeySet.BundledURL()], detailMap[bundle.TypeKeyCollection.BundledURL()]))
		assert.True(t, isLastUsedDateGreater(detailMap[bundle.TypeKeyCollection.BundledURL()], detailMap[bundle.TypeKeyAudio.BundledURL()]))
		assert.True(t, isLastUsedDateGreater(detailMap[bundle.TypeKeyCollection.BundledURL()], detailMap[bundle.TypeKeyDiaryEntry.BundledURL()]))
	})
}

func TestUpdateLastUsedDate(t *testing.T) {
	const spaceId = "space"

	ts := time.Now().Unix()

	isLastUsedDateRecent := func(details *types.Struct, deltaSeconds int64) bool {
		return pbtypes.GetInt64(details, bundle.RelationKeyLastUsedDate.String())+deltaSeconds > time.Now().Unix()
	}

	store := objectstore.NewStoreFixture(t)
	store.AddObjects(t, []objectstore.TestObject{
		{
			bundle.RelationKeyId:        pbtypes.String(bundle.RelationKeyCamera.URL()),
			bundle.RelationKeySpaceId:   pbtypes.String(spaceId),
			bundle.RelationKeyUniqueKey: pbtypes.String(bundle.RelationKeyCamera.URL()),
		},
		{
			bundle.RelationKeyId:        pbtypes.String(bundle.TypeKeyDiaryEntry.URL()),
			bundle.RelationKeySpaceId:   pbtypes.String(spaceId),
			bundle.RelationKeyUniqueKey: pbtypes.String(bundle.TypeKeyDiaryEntry.URL()),
		},
		{
			bundle.RelationKeyId:        pbtypes.String("rel-custom"),
			bundle.RelationKeySpaceId:   pbtypes.String(spaceId),
			bundle.RelationKeyUniqueKey: pbtypes.String("rel-custom"),
		},
		{
			bundle.RelationKeyId:        pbtypes.String("opt-done"),
			bundle.RelationKeySpaceId:   pbtypes.String(spaceId),
			bundle.RelationKeyUniqueKey: pbtypes.String("opt-done"),
		},
	})

	u := updater{store: store}

	getSpace := func() clientspace.Space {
		spc := mock_clientspace.NewMockSpace(t)
		spc.EXPECT().Id().Return(spaceId)
		spc.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, id string, apply func(smartblock.SmartBlock) error) error {
			sb := smarttest.New(id)
			err := apply(sb)
			require.NoError(t, err)

			assert.True(t, isLastUsedDateRecent(sb.LocalDetails(), 5))
			return nil
		})
		return spc
	}

	for _, tc := range []struct {
		name            string
		key             Key
		getSpace        func() clientspace.Space
		isErrorExpected bool
	}{
		{"built-in relation", bundle.RelationKeyCamera, getSpace, false},
		{"built-in type", bundle.TypeKeyDiaryEntry, getSpace, false},
		{"custom relation", domain.RelationKey("custom"), getSpace, false},
		{"option", domain.TypeKey("opt-done"), func() clientspace.Space {
			spc := mock_clientspace.NewMockSpace(t)
			return spc
		}, true},
		{"type that is not in store", bundle.TypeKeyAudio, func() clientspace.Space {
			spc := mock_clientspace.NewMockSpace(t)
			spc.EXPECT().Id().Return(spaceId)
			return spc
		}, true},
	} {
		t.Run("update lastUsedDate of "+tc.name, func(t *testing.T) {
			err := u.updateLastUsedDate(tc.getSpace(), tc.key, ts)
			if tc.isErrorExpected {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
