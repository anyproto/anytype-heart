package spaceindex

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
)

func TestFtCandidatesLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limit  int
		offset int
		want   int
	}{
		{"no limit gets one conservative round", 0, 0, ftCandidatesMin},
		{"small page is padded to the minimum", 10, 0, ftCandidatesMin},
		{"offset counts towards the multiplied budget", 50, 80, 260},
		{"big page is granted upfront with headroom", 400, 300, 1400},
		{"budget is capped by the hard limit", 900, 500, ftCandidatesHardLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ftCandidatesLimit(database.Query{Limit: tc.limit, Offset: tc.offset})
			assert.Equal(t, tc.want, got)
		})
	}
}

// Clients treat a page shorter than the requested limit as the end of the
// result set. Store-level filters run after the fulltext search, so the
// candidate budget must escalate until the page is filled or the index is
// genuinely exhausted — otherwise filtered-out candidates produce a short page
// while more matches exist.
func TestFulltextQueryFillsThePage(t *testing.T) {
	// given: 250 objects matching the query, 170 of them filtered out by the
	// default isArchived filter; only 80 results actually exist. The counts are
	// chosen so even the multiplied first-round budget (2x the page) cannot
	// fill the page and escalation must kick in.
	fx := NewStoreFixture(t)
	const (
		total    = 250
		archived = 170
	)
	objects := make([]TestObject, 0, total)
	batcher := fx.fts.NewAutoBatcher()
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("obj%d", i)
		obj := TestObject{
			bundle.RelationKeyId:   domain.String(id),
			bundle.RelationKeyName: domain.String("apple"),
		}
		if i < archived {
			obj[bundle.RelationKeyIsArchived] = domain.Bool(true)
		}
		objects = append(objects, obj)
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
			Id:      id + "/r/name",
			SpaceId: "test",
			Title:   "apple",
		}))
	}
	_, err := batcher.Finish()
	require.NoError(t, err)
	fx.AddObjects(t, objects)

	t.Run("page is filled past the first candidate round", func(t *testing.T) {
		// when: the first round (2x100 = 200 docs) can contain at most 80
		// non-archived objects, so filling a 100-record page needs escalation
		records, err := fx.Query(database.Query{
			SpaceId:   "test",
			TextQuery: "apple",
			Limit:     100,
		})

		// then: every existing result is returned; a shorter page would make
		// the client stop paginating while matches still exist
		require.NoError(t, err)
		assert.Len(t, records, total-archived)
	})

	t.Run("offset pages reach results beyond the first round", func(t *testing.T) {
		// when
		records, err := fx.Query(database.Query{
			SpaceId:   "test",
			TextQuery: "apple",
			Limit:     50,
			Offset:    60,
		})

		// then: 80 results exist, the page covers 60..79
		require.NoError(t, err)
		assert.Len(t, records, 20)
	})
}
