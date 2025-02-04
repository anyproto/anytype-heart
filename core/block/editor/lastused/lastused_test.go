package lastused

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestSetLastUsedDateForInitialType(t *testing.T) {
	isLastUsedDateGreater := func(details1, details2 *domain.Details) bool {
		return details1.GetInt64(bundle.RelationKeyLastUsedDate) > details2.GetInt64(bundle.RelationKeyLastUsedDate)
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
		detailMap := map[string]*domain.Details{}

		// when
		for _, id := range ots {
			details := domain.NewDetails()
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
