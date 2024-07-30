package objecttype

import (
	"math/rand"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestSetLastUsedDateForInitialType(t *testing.T) {
	isLastUsedDateGreater := func(details1, details2 *types.Struct) bool {
		return details1.GetInt64OrDefault(bundle.RelationKeyLastUsedDate, 0) > details2.GetInt64OrDefault(bundle.RelationKeyLastUsedDate, 0)
	}

	t.Run("object types are sorted by lastUsedDate in correct order", func(t *testing.T) {
		// given
		ots := []string{
			bundle.TypeKeySet.BundledURL(),
			bundle.TypeKeyNote.BundledURL(),
			bundle.TypeKeyCollection.BundledURL(),
			bundle.TypeKeyTask.BundledURL(),
			bundle.TypeKeyPage.BundledURL(),
			bundle.TypeKeyClassNote.BundledURL(),
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
		assert.True(t, isLastUsedDateGreater(detailMap[bundle.TypeKeyCollection.BundledURL()], detailMap[bundle.TypeKeyClassNote.BundledURL()]))
	})
}
