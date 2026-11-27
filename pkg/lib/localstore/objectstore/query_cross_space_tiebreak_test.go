package objectstore

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func recordIds(records []database.Record) []string {
	out := make([]string, len(records))
	for i, record := range records {
		out[i] = record.Details.GetString(bundle.RelationKeyId)
	}
	return out
}

func TestQueryCrossSpaceIdTieBreak(t *testing.T) {
	// the sorts an API v2 search sends over RPC: relevance, then the id,
	// whose tie-break mark rides the context
	wireSorts := []database.SortRequest{
		{RelationKey: bundle.RelationKey_score, Type: model.BlockContentDataviewSort_Desc},
		{RelationKey: bundle.RelationKeyId, Type: model.BlockContentDataviewSort_Asc},
	}
	marked := database.ContextWithIdTieBreak(context.Background())

	tiedFixture := func(t *testing.T, n int) *StoreFixture {
		fx := NewStoreFixture(t)
		batcher := fx.FullText.NewAutoBatcher()
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("obj%03d", i)
			fx.AddObjects(t, "space1", []TestObject{{bundle.RelationKeyId: domain.String(id), bundle.RelationKeyName: domain.String("Untitled")}})
			require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{Id: id + "/b/body", SpaceId: "space1", Text: "needle in every body"}))
		}
		_, err := batcher.Finish()
		require.NoError(t, err)
		require.NoError(t, fx.WaitStoresLoaded(context.Background()))
		return fx
	}

	t.Run("an unpaged search keeps its tied matches", func(t *testing.T) {
		// given: 150 matches tie across the first budget
		fx := tiedFixture(t, 150)

		// when
		records, _, err := fx.QueryCrossSpaceNoWait(marked, database.Query{TextQuery: "needle", Sorts: wireSorts})

		// then: the unpaged first round, as without the tie-break
		require.NoError(t, err)
		assert.Len(t, records, 100)
	})

	t.Run("a paged search orders tied matches by id at every page size", func(t *testing.T) {
		// given
		fx := tiedFixture(t, 150)
		page := func(offset, limit int) []string {
			records, _, err := fx.QueryCrossSpaceNoWait(marked, database.Query{TextQuery: "needle", Sorts: wireSorts, Offset: offset, Limit: limit})
			require.NoError(t, err)
			return recordIds(records)
		}
		want := make([]string, 150)
		for i := range want {
			want[i] = fmt.Sprintf("obj%03d", i)
		}

		// when
		var walked []string
		for offset := 0; offset < 150; offset++ {
			walked = append(walked, page(offset, 1)...)
		}

		// then
		assert.Equal(t, want, walked)
		assert.Equal(t, want, page(0, 1000))
	})

	t.Run("a plain merge keys each space's rows with its own linked objects", func(t *testing.T) {
		// given: space1's rows link zulu, which only space2 holds, and
		// alpha, which space1 holds: space1 sorts the unresolved link first
		fx := NewStoreFixture(t)
		for _, spaceId := range []string{"space1", "space2"} {
			fx.AddObjects(t, spaceId, []TestObject{{
				bundle.RelationKeyId:             domain.String("rel-linked"),
				bundle.RelationKeyRelationKey:    domain.String("linked"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			}})
		}
		fx.AddObjects(t, "space1", []TestObject{
			{bundle.RelationKeyId: domain.String("r1"), "linked": domain.StringList([]string{"zulu"}), bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic))},
			{bundle.RelationKeyId: domain.String("r2"), "linked": domain.StringList([]string{"alpha"}), bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic))},
			{bundle.RelationKeyId: domain.String("alpha"), bundle.RelationKeyName: domain.String("Alpha"), bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption))},
		})
		fx.AddObjects(t, "space2", []TestObject{
			{bundle.RelationKeyId: domain.String("zulu"), bundle.RelationKeyName: domain.String("Zulu"), bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption))},
		})
		require.NoError(t, fx.WaitStoresLoaded(context.Background()))
		want := []string{"r1", "r2"}

		// when
		records, _, err := fx.QueryCrossSpaceNoWait(context.Background(), database.Query{
			Filters: []database.FilterRequest{{
				RelationKey: bundle.RelationKeyResolvedLayout, Condition: model.BlockContentDataviewFilter_Equal,
				Value: domain.Int64(int64(model.ObjectType_basic)),
			}},
			Sorts: []database.SortRequest{{RelationKey: "linked", Type: model.BlockContentDataviewSort_Asc}},
			Limit: 10,
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, want, recordIds(records))
	})
}
