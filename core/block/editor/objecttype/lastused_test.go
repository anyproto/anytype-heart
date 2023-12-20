package objecttype

import (
	"math/rand"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestSetLastUsedDateForCrucialType(t *testing.T) {
	isLastUsedDateGreater := func(details1, details2 *types.Struct) bool {
		return pbtypes.GetInt64(details1, bundle.RelationKeyLastUsedDate.String()) > pbtypes.GetInt64(details2, bundle.RelationKeyLastUsedDate.String())
	}

	t.Run("crucial object types are sorted by lastUsedDate in correct order", func(t *testing.T) {
		// given
		ots := []string{
			bundle.TypeKeySet.BundledURL(),
			bundle.TypeKeyNote.BundledURL(),
			bundle.TypeKeyCollection.BundledURL(),
			bundle.TypeKeyTask.BundledURL(),
			bundle.TypeKeyPage.BundledURL(),
		}
		rand.Shuffle(len(ots), func(i, j int) {
			ots[i], ots[j] = ots[j], ots[i]
		})
		detailMap := map[string]*types.Struct{}

		// when
		for _, id := range ots {
			details := &types.Struct{Fields: make(map[string]*types.Value)}
			SetLastUsedDateForCrucialType(id, details)
			detailMap[id] = details
		}

		// then
		assert.True(t, isLastUsedDateGreater(detailMap[bundle.TypeKeyNote.BundledURL()], detailMap[bundle.TypeKeyPage.BundledURL()]))
		assert.True(t, isLastUsedDateGreater(detailMap[bundle.TypeKeyPage.BundledURL()], detailMap[bundle.TypeKeyTask.BundledURL()]))
		assert.True(t, isLastUsedDateGreater(detailMap[bundle.TypeKeyTask.BundledURL()], detailMap[bundle.TypeKeySet.BundledURL()]))
		assert.True(t, isLastUsedDateGreater(detailMap[bundle.TypeKeySet.BundledURL()], detailMap[bundle.TypeKeyCollection.BundledURL()]))
	})

	t.Run("lastUsedDate is not set to non-crucial types", func(t *testing.T) {
		// given
		details := &types.Struct{Fields: make(map[string]*types.Value)}

		// when
		SetLastUsedDateForCrucialType(bundle.TypeKeyClassNote.BundledURL(), details)

		// then
		assert.Zero(t, pbtypes.GetInt64(details, bundle.RelationKeyLastUsedDate.String()))
	})
}
