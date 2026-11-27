package v2service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// tiedPageSizes are the page sizes every walk uses. Before the id
// tie-break, sizes 1 to 3 made the store switch plans between pages and
// break ties two ways (GO-7612).
var tiedPageSizes = []int{1, 2, 3, 7}

// tiedIds returns n ids in a shuffled order, so the store's insertion order
// is not the id order. 7 must not share a factor with n.
func tiedIds(prefix string, n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%s%02d", prefix, (i*7)%n)
	}
	return ids
}

// sortedIds returns a sorted copy of ids, descending when desc is set.
func sortedIds(ids []string, desc bool) []string {
	out := slices.Clone(ids)
	slices.Sort(out)
	if desc {
		slices.Reverse(out)
	}
	return out
}

// tiedObjects builds one plain object per id. They share the name and the
// type, and none has a lastModifiedDate, like objects nobody edited yet.
func tiedObjects(ids []string, extra objectstore.TestObject) []objectstore.TestObject {
	objects := make([]objectstore.TestObject, 0, len(ids))
	for _, id := range ids {
		object := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String(id),
			bundle.RelationKeyName:           domain.String("Untitled"),
			bundle.RelationKeyType:           domain.String("type-chore"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		for key, value := range extra {
			object[key] = value
		}
		objects = append(objects, object)
	}
	return objects
}

// addRelationFiller adds n relations, which the object lists filter out. A
// real space holds hundreds. With them, the store reads a small page through
// the lastModifiedDate index and a large one by a full scan, the plan switch
// that broke ties two ways.
func addRelationFiller(t *testing.T, fx *v2Fixture, n int) {
	filler := make([]objectstore.TestObject, 0, n)
	for i := 0; i < n; i++ {
		filler = append(filler, objectstore.TestObject{
			bundle.RelationKeyId:               domain.String(fmt.Sprintf("rel%03d", i)),
			bundle.RelationKeyRelationKey:      domain.String(fmt.Sprintf("filler%03d", i)),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_relation)),
			bundle.RelationKeyLastModifiedDate: domain.Int64(int64(i)),
		})
	}
	fx.objectStore.AddObjects(t, testSpaceId, filler)
}

// pageFunc reads one page and returns its row ids and has_more.
type pageFunc func(offset, limit int) ([]string, bool, error)

// walkPages reads a list page by page at one page size and returns the ids
// in the order served.
func walkPages(t *testing.T, size int, page pageFunc) []string {
	t.Helper()
	var got []string
	for offset := 0; ; offset += size {
		ids, hasMore, err := page(offset, size)
		require.NoError(t, err)
		got = append(got, ids...)
		if !hasMore {
			return got
		}
		require.Less(t, offset, 1000, "the walk does not end")
	}
}

// assertConsistentPaging asserts that one page and every page-size walk
// serve want: each row once, none skipped, in one order.
func assertConsistentPaging(t *testing.T, want []string, page pageFunc) {
	t.Helper()
	assert.Equal(t, want, walkPages(t, 1000, page), "single page")
	for _, size := range tiedPageSizes {
		assert.Equal(t, want, walkPages(t, size, page), "page size %d", size)
	}
}

func TestV2OffsetPagingTieBreak(t *testing.T) {
	ctx := context.Background()

	t.Run("list objects pages never-edited objects by id", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		addRelationFiller(t, fx, 50)
		ids := tiedIds("obj", 30)
		fx.objectStore.AddObjects(t, testSpaceId, tiedObjects(ids, nil))
		// newest-modified first, so the id runs descending too
		want := sortedIds(ids, true)

		// when, then
		assertConsistentPaging(t, want, func(offset, limit int) ([]string, bool, error) {
			rows, _, hasMore, err := fx.ListObjects(ctx, testSpaceId, nil, offset, limit)
			return rowIds(rows), hasMore, err
		})
	})

	t.Run("space search without sorts pages tied rows by id", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		addRelationFiller(t, fx, 50)
		ids := tiedIds("obj", 30)
		fx.objectStore.AddObjects(t, testSpaceId, tiedObjects(ids, nil))
		want := sortedIds(ids, true)

		// when, then
		assertConsistentPaging(t, want, func(offset, limit int) ([]string, bool, error) {
			rows, _, hasMore, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{}, offset, limit)
			return rowIds(rows), hasMore, err
		})
	})

	t.Run("space search keeps the caller's sort and breaks its ties by id", func(t *testing.T) {
		// given: every object was modified in the same second
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		addRelationFiller(t, fx, 50)
		ids := tiedIds("obj", 30)
		fx.objectStore.AddObjects(t, testSpaceId, tiedObjects(ids, objectstore.TestObject{
			bundle.RelationKeyLastModifiedDate: domain.Int64(1000),
		}))
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:               domain.String("newest"),
			bundle.RelationKeyName:             domain.String("Untitled"),
			bundle.RelationKeyType:             domain.String("type-chore"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyLastModifiedDate: domain.Int64(2000),
		}})
		req := v2model.SearchRequest{Sorts: json.RawMessage(`[{"property":"lastModifiedDate","direction":"desc"}]`)}
		want := append([]string{"newest"}, sortedIds(ids, true)...)

		// when, then
		assertConsistentPaging(t, want, func(offset, limit int) ([]string, bool, error) {
			rows, _, hasMore, _, err := fx.SearchObjects(ctx, testSpaceId, req, offset, limit)
			return rowIds(rows), hasMore, err
		})
	})

	t.Run("global search merges tied rows of two spaces by id", func(t *testing.T) {
		// given
		const otherSpaceId = "space2"
		fx := newV2Fixture(t)
		fx.registerSpace(t, otherSpaceId)
		fx.addTaskType(t)
		here, there := tiedIds("a", 15), tiedIds("b", 15)
		fx.objectStore.AddObjects(t, testSpaceId, tiedObjects(here, nil))
		fx.objectStore.AddObjects(t, otherSpaceId, tiedObjects(there, nil))
		want := sortedIds(slices.Concat(here, there), true)

		// when, then
		assertConsistentPaging(t, want, func(offset, limit int) ([]string, bool, error) {
			rows, _, hasMore, _, err := fx.GlobalSearchObjects(ctx, v2model.SearchRequest{}, offset, limit)
			return rowIds(rows), hasMore, err
		})
	})

	t.Run("query objects page tied rows by id", func(t *testing.T) {
		// given: a query over the chore type, no stored view
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		addRelationFiller(t, fx, 50)
		ids := tiedIds("obj", 30)
		fx.objectStore.AddObjects(t, testSpaceId, tiedObjects(ids, nil))
		fx.expectListRead("query1", queryRead(nil))
		want := sortedIds(ids, true)

		// when, then
		assertConsistentPaging(t, want, func(offset, limit int) ([]string, bool, error) {
			rows, _, hasMore, _, err := fx.GetQueryObjects(ctx, testSpaceId, "query1", "", nil, offset, limit)
			return rowIds(rows), hasMore, err
		})
	})

	t.Run("members with one name page by id", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		ids := tiedIds("member", 30)
		members := make([]objectstore.TestObject, 0, len(ids))
		for _, id := range ids {
			members = append(members, objectstore.TestObject{
				bundle.RelationKeyId:                domain.String(id),
				bundle.RelationKeyName:              domain.String("Member"),
				bundle.RelationKeyResolvedLayout:    domain.Int64(int64(model.ObjectType_participant)),
				bundle.RelationKeyParticipantStatus: domain.Int64(int64(model.ParticipantStatus_Active)),
			})
		}
		fx.objectStore.AddObjects(t, testSpaceId, members)
		// by name ascending, so the id runs ascending too
		want := sortedIds(ids, false)

		// when, then
		assertConsistentPaging(t, want, func(offset, limit int) ([]string, bool, error) {
			rows, _, hasMore, err := fx.ListMembers(ctx, testSpaceId, offset, limit)
			got := make([]string, len(rows))
			for i, row := range rows {
				got[i] = row.Id
			}
			return got, hasMore, err
		})
	})
}
