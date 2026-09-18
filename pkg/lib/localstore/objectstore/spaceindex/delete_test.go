package spaceindex

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-store/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// liveObject is an object carrying both relations that must survive deletion and relations that
// must not.
func liveObject(id string) TestObject {
	return TestObject{
		bundle.RelationKeyId:                  domain.String(id),
		bundle.RelationKeySpaceId:             domain.String("test"),
		bundle.RelationKeyCreator:             domain.String("_participant_test_creator"),
		bundle.RelationKeyCreatedDate:         domain.Int64(1000),
		bundle.RelationKeyAddedDate:           domain.Int64(900),
		bundle.RelationKeyCreatedInContext:    domain.String("parentObjectId"),
		bundle.RelationKeyCreatedInContextRef: domain.String("blockId"),
		bundle.RelationKeyLastModifiedBy:      domain.String("_participant_test_editor"),
		bundle.RelationKeyLastModifiedDate:    domain.Int64(2000),
		bundle.RelationKeyType:                domain.String("ot-page"),
		bundle.RelationKeyResolvedLayout:      domain.Int64(int64(model.ObjectType_basic)),
		bundle.RelationKeySizeInBytes:         domain.Int64(4096),
		bundle.RelationKeyFileId:              domain.String("fileCid"),

		// dropped on deletion
		bundle.RelationKeyName:        domain.String("Quarterly report"),
		bundle.RelationKeyDescription: domain.String("secret"),
		bundle.RelationKeySnippet:     domain.String("body text"),
		bundle.RelationKeyIsFavorite:  domain.Bool(true),
	}
}

func TestDeleteObject_PreservesAuditRelations(t *testing.T) {
	t.Run("audit relations survive nested, user content does not", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{liveObject("id1")})
		want := makeDetails(TestObject{
			bundle.RelationKeyId:        domain.String("id1"),
			bundle.RelationKeySpaceId:   domain.String("test"),
			bundle.RelationKeyIsDeleted: domain.Bool(true),
			bundle.RelationKeyDeletedSnapshot: domain.NewValueMap(map[string]domain.Value{
				"creator":             domain.String("_participant_test_creator"),
				"createdDate":         domain.Int64(1000),
				"addedDate":           domain.Int64(900),
				"createdInContext":    domain.String("parentObjectId"),
				"createdInContextRef": domain.String("blockId"),
				"lastModifiedBy":      domain.String("_participant_test_editor"),
				"lastModifiedDate":    domain.Int64(2000),
				"type":                domain.String("ot-page"),
				"resolvedLayout":      domain.Int64(int64(model.ObjectType_basic)),
				"sizeInBytes":         domain.Int64(4096),
				"fileId":              domain.String("fileCid"),
			}),
		})

		// when
		err := s.DeleteObject("id1")

		// then
		require.NoError(t, err)
		got, err := s.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	// The nesting exists to keep these out of the objects collection's indexes. fileId's index is
	// sparse, so a top-level fileId would add rows to an index that holds no tombstones at all;
	// resolvedLayout, type and lastModifiedDate are non-sparse, so a top-level value would move the
	// tombstone out of the null bucket into the ranges live queries scan. Any of them reappearing at
	// the top level is the regression this guards.
	t.Run("a derived object's identity keys survive inside the snapshot", func(t *testing.T) {
		// given: a relation object, as the space stores one
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{{
			bundle.RelationKeyId:             domain.String("rel1"),
			bundle.RelationKeySpaceId:        domain.String("space1"),
			bundle.RelationKeyUniqueKey:      domain.String("rel-6a7663db61fab21cd4b9e201"),
			bundle.RelationKeyRelationKey:    domain.String("6a7663db61fab21cd4b9e201"),
			bundle.RelationKeyApiObjectKey:   domain.String("warranty_until"),
			bundle.RelationKeyName:           domain.String("Warranty until"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		}})

		// when
		require.NoError(t, s.DeleteObject("rel1"))

		// then: nothing indexed at the top level, the identity inside
		got, err := s.GetDetails("rel1")
		require.NoError(t, err)
		assert.False(t, got.Has(bundle.RelationKeyRelationKey), "relationKey must stay inside deletedSnapshot")
		assert.False(t, got.Has(bundle.RelationKeyName), "no user-authored content survives")
		snapshot, ok := got.TryMapValue(bundle.RelationKeyDeletedSnapshot)
		require.True(t, ok)
		assert.Equal(t, "warranty_until", snapshot.GetString(bundle.RelationKeyApiObjectKey.String()))
		assert.Equal(t, "6a7663db61fab21cd4b9e201", snapshot.GetString(bundle.RelationKeyRelationKey.String()))
		assert.Equal(t, "rel-6a7663db61fab21cd4b9e201", snapshot.GetString(bundle.RelationKeyUniqueKey.String()))
	})

	t.Run("indexed relations never appear at the top level", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{liveObject("id1")})
		indexed := []domain.RelationKey{
			bundle.RelationKeyResolvedLayout,
			bundle.RelationKeyType,
			bundle.RelationKeyLastModifiedDate,
			bundle.RelationKeyFileId,
		}

		// when
		require.NoError(t, s.DeleteObject("id1"))
		// twice: a re-delete must not promote the snapshot back up
		require.NoError(t, s.DeleteObject("id1"))

		// then
		got, err := s.GetDetails("id1")
		require.NoError(t, err)
		for _, key := range indexed {
			assert.False(t, got.Has(key), "%s must stay inside deletedSnapshot", key)
		}
		snapshot, ok := got.TryMapValue(bundle.RelationKeyDeletedSnapshot)
		require.True(t, ok)
		for _, key := range indexed {
			assert.True(t, snapshot.Has(key.String()), "%s missing from deletedSnapshot", key)
		}
	})

	t.Run("re-deleting keeps the preserved values", func(t *testing.T) {
		// reindexDeletedObjects calls DeleteObject for every deleted tree id on every reindex, so a
		// second call must not wipe what the first one kept
		// given
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{liveObject("id1")})
		require.NoError(t, s.DeleteObject("id1"))
		want, err := s.GetDetails("id1")
		require.NoError(t, err)

		// when
		err = s.DeleteObject("id1")

		// then
		require.NoError(t, err)
		got, err := s.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("deletion-side relations survive a later re-delete", func(t *testing.T) {
		// the audit materializer stamps these onto an already-deleted object
		// given
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{liveObject("id1")})
		require.NoError(t, s.DeleteObject("id1"))
		err := s.ModifyObjectDetails("id1", func(details *domain.Details) (*domain.Details, bool, error) {
			details.SetString(bundle.RelationKeyDeletedBy, "_participant_test_deleter")
			details.SetInt64(bundle.RelationKeyDeletedDate, 3000)
			details.SetString(bundle.RelationKeyDeletionChangeId, "changeCid")
			return details, true, nil
		}, false)
		require.NoError(t, err)

		// when
		err = s.DeleteObject("id1")

		// then
		require.NoError(t, err)
		got, err := s.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, "_participant_test_deleter", got.GetString(bundle.RelationKeyDeletedBy))
		assert.Equal(t, int64(3000), got.GetInt64(bundle.RelationKeyDeletedDate))
		assert.Equal(t, "changeCid", got.GetString(bundle.RelationKeyDeletionChangeId))
		snapshot, ok := got.TryMapValue(bundle.RelationKeyDeletedSnapshot)
		require.True(t, ok)
		assert.Equal(t, "_participant_test_creator", snapshot.GetString("creator"))
	})

	// QueryRaw applies no implicit isDeleted filter, so it is what a caller reaching for an indexed
	// key sees. Several such callers exist (installer.go, kanban) and today they each happen to carry
	// their own isDeleted clause; nesting is what makes that not matter.
	t.Run("QueryRaw on an indexed key does not reach tombstones", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{liveObject("id1"), liveObject("id2")})
		require.NoError(t, s.DeleteObject("id1"))

		for _, tc := range []struct {
			name   string
			filter database.Filter
		}{
			{"fileId (sparse index)", database.FilterEq{
				Key: bundle.RelationKeyFileId, Cond: model.BlockContentDataviewFilter_Equal,
				Value: domain.String("fileCid"),
			}},
			{"resolvedLayout", database.FilterEq{
				Key: bundle.RelationKeyResolvedLayout, Cond: model.BlockContentDataviewFilter_Equal,
				Value: domain.Int64(int64(model.ObjectType_basic)),
			}},
			{"type", database.FilterEq{
				Key: bundle.RelationKeyType, Cond: model.BlockContentDataviewFilter_Equal,
				Value: domain.String("ot-page"),
			}},
			{"lastModifiedDate", database.FilterEq{
				Key: bundle.RelationKeyLastModifiedDate, Cond: model.BlockContentDataviewFilter_Equal,
				Value: domain.Int64(2000),
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				// when
				got, err := s.QueryRaw(&database.Filters{FilterObj: tc.filter}, 0, 0)

				// then
				require.NoError(t, err)
				require.Len(t, got, 1, "only the live object matches")
				assert.Equal(t, "id2", got[0].Details.GetString(bundle.RelationKeyId))
			})
		}
	})

	t.Run("object that was never indexed gets a bare tombstone", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		want := makeDetails(TestObject{
			bundle.RelationKeyId:        domain.String("neverSeen"),
			bundle.RelationKeySpaceId:   domain.String("test"),
			bundle.RelationKeyIsDeleted: domain.Bool(true),
		})

		// when
		err := s.DeleteObject("neverSeen")

		// then
		require.NoError(t, err)
		got, err := s.GetDetails("neverSeen")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
}

func TestDeletionAuditMark(t *testing.T) {
	t.Run("unset mark reads as empty", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		// when
		got, err := s.GetDeletionAuditMark()

		// then
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("mark round-trips and overwrites", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		require.NoError(t, s.SetDeletionAuditMark("headA"))

		// when
		require.NoError(t, s.SetDeletionAuditMark("headA,headB"))
		got, err := s.GetDeletionAuditMark()

		// then
		require.NoError(t, err)
		assert.Equal(t, "headA,headB", got)
	})
}

// The deletion audit is only affordable because of this index: without it, listing removals is a
// full scan of the objects collection plus an in-memory sort, on every page.
func TestDeletedDateIndex(t *testing.T) {
	// seed writes 200 live objects and 20 removed ones.
	seed := func(t *testing.T, s *StoreFixture) {
		var objs []TestObject
		for i := 0; i < 200; i++ {
			objs = append(objs, TestObject{
				bundle.RelationKeyId:      domain.String(fmt.Sprintf("live%d", i)),
				bundle.RelationKeySpaceId: domain.String("test"),
			})
		}
		for i := 0; i < 20; i++ {
			objs = append(objs, TestObject{
				bundle.RelationKeyId:          domain.String(fmt.Sprintf("gone%d", i)),
				bundle.RelationKeySpaceId:     domain.String("test"),
				bundle.RelationKeyIsDeleted:   domain.Bool(true),
				bundle.RelationKeyDeletedDate: domain.Int64(int64(1000 + i)),
			})
		}
		s.AddObjects(t, objs)
	}

	// Mirrors deletionaudit.auditFilters(). FilterNotNull, not FilterExists: see the index comment
	// in store.go — a sparse index cannot serve $exists, which also matches an explicit null.
	auditFilter := database.FiltersAnd{
		database.FilterEq{
			Key: bundle.RelationKeyIsDeleted, Cond: model.BlockContentDataviewFilter_Equal,
			Value: domain.Bool(true),
		},
		database.FilterNotNull{Key: bundle.RelationKeyDeletedDate},
	}

	t.Run("the audit query and sort use it", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		seed(t, s)
		sort, err := query.ParseSort("-"+bundle.RelationKeyDeletedDate.String(), bundle.RelationKeyId.String())
		require.NoError(t, err)

		// when
		explain, err := s.objects.Find(auditFilter.AnystoreFilter()).Sort(sort).Explain(ctx)

		// then
		require.NoError(t, err)
		used := map[string]bool{}
		for _, idx := range explain.Indexes {
			used[idx.Name] = idx.Used
		}
		assert.True(t, used["deletedDate"], "audit query fell back to a full scan: %s", explain.Sql)
	})

	t.Run("sparse holds only rows carrying deletedDate", func(t *testing.T) {
		// The plan INNER JOINs the index table, so a doc with no index entry cannot come back. With
		// a filter that guarantees deletedDate is present the join is sound, and the rows it yields
		// are exactly the ones the index stores.
		// given
		s := NewStoreFixture(t)
		seed(t, s)
		sort, err := query.ParseSort("-" + bundle.RelationKeyDeletedDate.String())
		require.NoError(t, err)
		present := database.FilterNotNull{Key: bundle.RelationKeyDeletedDate}

		// when
		iter, err := s.objects.Find(present.AnystoreFilter()).Sort(sort).Iter(ctx)
		require.NoError(t, err)
		defer iter.Close()
		var indexed int
		for iter.Next() {
			indexed++
		}

		// then
		assert.Equal(t, 20, indexed, "index must hold only rows carrying deletedDate")
	})

	t.Run("a sort alone cannot pull a query onto it", func(t *testing.T) {
		// Regression guard for GO-7510. Before any-store v1.0.2 a sort on deletedDate forced this
		// sparse index onto a query that said nothing about deletedDate, and the INNER JOIN then
		// silently dropped the 200 live objects the index never stored. The planner must now refuse
		// the index and return every row.
		// given
		s := NewStoreFixture(t)
		seed(t, s)
		sort, err := query.ParseSort("-" + bundle.RelationKeyDeletedDate.String())
		require.NoError(t, err)
		everything := database.FilterExists{Key: bundle.RelationKeyId}

		// when
		iter, err := s.objects.Find(everything.AnystoreFilter()).Sort(sort).Iter(ctx)
		require.NoError(t, err)
		defer iter.Close()
		var got int
		for iter.Next() {
			got++
		}
		total, err := s.objects.Find(everything.AnystoreFilter()).Count(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, 220, total)
		assert.Equal(t, 220, got, "sorting by a sparse field must not drop rows that lack it")
	})
}

func TestCountRaw(t *testing.T) {
	t.Run("counts deleted objects that Query would hide", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{liveObject("id1"), liveObject("id2"), liveObject("id3")})
		require.NoError(t, s.DeleteObject("id1"))
		require.NoError(t, s.DeleteObject("id2"))
		filters := &database.Filters{FilterObj: database.FilterEq{
			Key:   bundle.RelationKeyIsDeleted,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: domain.Bool(true),
		}}

		// when
		got, err := s.CountRaw(filters)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, got)
	})

	t.Run("nil filters are rejected", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		// when
		_, err := s.CountRaw(nil)

		// then
		require.Error(t, err)
	})
}

func TestDeleteObject_DerivedTombstoneKeepsItsLayoutTopLevel(t *testing.T) {
	// a deleted type's tombstone is findable as a type through an indexed
	// query; an ordinary object's tombstone carries no such marker
	s := NewStoreFixture(t)
	s.AddObjects(t, []TestObject{
		{
			bundle.RelationKeyId:             domain.String("typeA"),
			bundle.RelationKeySpaceId:        domain.String("test"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-typeA"),
			bundle.RelationKeyApiObjectKey:   domain.String("type_a"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		},
		liveObject("page1"),
	})

	require.NoError(t, s.DeleteObject("typeA"))
	require.NoError(t, s.DeleteObject("page1"))

	typeRow, err := s.GetDetails("typeA")
	require.NoError(t, err)
	assert.Equal(t, int64(model.ObjectType_objectType), typeRow.GetInt64(bundle.RelationKeyDeletedLayout))
	assert.False(t, typeRow.Has(bundle.RelationKeyResolvedLayout), "the indexed live layout stays out of the tombstone")
	pageRow, err := s.GetDetails("page1")
	require.NoError(t, err)
	assert.False(t, pageRow.Has(bundle.RelationKeyDeletedLayout))

	// re-deleting keeps the marker
	require.NoError(t, s.DeleteObject("typeA"))
	typeRow, err = s.GetDetails("typeA")
	require.NoError(t, err)
	assert.Equal(t, int64(model.ObjectType_objectType), typeRow.GetInt64(bundle.RelationKeyDeletedLayout))

	// a tombstone written before the marker existed is backfilled by the
	// re-delete a reindex runs, from the snapshot it kept
	s.AddObjects(t, []TestObject{{
		bundle.RelationKeyId:        domain.String("typeOld"),
		bundle.RelationKeySpaceId:   domain.String("test"),
		bundle.RelationKeyIsDeleted: domain.Bool(true),
		bundle.RelationKeyDeletedSnapshot: domain.NewValueMap(map[string]domain.Value{
			bundle.RelationKeyResolvedLayout.String(): domain.Int64(int64(model.ObjectType_objectType)),
			bundle.RelationKeyUniqueKey.String():      domain.String("ot-typeOld"),
			bundle.RelationKeyApiObjectKey.String():   domain.String("type_old"),
		}),
	}})
	require.NoError(t, s.DeleteObject("typeOld"))
	oldRow, err := s.GetDetails("typeOld")
	require.NoError(t, err)
	assert.Equal(t, int64(model.ObjectType_objectType), oldRow.GetInt64(bundle.RelationKeyDeletedLayout))
	snapshot, ok := oldRow.TryMapValue(bundle.RelationKeyDeletedSnapshot)
	require.True(t, ok)
	assert.Equal(t, "type_old", snapshot.GetString(bundle.RelationKeyApiObjectKey.String()), "the snapshot is kept as it was")
}

func TestBackfillDeletedLayout(t *testing.T) {
	s := NewStoreFixture(t)
	ctx := context.Background()
	// a legacy type tombstone (snapshot, no marker), a legacy relation
	// tombstone, a bare tombstone, an ordinary object's tombstone
	s.AddObjects(t, []TestObject{
		{
			bundle.RelationKeyId: domain.String("typeOld"), bundle.RelationKeySpaceId: domain.String("test"), bundle.RelationKeyIsDeleted: domain.Bool(true),
			bundle.RelationKeyDeletedSnapshot: domain.NewValueMap(map[string]domain.Value{
				bundle.RelationKeyResolvedLayout.String(): domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyUniqueKey.String():      domain.String("ot-typeOld"),
				bundle.RelationKeyApiObjectKey.String():   domain.String("type_old"),
			}),
		},
		{
			bundle.RelationKeyId: domain.String("relOld"), bundle.RelationKeySpaceId: domain.String("test"), bundle.RelationKeyIsDeleted: domain.Bool(true),
			bundle.RelationKeyDeletedSnapshot: domain.NewValueMap(map[string]domain.Value{
				bundle.RelationKeyResolvedLayout.String(): domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyRelationKey.String():    domain.String("relOldKey"),
			}),
		},
		{bundle.RelationKeyId: domain.String("bare"), bundle.RelationKeySpaceId: domain.String("test"), bundle.RelationKeyIsDeleted: domain.Bool(true)},
		{
			bundle.RelationKeyId: domain.String("pageOld"), bundle.RelationKeySpaceId: domain.String("test"), bundle.RelationKeyIsDeleted: domain.Bool(true),
			bundle.RelationKeyDeletedSnapshot: domain.NewValueMap(map[string]domain.Value{
				bundle.RelationKeyResolvedLayout.String(): domain.Int64(int64(model.ObjectType_basic)),
			}),
		},
	})

	require.NoError(t, s.BackfillDeletedLayout(ctx))

	for id, want := range map[string]int64{"typeOld": int64(model.ObjectType_objectType), "relOld": int64(model.ObjectType_relation)} {
		row, err := s.GetDetails(id)
		require.NoError(t, err)
		assert.Equal(t, want, row.GetInt64(bundle.RelationKeyDeletedLayout), id)
		assert.True(t, row.GetBool(bundle.RelationKeyIsDeleted), "still a tombstone")
	}
	for _, id := range []string{"bare", "pageOld"} {
		row, err := s.GetDetails(id)
		require.NoError(t, err)
		assert.False(t, row.Has(bundle.RelationKeyDeletedLayout), id)
	}
	marker, err := s.GetReconcileMarker(ctx, deletedLayoutBackfillMarkerId)
	require.NoError(t, err)
	assert.Equal(t, "1", marker, "completion recorded")

	// idempotent: a second run is a marker read
	require.NoError(t, s.BackfillDeletedLayout(ctx))

	// the marked tombstone is what the exact query finds
	records, err := s.Query(database.Query{Filters: []database.FilterRequest{
		{RelationKey: bundle.RelationKeyDeletedLayout, Condition: model.BlockContentDataviewFilter_Equal, Value: domain.Int64(int64(model.ObjectType_objectType))},
		{RelationKey: bundle.RelationKeyIsDeleted, Condition: model.BlockContentDataviewFilter_Equal, Value: domain.Bool(true)},
		{RelationKey: bundle.RelationKeyIsArchived, Condition: model.BlockContentDataviewFilter_None},
	}})
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "typeOld", records[0].Details.GetString(bundle.RelationKeyId))
}
