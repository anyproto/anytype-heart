package v2service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// tiedPageSizes are the page sizes every walk uses. Before the id
// tie-break, sizes 1 to 3 made the store switch plans between pages and
// break ties two ways (GO-7612).
var tiedPageSizes = []int{1, 2, 3, 7}

var (
	tieBreakAsc  = database.IdTieBreak(model.BlockContentDataviewSort_Asc)
	tieBreakDesc = database.IdTieBreak(model.BlockContentDataviewSort_Desc)
	// the tie-break as a cross-space request carries it: a plain id sort,
	// whose mark rides the context (database.ContextWithIdTieBreak)
	wireIdAsc  = database.SortRequest{RelationKey: bundle.RelationKeyId, Type: model.BlockContentDataviewSort_Asc}
	wireIdDesc = database.SortRequest{RelationKey: bundle.RelationKeyId, Type: model.BlockContentDataviewSort_Desc}
	// lastModifiedDesc is the default sort of the object lists
	lastModifiedDesc = database.SortRequest{
		RelationKey: bundle.RelationKeyLastModifiedDate,
		Type:        model.BlockContentDataviewSort_Desc,
		IncludeTime: true,
	}
)

// tiedIds returns n ids in a shuffled order, so the store's insertion order
// is not the id order. 7 must not share a factor with n.
func tiedIds(prefix string, n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%s%03d", prefix, (i*7)%n)
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
// real space holds hundreds. With them, the store serves a small page in the
// order of the lastModifiedDate index and sorts a large one in memory: the
// plan switch that broke ties two ways.
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

// indexBodies indexes one identical body doc per object, in the given
// order, so every object scores the same.
func indexBodies(t *testing.T, fx *v2Fixture, spaceId string, ids []string) {
	batcher := fx.objectStore.FullText.NewAutoBatcher()
	for _, id := range ids {
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{Id: id + "/b/body", SpaceId: spaceId, Text: "sharedword in every body"}))
	}
	_, err := batcher.Finish()
	require.NoError(t, err)
}

// queryLog records the queries the service sends to space indexes.
type queryLog struct {
	mu      sync.Mutex
	queries []database.Query
}

func (l *queryLog) add(q database.Query) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.queries = append(l.queries, q)
}

// sortsLed returns the sorts of every recorded query whose first sort is on key.
func (l *queryLog) sortsLed(key domain.RelationKey) [][]database.SortRequest {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out [][]database.SortRequest
	for _, q := range l.queries {
		if len(q.Sorts) > 0 && q.Sorts[0].RelationKey == key {
			out = append(out, q.Sorts)
		}
	}
	return out
}

type recordingStore struct {
	objectstore.ObjectStore
	log *queryLog
}

func (r recordingStore) SpaceIndex(spaceId string) spaceindex.Store {
	return recordingIndex{Store: r.ObjectStore.SpaceIndex(spaceId), log: r.log}
}

type recordingIndex struct {
	spaceindex.Store
	log *queryLog
}

func (r recordingIndex) Query(q database.Query) ([]database.Record, error) {
	r.log.add(q)
	return r.Store.Query(q)
}

func (r recordingIndex) QueryAndCount(q database.Query) ([]database.Record, int, error) {
	r.log.add(q)
	return r.Store.QueryAndCount(q)
}

// recordQueries makes the service's space index queries observable.
func (fx *v2Fixture) recordQueries() *queryLog {
	log := &queryLog{}
	fx.Service.store = recordingStore{ObjectStore: fx.Service.store, log: log}
	return log
}

// crossSpaceSorts returns the sorts of every cross-space search sent.
func (fx *v2Fixture) crossSpaceSorts() [][]database.SortRequest {
	var out [][]database.SortRequest
	for _, call := range fx.mwMock.Calls {
		if call.Method != "ObjectCrossSpaceSearch" {
			continue
		}
		if req, ok := call.Arguments.Get(1).(*pb.RpcObjectCrossSpaceSearchRequest); ok {
			sorts := database.SortsFromProto(req.Sorts)
			for i := range sorts {
				if len(sorts[i].CustomOrder) == 0 {
					sorts[i].CustomOrder = nil // the proto has no nil list
				}
			}
			out = append(out, sorts)
		}
	}
	return out
}

// assertEverySorts asserts that queries were sent, all with want.
func assertEverySorts(t *testing.T, want []database.SortRequest, got [][]database.SortRequest) {
	t.Helper()
	require.NotEmpty(t, got, "no query was sent")
	for _, sorts := range got {
		assert.Equal(t, want, sorts)
	}
}

// pageFunc reads one page and returns its row keys and has_more.
type pageFunc func(offset, limit int) ([]string, bool, error)

// walkPages reads a list of n rows page by page at one page size and
// returns the keys in the order served. Each page must be full but the
// last, and has_more must say whether another follows.
func walkPages(t *testing.T, n, size int, page pageFunc) []string {
	t.Helper()
	var got []string
	for offset := 0; ; offset += size {
		keys, hasMore, err := page(offset, size)
		require.NoError(t, err)
		assert.Len(t, keys, max(0, min(size, n-offset)), "page size %d at offset %d", size, offset)
		assert.Equal(t, offset+size < n, hasMore, "has_more of page size %d at offset %d", size, offset)
		got = append(got, keys...)
		if !hasMore || offset > 2*n {
			return got
		}
	}
}

// assertConsistentPaging asserts that one page and every page-size walk
// serve want: each row once, none skipped, in one order.
func assertConsistentPaging(t *testing.T, want []string, sizes []int, page pageFunc) {
	t.Helper()
	assert.Equal(t, want, walkPages(t, len(want), 1000, page), "single page")
	for _, size := range sizes {
		assert.Equal(t, want, walkPages(t, len(want), size, page), "page size %d", size)
	}
}

// objectPages adapts a list of object rows to a pageFunc.
func objectPages(list func(offset, limit int) ([]v2model.ObjectRow, bool, error)) pageFunc {
	return func(offset, limit int) ([]string, bool, error) {
		rows, hasMore, err := list(offset, limit)
		return rowIds(rows), hasMore, err
	}
}

// keysOf maps rows to the key a walk compares.
func keysOf[T any](rows []T, key func(T) string) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = key(row)
	}
	return out
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
		queries := fx.recordQueries()
		// newest-modified first, so the id runs descending too
		want := sortedIds(ids, true)

		// when, then
		assertConsistentPaging(t, want, tiedPageSizes, objectPages(func(offset, limit int) ([]v2model.ObjectRow, bool, error) {
			rows, _, hasMore, err := fx.ListObjects(ctx, testSpaceId, nil, offset, limit)
			return rows, hasMore, err
		}))
		assertEverySorts(t, []database.SortRequest{lastModifiedDesc, tieBreakDesc}, queries.sortsLed(bundle.RelationKeyLastModifiedDate))
	})

	t.Run("space search without sorts pages tied rows by id", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		addRelationFiller(t, fx, 50)
		ids := tiedIds("obj", 30)
		fx.objectStore.AddObjects(t, testSpaceId, tiedObjects(ids, nil))
		queries := fx.recordQueries()
		want := sortedIds(ids, true)

		// when, then
		assertConsistentPaging(t, want, tiedPageSizes, objectPages(func(offset, limit int) ([]v2model.ObjectRow, bool, error) {
			rows, _, hasMore, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{}, offset, limit)
			return rows, hasMore, err
		}))
		assertEverySorts(t, []database.SortRequest{lastModifiedDesc, tieBreakDesc}, queries.sortsLed(bundle.RelationKeyLastModifiedDate))
	})

	t.Run("space search keeps the caller's sorts and breaks their ties by id", func(t *testing.T) {
		// given: two names, all modified in the same second; name descending,
		// then the date ascending, which ties
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		addRelationFiller(t, fx, 50)
		// a space holds the name property, which the sort channel resolves
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("rel-name"),
			bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyName.String()),
			bundle.RelationKeyName:           domain.String("Name"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		}})
		alphas, betas := tiedIds("alpha", 10), tiedIds("beta", 10)
		fx.objectStore.AddObjects(t, testSpaceId, tiedObjects(alphas, objectstore.TestObject{
			bundle.RelationKeyName: domain.String("Alpha"), bundle.RelationKeyLastModifiedDate: domain.Int64(1000),
		}))
		fx.objectStore.AddObjects(t, testSpaceId, tiedObjects(betas, objectstore.TestObject{
			bundle.RelationKeyName: domain.String("Beta"), bundle.RelationKeyLastModifiedDate: domain.Int64(1000),
		}))
		queries := fx.recordQueries()
		req := v2model.SearchRequest{Sorts: json.RawMessage(`[{"property":"name","direction":"desc"},{"property":"lastModifiedDate","direction":"asc"}]`)}
		want := append(sortedIds(betas, false), sortedIds(alphas, false)...)

		// when, then
		assertConsistentPaging(t, want, tiedPageSizes, objectPages(func(offset, limit int) ([]v2model.ObjectRow, bool, error) {
			rows, _, hasMore, _, err := fx.SearchObjects(ctx, testSpaceId, req, offset, limit)
			return rows, hasMore, err
		}))
		sorts := queries.sortsLed(bundle.RelationKeyName)
		require.NotEmpty(t, sorts)
		for _, got := range sorts {
			assert.Equal(t, tieBreakAsc, got[len(got)-1], "the id follows the last sort, ascending")
		}
	})

	t.Run("an explicit day precision groups one day's objects by id", func(t *testing.T) {
		// given: z changed at 09:00, a at 17:00 the same day
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		day := time.Date(2026, 11, 23, 0, 0, 0, 0, time.Local)
		fx.objectStore.AddObjects(t, testSpaceId, tiedObjects([]string{"z"}, objectstore.TestObject{
			bundle.RelationKeyLastModifiedDate: domain.Int64(day.Add(9 * time.Hour).Unix()),
		}))
		fx.objectStore.AddObjects(t, testSpaceId, tiedObjects([]string{"a"}, objectstore.TestObject{
			bundle.RelationKeyLastModifiedDate: domain.Int64(day.Add(17 * time.Hour).Unix()),
		}))
		req := v2model.SearchRequest{Sorts: json.RawMessage(`[{"property":"lastModifiedDate","direction":"asc","include_time":false}]`)}
		want := []string{"a", "z"}

		// when
		rows, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, req, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rowIds(rows))
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
		assertConsistentPaging(t, want, tiedPageSizes, objectPages(func(offset, limit int) ([]v2model.ObjectRow, bool, error) {
			rows, _, hasMore, _, err := fx.GlobalSearchObjects(ctx, v2model.SearchRequest{}, offset, limit)
			return rows, hasMore, err
		}))
		assertEverySorts(t, []database.SortRequest{lastModifiedDesc, wireIdDesc}, fx.crossSpaceSorts())
	})

	t.Run("global search merges two query groups in the store's name order", func(t *testing.T) {
		// given: the chore type has a different id in each space, so each
		// space runs its own query. The store ignores case, so a, a and A tie
		fx := newV2Fixture(t)
		const otherSpaceId = "space2"
		fx.registerSpace(t, otherSpaceId)
		fx.addTaskType(t)
		fx.addType(t, otherSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:        domain.String("other-chore-type"),
			bundle.RelationKeyName:      domain.String("Chore"),
			bundle.RelationKeyUniqueKey: domain.String("ot-chore"),
		})
		named := func(id, name, typeId string) objectstore.TestObject {
			return objectstore.TestObject{
				bundle.RelationKeyId:             domain.String(id),
				bundle.RelationKeyName:           domain.String(name),
				bundle.RelationKeyType:           domain.String(typeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			}
		}
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			named("c03", "A", "type-chore"), named("c01", "a", "type-chore"), named("c02", "a", "type-chore"),
			named("c04", "b", "type-chore"), named("c05", "Z", "type-chore"),
		})
		fx.objectStore.AddObjects(t, otherSpaceId, []objectstore.TestObject{named("d01", "zzzzz", "other-chore-type")})
		for _, spaceId := range []string{testSpaceId, otherSpaceId} {
			fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:             domain.String("rel-name"),
				bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyName.String()),
				bundle.RelationKeyName:           domain.String("Name"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			}})
		}
		req := v2model.SearchRequest{Type: "chore", Sorts: json.RawMessage(`[{"property":"name","direction":"asc"}]`)}
		want := []string{"c01", "c02", "c03", "c04", "c05", "d01"}

		// when, then
		assertConsistentPaging(t, want, tiedPageSizes, objectPages(func(offset, limit int) ([]v2model.ObjectRow, bool, error) {
			rows, _, hasMore, _, err := fx.GlobalSearchObjects(ctx, req, offset, limit)
			return rows, hasMore, err
		}))
	})

	t.Run("global search keeps the store's place for an undated row", func(t *testing.T) {
		// given: a day-only date sort; the store puts the undated row last
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		day := time.Date(2026, 11, 23, 9, 0, 0, 0, time.Local)
		fx.objectStore.AddObjects(t, testSpaceId, slices.Concat(
			tiedObjects([]string{"dated1"}, objectstore.TestObject{bundle.RelationKeyLastModifiedDate: domain.Int64(day.Unix())}),
			tiedObjects([]string{"dated2"}, objectstore.TestObject{bundle.RelationKeyLastModifiedDate: domain.Int64(day.AddDate(0, 0, 1).Unix())}),
			tiedObjects([]string{"undated"}, nil),
		))
		req := v2model.SearchRequest{Sorts: json.RawMessage(`[{"property":"lastModifiedDate","direction":"asc","include_time":false}]`)}
		want := []string{"dated1", "dated2", "undated"}

		// when, then
		assertConsistentPaging(t, want, []int{1, 2}, objectPages(func(offset, limit int) ([]v2model.ObjectRow, bool, error) {
			rows, _, hasMore, _, err := fx.GlobalSearchObjects(ctx, req, offset, limit)
			return rows, hasMore, err
		}))
	})

	t.Run("global search keys each space's rows with its own linked objects", func(t *testing.T) {
		// given: rows in space1 link Zulu, which only space2 holds, and
		// Alpha, which space1 holds; space1 sorts its unresolved link first
		fx := newV2Fixture(t)
		const otherSpaceId = "space2"
		fx.registerSpace(t, otherSpaceId)
		fx.addTaskType(t)
		for _, spaceId := range []string{testSpaceId, otherSpaceId} {
			fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:             domain.String("rel-linked"),
				bundle.RelationKeyRelationKey:    domain.String("linked"),
				bundle.RelationKeyName:           domain.String("Linked"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			}})
		}
		hiddenNamed := func(id, name string) objectstore.TestObject {
			return objectstore.TestObject{
				bundle.RelationKeyId:             domain.String(id),
				bundle.RelationKeyName:           domain.String(name),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyIsHidden:       domain.Bool(true),
			}
		}
		fx.objectStore.AddObjects(t, testSpaceId, slices.Concat(
			tiedObjects([]string{"r1", "r2"}, objectstore.TestObject{"linked": domain.StringList([]string{"zulu"})}),
			tiedObjects([]string{"r3"}, objectstore.TestObject{"linked": domain.StringList([]string{"alpha"})}),
			[]objectstore.TestObject{hiddenNamed("alpha", "Alpha")},
		))
		fx.objectStore.AddObjects(t, otherSpaceId, []objectstore.TestObject{hiddenNamed("zulu", "Zulu")})
		req := v2model.SearchRequest{Sorts: json.RawMessage(`[{"property":"linked","direction":"asc"}]`)}
		want := []string{"r1", "r2", "r3"}

		// when, then
		assertConsistentPaging(t, want, []int{1, 2}, objectPages(func(offset, limit int) ([]v2model.ObjectRow, bool, error) {
			rows, _, hasMore, _, err := fx.GlobalSearchObjects(ctx, req, offset, limit)
			return rows, hasMore, err
		}))
	})

	t.Run("global search keys each space's rows with its own options", func(t *testing.T) {
		// given: space2 holds an option with space1's id under another name
		fx := newV2Fixture(t)
		const otherSpaceId = "space2"
		fx.registerSpace(t, otherSpaceId)
		fx.addTaskType(t)
		option := func(id, name string) objectstore.TestObject {
			return objectstore.TestObject{
				bundle.RelationKeyId:             domain.String(id),
				bundle.RelationKeyRelationKey:    domain.String("label"),
				bundle.RelationKeyName:           domain.String(name),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			}
		}
		for _, spaceId := range []string{testSpaceId, otherSpaceId} {
			fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:             domain.String("rel-label"),
				bundle.RelationKeyRelationKey:    domain.String("label"),
				bundle.RelationKeyName:           domain.String("Label"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			}})
		}
		fx.objectStore.AddObjects(t, testSpaceId, slices.Concat(
			tiedObjects([]string{"x"}, objectstore.TestObject{"label": domain.StringList([]string{"opt1"})}),
			tiedObjects([]string{"y"}, objectstore.TestObject{"label": domain.StringList([]string{"opt2"})}),
			[]objectstore.TestObject{option("opt1", "b"), option("opt2", "c")},
		))
		fx.objectStore.AddObjects(t, otherSpaceId, []objectstore.TestObject{option("opt1", "z")})
		req := v2model.SearchRequest{Sorts: json.RawMessage(`[{"property":"label","direction":"asc"}]`)}
		want := []string{"x", "y"}

		// when, then
		assertConsistentPaging(t, want, []int{1}, objectPages(func(offset, limit int) ([]v2model.ObjectRow, bool, error) {
			rows, _, hasMore, _, err := fx.GlobalSearchObjects(ctx, req, offset, limit)
			return rows, hasMore, err
		}))
	})

	// spaceIdRows adds the spaceId relation to both spaces and, in each,
	// rows without a stored spaceId (written before it was stored), with
	// ids before the rows that have it.
	spaceIdRows := func(t *testing.T, fx *v2Fixture, otherSpaceId, otherTypeId string) {
		for _, spaceId := range []string{testSpaceId, otherSpaceId} {
			fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:             domain.String("rel-spaceId"),
				bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeySpaceId.String()),
				bundle.RelationKeyName:           domain.String("Space"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			}})
		}
		fx.objectStore.AddObjects(t, testSpaceId, slices.Concat(
			tiedObjects([]string{"b1", "b2"}, nil),
			tiedObjects([]string{"y1", "z1"}, objectstore.TestObject{bundle.RelationKeySpaceId: domain.String(testSpaceId)}),
		))
		fx.objectStore.AddObjects(t, otherSpaceId, slices.Concat(
			tiedObjects([]string{"b3"}, objectstore.TestObject{bundle.RelationKeyType: domain.String(otherTypeId)}),
			tiedObjects([]string{"z2"}, objectstore.TestObject{bundle.RelationKeyType: domain.String(otherTypeId), bundle.RelationKeySpaceId: domain.String(otherSpaceId)}),
		))
	}
	// a manual order of the spaces: a row without a stored spaceId is unlisted
	spaceIdSort := func(otherSpaceId string) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`[{"property":"spaceId","direction":"custom","custom_order":[%q,%q]}]`, testSpaceId, otherSpaceId))
	}

	t.Run("global search pages rows that lack their stored spaceId", func(t *testing.T) {
		// given: one query group over two spaces
		fx := newV2Fixture(t)
		const otherSpaceId = "space2"
		fx.registerSpace(t, otherSpaceId)
		fx.addTaskType(t)
		spaceIdRows(t, fx, otherSpaceId, "type-chore")
		req := v2model.SearchRequest{Sorts: spaceIdSort(otherSpaceId)}
		// the listed rows, then the unlisted ones as their stores cut them
		want := []string{"y1", "z1", "z2", "b1", "b2", "b3"}

		// when, then
		assertConsistentPaging(t, want, []int{1, 2}, objectPages(func(offset, limit int) ([]v2model.ObjectRow, bool, error) {
			rows, _, hasMore, _, err := fx.GlobalSearchObjects(ctx, req, offset, limit)
			return rows, hasMore, err
		}))
	})

	t.Run("global search pages rows that lack their stored spaceId across query groups", func(t *testing.T) {
		// given: the chore type has another id in space2, so each space
		// runs its own query
		fx := newV2Fixture(t)
		const otherSpaceId = "space2"
		fx.registerSpace(t, otherSpaceId)
		for spaceId, typeId := range map[string]string{testSpaceId: "type-chore", otherSpaceId: "other-chore-type"} {
			fx.addType(t, spaceId, objectstore.TestObject{
				bundle.RelationKeyId:                   domain.String(typeId),
				bundle.RelationKeyName:                 domain.String("Chore"),
				bundle.RelationKeyUniqueKey:            domain.String("ot-chore"),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-spaceId"}),
			})
		}
		spaceIdRows(t, fx, otherSpaceId, "other-chore-type")
		req := v2model.SearchRequest{Type: "chore", Sorts: spaceIdSort(otherSpaceId)}
		// each space's cut keeps its order; the cuts merge on the spaceId
		// served
		want := []string{"y1", "z1", "b1", "b2", "z2", "b3"}

		// when, then
		assertConsistentPaging(t, want, []int{1, 2}, objectPages(func(offset, limit int) ([]v2model.ObjectRow, bool, error) {
			rows, _, hasMore, _, err := fx.GlobalSearchObjects(ctx, req, offset, limit)
			return rows, hasMore, err
		}))
	})

	t.Run("global full-text search orders the best 100 of two groups by the requested sort", func(t *testing.T) {
		// given: 500 tied matches over two query groups (the chore type has
		// another id in space2), indexed in reverse id order, so a small cut
		// keeps an arbitrary 200 of them; names run against relevance order
		fx := newV2Fixture(t)
		const otherSpaceId = "space2"
		fx.registerSpace(t, otherSpaceId)
		fx.addTaskType(t)
		fx.addType(t, otherSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:        domain.String("other-chore-type"),
			bundle.RelationKeyName:      domain.String("Chore"),
			bundle.RelationKeyUniqueKey: domain.String("ot-chore"),
		})
		const perSpace = 250
		var here, there []string
		for i := 0; i < perSpace; i++ {
			here = append(here, fmt.Sprintf("a%03d", i))
			there = append(there, fmt.Sprintf("b%03d", i))
		}
		groups := []struct {
			spaceId, typeId string
			ids             []string
		}{{testSpaceId, "type-chore", here}, {otherSpaceId, "other-chore-type", there}}
		for spaceIndex, group := range groups {
			objects := make([]objectstore.TestObject, 0, len(group.ids))
			for i, id := range group.ids {
				objects = append(objects, objectstore.TestObject{
					bundle.RelationKeyId:             domain.String(id),
					bundle.RelationKeyName:           domain.String(fmt.Sprintf("n%03d", 2*perSpace-1-(spaceIndex*perSpace+i))),
					bundle.RelationKeyType:           domain.String(group.typeId),
					bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
				})
			}
			fx.objectStore.AddObjects(t, group.spaceId, objects)
		}
		for _, group := range slices.Backward(groups) {
			indexBodies(t, fx, group.spaceId, sortedIds(group.ids, true))
		}
		req := v2model.SearchRequest{Query: "sharedword", Type: "chore", Sorts: json.RawMessage(`[{"property":"name","direction":"asc"}]`)}
		// the best 100 by relevance (ties by id) in name order, then the rest
		// in relevance order
		want := slices.Concat(sortedIds(here[:100], true), here[100:], there)

		// when, then
		assertConsistentPaging(t, want, []int{60, 7}, objectPages(func(offset, limit int) ([]v2model.ObjectRow, bool, error) {
			rows, _, hasMore, _, err := fx.GlobalSearchObjects(ctx, req, offset, limit)
			return rows, hasMore, err
		}))
	})

	for _, tc := range []struct {
		name, key string
		format    model.RelationFormat
		sort      string
		rows      map[string]domain.Value
		want      []string
	}{
		{
			name: "a zero under empty placement", key: "rank", format: model.RelationFormat_number,
			sort: `[{"property":"rank","direction":"asc","empty_placement":"end"}]`,
			rows: map[string]domain.Value{"zero": domain.Int64(0), "one": domain.Int64(1)},
			want: []string{"zero", "one"},
		},
		{
			name: "URLs of two cases", key: "site", format: model.RelationFormat_url,
			sort: `[{"property":"site","direction":"asc"}]`,
			rows: map[string]domain.Value{"upper": domain.String("https://x.test/Z"), "lower": domain.String("https://x.test/a")},
			want: []string{"lower", "upper"},
		},
		{
			name: "emails of two cases", key: "mail", format: model.RelationFormat_email,
			sort: `[{"property":"mail","direction":"asc"}]`,
			rows: map[string]domain.Value{"zoo": domain.String("Zoo@example.com"), "alpha": domain.String("alpha@example.com")},
			want: []string{"alpha", "zoo"},
		},
	} {
		t.Run("space and global full-text heads agree on "+tc.name, func(t *testing.T) {
			// given: one space, so both heads order the same two matches
			fx := newV2Fixture(t)
			fx.addTaskType(t)
			fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:             domain.String("rel-" + tc.key),
				bundle.RelationKeyRelationKey:    domain.String(tc.key),
				bundle.RelationKeyName:           domain.String(tc.key),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(tc.format)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			}})
			ids := make([]string, 0, len(tc.rows))
			for id, value := range tc.rows {
				fx.objectStore.AddObjects(t, testSpaceId, tiedObjects([]string{id}, objectstore.TestObject{domain.RelationKey(tc.key): value}))
				ids = append(ids, id)
			}
			indexBodies(t, fx, testSpaceId, ids)
			req := v2model.SearchRequest{Query: "sharedword", Sorts: json.RawMessage(tc.sort)}

			// when
			spaceRows, _, _, _, spaceErr := fx.SearchObjects(ctx, testSpaceId, req, 0, 25)
			globalRows, _, _, _, globalErr := fx.GlobalSearchObjects(ctx, req, 0, 25)

			// then
			require.NoError(t, spaceErr)
			require.NoError(t, globalErr)
			assert.Equal(t, tc.want, rowIds(spaceRows), "space search")
			assert.Equal(t, tc.want, rowIds(globalRows), "global search")
		})
	}

	t.Run("global full-text search pages 160 tied matches by id", func(t *testing.T) {
		// given: one space, 160 objects with the same body, so every score ties
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		ids := tiedIds("obj", 160)
		fx.objectStore.AddObjects(t, testSpaceId, tiedObjects(ids, nil))
		indexBodies(t, fx, testSpaceId, ids)
		req := v2model.SearchRequest{Query: "sharedword"}
		want := sortedIds(ids, false)

		// when, then
		assertConsistentPaging(t, want, []int{80, 7}, objectPages(func(offset, limit int) ([]v2model.ObjectRow, bool, error) {
			rows, _, hasMore, _, err := fx.GlobalSearchObjects(ctx, req, offset, limit)
			return rows, hasMore, err
		}))
		assertEverySorts(t, []database.SortRequest{{RelationKey: bundle.RelationKey_score, Type: model.BlockContentDataviewSort_Desc}, wireIdAsc}, fx.crossSpaceSorts())
	})

	t.Run("space full-text search settles a score tie at the candidate boundary", func(t *testing.T) {
		// given: 150 tied matches. Asked for 100 docs, the index keeps 100
		// of them by its own tie order (here it leaves obj001 out), so a
		// small page's candidates are not the first 100 ids
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		ids := tiedIds("obj", 150)
		fx.objectStore.AddObjects(t, testSpaceId, tiedObjects(ids, nil))
		indexBodies(t, fx, testSpaceId, sortedIds(ids, false))
		req := v2model.SearchRequest{Query: "sharedword"}
		want := sortedIds(ids, false)

		// when, then
		assertConsistentPaging(t, want, tiedPageSizes, objectPages(func(offset, limit int) ([]v2model.ObjectRow, bool, error) {
			rows, _, hasMore, _, err := fx.SearchObjects(ctx, testSpaceId, req, offset, limit)
			return rows, hasMore, err
		}))
	})

	t.Run("query objects page tied rows by id", func(t *testing.T) {
		// given: a query over the chore type, no stored view
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		addRelationFiller(t, fx, 50)
		ids := tiedIds("obj", 30)
		fx.objectStore.AddObjects(t, testSpaceId, tiedObjects(ids, nil))
		fx.expectListRead("query1", queryRead(nil))
		queries := fx.recordQueries()
		want := sortedIds(ids, true)

		// when, then
		assertConsistentPaging(t, want, tiedPageSizes, objectPages(func(offset, limit int) ([]v2model.ObjectRow, bool, error) {
			rows, _, hasMore, _, err := fx.GetQueryObjects(ctx, testSpaceId, "query1", "", nil, offset, limit)
			return rows, hasMore, err
		}))
		assertEverySorts(t, []database.SortRequest{lastModifiedDesc, tieBreakDesc}, queries.sortsLed(bundle.RelationKeyLastModifiedDate))
	})

	t.Run("a stored view's sorts keep their order and tie by id", func(t *testing.T) {
		// given: the view sorts by name descending, then by created date,
		// which ties
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		alphas, betas := tiedIds("alpha", 6), tiedIds("beta", 6)
		fx.objectStore.AddObjects(t, testSpaceId, tiedObjects(alphas, objectstore.TestObject{
			bundle.RelationKeyName: domain.String("Alpha"), bundle.RelationKeyCreatedDate: domain.Int64(1000),
		}))
		fx.objectStore.AddObjects(t, testSpaceId, tiedObjects(betas, objectstore.TestObject{
			bundle.RelationKeyName: domain.String("Beta"), bundle.RelationKeyCreatedDate: domain.Int64(1000),
		}))
		fx.expectListRead("query1", queryRead(&model.BlockContentDataview{Views: []*model.BlockContentDataviewView{{
			Id: "view1abc",
			Sorts: []*model.BlockContentDataviewSort{
				{RelationKey: bundle.RelationKeyName.String(), Type: model.BlockContentDataviewSort_Desc},
				{RelationKey: bundle.RelationKeyCreatedDate.String(), Type: model.BlockContentDataviewSort_Asc},
			},
		}}}))
		want := append(sortedIds(betas, false), sortedIds(alphas, false)...)

		// when, then
		assertConsistentPaging(t, want, tiedPageSizes, objectPages(func(offset, limit int) ([]v2model.ObjectRow, bool, error) {
			rows, _, hasMore, _, err := fx.GetQueryObjects(ctx, testSpaceId, "query1", "", nil, offset, limit)
			return rows, hasMore, err
		}))
	})

	t.Run("a new query's default sort keeps the seconds the app shows", func(t *testing.T) {
		// given: z changed at 09:00, a at 17:00 the same day; the view holds
		// the sort a new query stores, with no format and no precision
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		day := time.Date(2026, 11, 23, 0, 0, 0, 0, time.Local)
		fx.objectStore.AddObjects(t, testSpaceId, slices.Concat(
			tiedObjects([]string{"z"}, objectstore.TestObject{bundle.RelationKeyLastModifiedDate: domain.Int64(day.Add(9 * time.Hour).Unix())}),
			tiedObjects([]string{"a"}, objectstore.TestObject{bundle.RelationKeyLastModifiedDate: domain.Int64(day.Add(17 * time.Hour).Unix())}),
		))
		fx.expectListRead("query1", queryRead(&model.BlockContentDataview{Views: []*model.BlockContentDataviewView{{
			Id:    "view1abc",
			Sorts: template.DefaultLastModifiedDateSort(),
		}}}))
		// newest first, to the second
		want := []string{"a", "z"}

		// when
		rows, _, _, _, err := fx.GetQueryObjects(ctx, testSpaceId, "query1", "", nil, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rowIds(rows))
	})

	t.Run("a stored view sorted by id lists objects by id", func(t *testing.T) {
		// given: names run against ids; the store reads an id as no
		// reference, so the appended raw id decides
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		fx.objectStore.AddObjects(t, testSpaceId, slices.Concat(
			tiedObjects([]string{"a"}, objectstore.TestObject{bundle.RelationKeyName: domain.String("Zulu")}),
			tiedObjects([]string{"b"}, objectstore.TestObject{bundle.RelationKeyName: domain.String("Alpha")}),
		))
		fx.expectListRead("query1", queryRead(&model.BlockContentDataview{Views: []*model.BlockContentDataviewView{{
			Id:    "view1abc",
			Sorts: []*model.BlockContentDataviewSort{{RelationKey: bundle.RelationKeyId.String(), Type: model.BlockContentDataviewSort_Asc}},
		}}}))
		want := []string{"a", "b"}

		// when
		rows, _, _, _, err := fx.GetQueryObjects(ctx, testSpaceId, "query1", "", nil, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rowIds(rows))
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
		queries := fx.recordQueries()
		// by name ascending, so the id runs ascending too
		want := sortedIds(ids, false)

		// when, then
		assertConsistentPaging(t, want, tiedPageSizes, func(offset, limit int) ([]string, bool, error) {
			rows, _, hasMore, err := fx.ListMembers(ctx, testSpaceId, offset, limit)
			return keysOf(rows, func(row v2model.MemberRow) string { return row.Id }), hasMore, err
		})
		assertEverySorts(t, []database.SortRequest{{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc}, tieBreakAsc},
			queries.sortsLed(bundle.RelationKeyName))
	})

	t.Run("chats modified in one second page by id", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		ids := tiedIds("chat", 5)
		states := map[string][2]int32{}
		for _, id := range ids {
			fx.addChat(t, id, "Chat", 1000)
			states[id] = [2]int32{0, 0}
		}
		fx.withChatStates(t, states)
		queries := fx.recordQueries()
		want := sortedIds(ids, true)

		// when, then
		assertConsistentPaging(t, want, []int{1, 2}, func(offset, limit int) ([]string, bool, error) {
			rows, _, hasMore, err := fx.ListChats(ctx, testSpaceId, ChatListQuery{}, offset, limit)
			return keysOf(rows, func(row v2model.ChatRow) string { return row.Id }), hasMore, err
		})
		assertEverySorts(t, []database.SortRequest{lastModifiedDesc, tieBreakDesc}, queries.sortsLed(bundle.RelationKeyLastModifiedDate))
	})

	t.Run("templates modified in one second page by id", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		ids := tiedIds("tpl", 5)
		for _, id := range ids {
			fx.addTemplate(t, id, "Memo template", testMemoTypeId, objectstore.TestObject{
				bundle.RelationKeyLastModifiedDate: domain.Int64(1000),
			})
		}
		queries := fx.recordQueries()
		want := sortedIds(ids, true)

		// when, then
		assertConsistentPaging(t, want, []int{1, 2}, func(offset, limit int) ([]string, bool, error) {
			rows, _, hasMore, err := fx.ListTemplates(ctx, testSpaceId, "", offset, limit)
			return keysOf(rows, func(row v2model.TemplateRow) string { return row.Id }), hasMore, err
		})
		assertEverySorts(t, []database.SortRequest{lastModifiedDesc, tieBreakDesc}, queries.sortsLed(bundle.RelationKeyLastModifiedDate))
	})

	t.Run("out-of-sync objects changed in one second page by id", func(t *testing.T) {
		// given
		fx, _ := newSyncFixture(t)
		ids := tiedIds("obj", 5)
		for _, id := range ids {
			fx.addPendingObject(t, id, domain.ObjectSyncStatusSyncing, domain.SyncErrorNull, 1000)
		}
		queries := fx.recordQueries()
		want := sortedIds(ids, true)

		// when, then
		assertConsistentPaging(t, want, []int{1, 2}, func(offset, limit int) ([]string, bool, error) {
			objects, hasMore, err := fx.outOfSyncObjects(testSpaceId, offset, limit)
			return keysOf(objects, func(object v2model.PendingObject) string { return object.Id }), hasMore, err
		})
		assertEverySorts(t, []database.SortRequest{lastModifiedDesc, tieBreakDesc}, queries.sortsLed(bundle.RelationKeyLastModifiedDate))
	})

	t.Run("types with one name page by id and count only servable rows", func(t *testing.T) {
		// given: three types named Same, and one more whose unique key is
		// not a type key, so it serves no row
		fx := newV2Fixture(t)
		for _, key := range []string{"tie2", "tie0", "tie1"} {
			fx.addType(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:           domain.String("type-" + key),
				bundle.RelationKeyName:         domain.String("Same"),
				bundle.RelationKeyUniqueKey:    domain.String("ot-" + key),
				bundle.RelationKeyApiObjectKey: domain.String(key),
			})
		}
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:        domain.String("type-tie05"),
			bundle.RelationKeyName:      domain.String("Same"),
			bundle.RelationKeyUniqueKey: domain.String("not-a-type-key"),
		})
		want := []string{"tie0", "tie1", "tie2"}

		// when, then
		assertConsistentPaging(t, want, []int{1, 2}, func(offset, limit int) ([]string, bool, error) {
			rows, total, hasMore, err := fx.ListTypes(ctx, testSpaceId, offset, limit)
			assert.Equal(t, len(want), total)
			return keysOf(rows, func(row v2model.TypeRow) string { return row.Key }), hasMore, err
		})
	})

	t.Run("properties with one name page by id and count only servable rows", func(t *testing.T) {
		// given: three properties named Same, and one without a key
		fx := newV2Fixture(t)
		for _, key := range []string{"tie2", "tie0", "tie1"} {
			fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:             domain.String("rel-" + key),
				bundle.RelationKeyRelationKey:    domain.String(key),
				bundle.RelationKeyName:           domain.String("Same"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			}})
		}
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("rel-tie05"),
			bundle.RelationKeyName:           domain.String("Same"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		}})
		want := []string{"tie0", "tie1", "tie2"}

		// when, then
		assertConsistentPaging(t, want, []int{1, 2}, func(offset, limit int) ([]string, bool, error) {
			rows, total, hasMore, err := fx.ListProperties(ctx, testSpaceId, offset, limit)
			assert.Equal(t, len(want), total)
			return keysOf(rows, func(row v2model.PropertyRow) string { return row.Key }), hasMore, err
		})
	})

	t.Run("options with one name page by id", func(t *testing.T) {
		// given: three options named Same, added in reverse id order
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("rel-label"),
			bundle.RelationKeyRelationKey:    domain.String("label"),
			bundle.RelationKeyName:           domain.String("Label"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		}})
		for _, option := range [][2]string{{"opt-c", "red"}, {"opt-b", "blue"}, {"opt-a", "green"}} {
			fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:                  domain.String(option[0]),
				bundle.RelationKeyRelationKey:         domain.String("label"),
				bundle.RelationKeyName:                domain.String("Same"),
				bundle.RelationKeyRelationOptionColor: domain.String(option[1]),
				bundle.RelationKeyResolvedLayout:      domain.Int64(int64(model.ObjectType_relationOption)),
			}})
		}
		want := []string{"green", "blue", "red"}

		// when, then
		assertConsistentPaging(t, want, []int{1, 2}, func(offset, limit int) ([]string, bool, error) {
			rows, _, hasMore, err := fx.ListPropertyOptions(ctx, testSpaceId, "label", "", offset, limit)
			return keysOf(rows, func(row v2model.OptionRow) string { return row.Color }), hasMore, err
		})
	})
}

func TestGlobalSearchNarrowHint(t *testing.T) {
	t.Run("a page within the depth cap steers to the next offset", func(t *testing.T) {
		// when
		got := GlobalSearchNarrowHint(0, 25)

		// then
		assert.Equal(t, SearchNarrowHint, got)
	})

	t.Run("a page whose next offset passes the cap steers to space search", func(t *testing.T) {
		// given
		want := "the maximum starting offset of global search is 2000; narrow with filter or query, or search one space at a time"

		// when
		got := GlobalSearchNarrowHint(2000, 1)

		// then
		assert.Equal(t, want, got)
	})
}
