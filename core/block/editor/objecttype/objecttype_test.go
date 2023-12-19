package objecttype

import (
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
		ots := map[string]*types.Struct{
			bundle.TypeKeySet.BundledURL():        {Fields: make(map[string]*types.Value)},
			bundle.TypeKeyNote.BundledURL():       {Fields: make(map[string]*types.Value)},
			bundle.TypeKeyCollection.BundledURL(): {Fields: make(map[string]*types.Value)},
			bundle.TypeKeyTask.BundledURL():       {Fields: make(map[string]*types.Value)},
			bundle.TypeKeyPage.BundledURL():       {Fields: make(map[string]*types.Value)},
		}

		// when
		for id, details := range ots {
			SetLastUsedDateForCrucialType(id, details)
		}

		// then
		assert.True(t, isLastUsedDateGreater(ots[bundle.TypeKeyNote.BundledURL()], ots[bundle.TypeKeyPage.BundledURL()]))
		assert.True(t, isLastUsedDateGreater(ots[bundle.TypeKeyPage.BundledURL()], ots[bundle.TypeKeyTask.BundledURL()]))
		assert.True(t, isLastUsedDateGreater(ots[bundle.TypeKeyTask.BundledURL()], ots[bundle.TypeKeySet.BundledURL()]))
		assert.True(t, isLastUsedDateGreater(ots[bundle.TypeKeySet.BundledURL()], ots[bundle.TypeKeyCollection.BundledURL()]))
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
