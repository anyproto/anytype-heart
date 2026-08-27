package deletionaudit

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const testSpaceId = "test"

func testIdentity(t *testing.T) crypto.PubKey {
	_, pub, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)
	return pub
}

// deleteChange builds the settings tree change that deleting ids produces. PreviousIds is non-empty
// because only the tree root has none, and materialize skips the root.
func deleteChange(id string, identity crypto.PubKey, timestamp int64, deletedIds ...string) *objecttree.Change {
	content := make([]*spacesyncproto.SpaceSettingsContent, 0, len(deletedIds))
	for _, deletedId := range deletedIds {
		content = append(content, &spacesyncproto.SpaceSettingsContent{
			Value: &spacesyncproto.SpaceSettingsContent_ObjectDelete{
				ObjectDelete: &spacesyncproto.ObjectDelete{Id: deletedId},
			},
		})
	}
	return &objecttree.Change{
		Id:          id,
		PreviousIds: []string{"previous"},
		Identity:    identity,
		Timestamp:   timestamp,
		Model:       &spacesyncproto.SettingsData{Content: content},
	}
}

func TestApplyChange(t *testing.T) {
	t.Run("stamps the deletion facts onto an existing tombstone", func(t *testing.T) {
		// given
		s := &service{}
		index := spaceindex.NewStoreFixture(t)
		index.AddObjects(t, []spaceindex.TestObject{{
			bundle.RelationKeyId:          domain.String("id1"),
			bundle.RelationKeySpaceId:     domain.String(testSpaceId),
			bundle.RelationKeyCreator:     domain.String("_participant_test_creator"),
			bundle.RelationKeyCreatedDate: domain.Int64(1000),
			bundle.RelationKeyName:        domain.String("Quarterly report"),
		}})
		require.NoError(t, index.DeleteObject("id1"))
		identity := testIdentity(t)
		change := deleteChange("changeCid", identity, 3000, "id1")

		// when
		err := s.applyChange(context.Background(), index, testSpaceId, change, change.Model.(*spacesyncproto.SettingsData))

		// then
		require.NoError(t, err)
		got, err := index.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, domain.NewParticipantId(testSpaceId, identity.Account()), got.GetString(bundle.RelationKeyDeletedBy))
		assert.Equal(t, int64(3000), got.GetInt64(bundle.RelationKeyDeletedDate))
		assert.Equal(t, "changeCid", got.GetString(bundle.RelationKeyDeletionChangeId))
		// the creation side the tombstone kept is untouched
		snapshot, ok := got.TryMapValue(bundle.RelationKeyDeletedSnapshot)
		require.True(t, ok)
		assert.Equal(t, "_participant_test_creator", snapshot.GetString("creator"))
		assert.Equal(t, int64(1000), snapshot.GetInt64("createdDate"))
		// the name never survived the deletion and is not resurrected
		assert.Empty(t, got.GetString(bundle.RelationKeyName))
		assert.Empty(t, snapshot.GetString("name"))
	})

	t.Run("creates a tombstone for an object this device never indexed", func(t *testing.T) {
		// a device that joined the space after the deletion has no row to stamp
		// given
		s := &service{}
		index := spaceindex.NewStoreFixture(t)
		identity := testIdentity(t)
		change := deleteChange("changeCid", identity, 3000, "neverSeen")

		// when
		err := s.applyChange(context.Background(), index, testSpaceId, change, change.Model.(*spacesyncproto.SettingsData))

		// then
		require.NoError(t, err)
		got, err := index.GetDetails("neverSeen")
		require.NoError(t, err)
		assert.True(t, got.GetBool(bundle.RelationKeyIsDeleted))
		assert.Equal(t, testSpaceId, got.GetString(bundle.RelationKeySpaceId))
		assert.Equal(t, "changeCid", got.GetString(bundle.RelationKeyDeletionChangeId))
		assert.False(t, got.Has(bundle.RelationKeyDeletedSnapshot), "nothing was ever known about it")
	})

	t.Run("a cascade batch shares one change id and timestamp", func(t *testing.T) {
		// deleting a parent cascades to its bound children in a single settings change
		// given
		s := &service{}
		index := spaceindex.NewStoreFixture(t)
		identity := testIdentity(t)
		change := deleteChange("changeCid", identity, 3000, "parent", "child1", "child2")

		// when
		err := s.applyChange(context.Background(), index, testSpaceId, change, change.Model.(*spacesyncproto.SettingsData))

		// then
		require.NoError(t, err)
		for _, id := range []string{"parent", "child1", "child2"} {
			got, err := index.GetDetails(id)
			require.NoError(t, err)
			assert.Equal(t, "changeCid", got.GetString(bundle.RelationKeyDeletionChangeId), id)
			assert.Equal(t, int64(3000), got.GetInt64(bundle.RelationKeyDeletedDate), id)
		}
	})

	t.Run("replaying a change is idempotent", func(t *testing.T) {
		// given
		s := &service{}
		index := spaceindex.NewStoreFixture(t)
		identity := testIdentity(t)
		change := deleteChange("changeCid", identity, 3000, "id1")
		data := change.Model.(*spacesyncproto.SettingsData)
		require.NoError(t, s.applyChange(context.Background(), index, testSpaceId, change, data))
		want, err := index.GetDetails("id1")
		require.NoError(t, err)

		// when
		err = s.applyChange(context.Background(), index, testSpaceId, change, data)

		// then
		require.NoError(t, err)
		got, err := index.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("a snapshot's accumulated ids are ignored", func(t *testing.T) {
		// a snapshot restates every id deleted so far with no idea who deleted each; reading it
		// would attribute other people's deletions to whoever triggered the snapshot
		// given
		s := &service{}
		index := spaceindex.NewStoreFixture(t)
		identity := testIdentity(t)
		change := deleteChange("changeCid", identity, 3000, "mine")
		data := change.Model.(*spacesyncproto.SettingsData)
		data.Snapshot = &spacesyncproto.SpaceSettingsSnapshot{DeletedIds: []string{"mine", "someoneElses"}}

		// when
		err := s.applyChange(context.Background(), index, testSpaceId, change, data)

		// then
		require.NoError(t, err)
		got, err := index.GetDetails("someoneElses")
		require.NoError(t, err)
		assert.Empty(t, got.GetString(bundle.RelationKeyDeletionChangeId))
	})
}

// Compare must rank the same way AnystoreSort does. Nothing forces them to agree, and a mismatch
// would only show up as records shuffling when the same order is applied in memory.
func TestDeletedDateDesc_Compare(t *testing.T) {
	rec := func(id string, deletedDate int64) *domain.Details {
		d := domain.NewDetails()
		d.SetString(bundle.RelationKeyId, id)
		d.SetInt64(bundle.RelationKeyDeletedDate, deletedDate)
		return d
	}
	order := deletedDateDesc{}

	assert.Negative(t, order.Compare(rec("a", 2000), rec("b", 1000)), "newer sorts first")
	assert.Positive(t, order.Compare(rec("a", 1000), rec("b", 2000)), "older sorts last")
	assert.Negative(t, order.Compare(rec("a", 2000), rec("b", 2000)), "equal dates tiebreak on id")
	assert.Positive(t, order.Compare(rec("b", 2000), rec("a", 2000)), "equal dates tiebreak on id")
	assert.Zero(t, order.Compare(rec("a", 2000), rec("a", 2000)))
}

// uninstallChange builds the object-tree change that setting isUninstalled produces.
func uninstallChange(id string, identity crypto.PubKey, timestamp int64, value bool) *objecttree.Change {
	return &objecttree.Change{
		Id:          id,
		PreviousIds: []string{"previous"},
		Identity:    identity,
		Timestamp:   timestamp,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{{
				Value: &pb.ChangeContentValueOfDetailsSet{
					DetailsSet: &pb.ChangeDetailsSet{
						Key:   bundle.RelationKeyIsUninstalled.String(),
						Value: pbtypes.Bool(value),
					},
				},
			}},
		},
	}
}

// pickUninstallChange mirrors findUninstallChange's selection over an in-memory change list. The tree
// build is what makes findUninstallChange itself awkward to unit test; the rule it applies is not.
func pickUninstallChange(changes []*objecttree.Change) *objecttree.Change {
	var (
		last      *objecttree.Change
		lastValue bool
	)
	for _, change := range changes {
		model, ok := change.Model.(*pb.Change)
		if !ok {
			continue
		}
		for _, content := range model.GetContent() {
			set := content.GetDetailsSet()
			if (set == nil) || (set.GetKey() != bundle.RelationKeyIsUninstalled.String()) {
				continue
			}
			last = change
			lastValue = set.GetValue().GetBoolValue()
		}
	}
	if !lastValue {
		return nil
	}
	return last
}

func TestUninstallChangeSelection(t *testing.T) {
	identityA, identityB := testIdentity(t), testIdentity(t)

	t.Run("picks the only uninstall", func(t *testing.T) {
		// given
		changes := []*objecttree.Change{uninstallChange("c1", identityA, 1000, true)}

		// when
		got := pickUninstallChange(changes)

		// then
		require.NotNil(t, got)
		assert.Equal(t, "c1", got.Id)
	})

	t.Run("reinstall then uninstall reports the latest uninstaller", func(t *testing.T) {
		// uninstalling is reversible, so a type can cross the line repeatedly; only the most recent
		// crossing describes how it got to where it is now
		// given
		changes := []*objecttree.Change{
			uninstallChange("c1", identityA, 1000, true),
			uninstallChange("c2", identityA, 2000, false), // reinstalled
			uninstallChange("c3", identityB, 3000, true),  // uninstalled again, by someone else
		}

		// when
		got := pickUninstallChange(changes)

		// then
		require.NotNil(t, got)
		assert.Equal(t, "c3", got.Id)
		assert.Equal(t, int64(3000), got.Timestamp)
		assert.True(t, got.Identity.Equals(identityB))
	})

	t.Run("a reinstalled object has no current uninstaller", func(t *testing.T) {
		// given
		changes := []*objecttree.Change{
			uninstallChange("c1", identityA, 1000, true),
			uninstallChange("c2", identityA, 2000, false),
		}

		// when
		got := pickUninstallChange(changes)

		// then — the last crossing set it back to false, so nothing to report
		assert.Nil(t, got)
	})

	t.Run("ignores changes that touch other relations", func(t *testing.T) {
		// given
		other := &objecttree.Change{
			Id: "c1", PreviousIds: []string{"previous"}, Identity: identityA, Timestamp: 1000,
			Model: &pb.Change{Content: []*pb.ChangeContent{{
				Value: &pb.ChangeContentValueOfDetailsSet{DetailsSet: &pb.ChangeDetailsSet{
					Key: bundle.RelationKeyName.String(), Value: pbtypes.String("Task"),
				}},
			}}},
		}

		// when
		got := pickUninstallChange([]*objecttree.Change{other})

		// then
		assert.Nil(t, got)
	})
}

func TestUninstalledFilters(t *testing.T) {
	t.Run("selects uninstalled rows that are not yet materialized", func(t *testing.T) {
		// given
		index := spaceindex.NewStoreFixture(t)
		index.AddObjects(t, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:            domain.String("pending"),
				bundle.RelationKeySpaceId:       domain.String(testSpaceId),
				bundle.RelationKeyIsUninstalled: domain.Bool(true),
				bundle.RelationKeyIsDeleted:     domain.Bool(true),
			},
			{
				// already stamped: the indexer has not wiped it, so leave it alone
				bundle.RelationKeyId:            domain.String("done"),
				bundle.RelationKeySpaceId:       domain.String(testSpaceId),
				bundle.RelationKeyIsUninstalled: domain.Bool(true),
				bundle.RelationKeyIsDeleted:     domain.Bool(true),
				bundle.RelationKeyDeletedDate:   domain.Int64(3000),
			},
			{
				// a live type
				bundle.RelationKeyId:      domain.String("installed"),
				bundle.RelationKeySpaceId: domain.String(testSpaceId),
			},
		})

		// when
		got, err := index.QueryRaw(uninstalledFilters(), 0, 0)

		// then
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "pending", got[0].Details.GetString(bundle.RelationKeyId))
	})
}

func TestHeadsMark(t *testing.T) {
	t.Run("head order does not change the mark", func(t *testing.T) {
		// head order is not stable across reads; an order flip must not look like a moved tree and
		// force a full settings walk
		assert.Equal(t, headsMark([]string{"a", "b"}), headsMark([]string{"b", "a"}))
	})

	t.Run("different heads give different marks", func(t *testing.T) {
		assert.NotEqual(t, headsMark([]string{"a"}), headsMark([]string{"a", "b"}))
	})

	t.Run("no heads is empty", func(t *testing.T) {
		// must not collide with the "never materialized" mark in a way that skips the first walk
		assert.Empty(t, headsMark(nil))
	})

	t.Run("does not mutate the caller's slice", func(t *testing.T) {
		// given — entry.Heads belongs to head storage, not to us
		heads := []string{"b", "a"}

		// when
		headsMark(heads)

		// then
		assert.Equal(t, []string{"b", "a"}, heads)
	})
}

func TestAuditQuery(t *testing.T) {
	// seed writes n deleted objects, each stamped as deleted at the given timestamp.
	seed := func(t *testing.T, index spaceindex.Store, id string, deletedDate int64) {
		err := index.ModifyObjectDetails(id, func(details *domain.Details) (*domain.Details, bool, error) {
			details.SetString(bundle.RelationKeyId, id)
			details.SetString(bundle.RelationKeySpaceId, testSpaceId)
			details.SetBool(bundle.RelationKeyIsDeleted, true)
			details.SetInt64(bundle.RelationKeyDeletedDate, deletedDate)
			details.SetString(bundle.RelationKeyDeletionChangeId, "change-"+id)
			return details, true, nil
		}, true)
		require.NoError(t, err)
	}

	t.Run("returns only materialized tombstones, newest first", func(t *testing.T) {
		// given
		index := spaceindex.NewStoreFixture(t)
		seed(t, index, "old", 1000)
		seed(t, index, "new", 3000)
		seed(t, index, "middle", 2000)
		// a live object, and a tombstone with no deletion change (index churn, not a real deletion)
		index.AddObjects(t, []spaceindex.TestObject{{
			bundle.RelationKeyId:             domain.String("alive"),
			bundle.RelationKeySpaceId:        domain.String(testSpaceId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})
		require.NoError(t, index.DeleteObject("churn"))

		// when
		got, err := index.QueryRaw(auditFilters(), 0, 0)

		// then
		require.NoError(t, err)
		ids := make([]string, 0, len(got))
		for _, r := range got {
			ids = append(ids, r.Details.GetString(bundle.RelationKeyId))
		}
		assert.Equal(t, []string{"new", "middle", "old"}, ids)
	})

	t.Run("total ignores limit and offset", func(t *testing.T) {
		// given
		index := spaceindex.NewStoreFixture(t)
		seed(t, index, "a", 1000)
		seed(t, index, "b", 2000)
		seed(t, index, "c", 3000)

		// when
		page, err := index.QueryRaw(auditFilters(), 2, 1)
		require.NoError(t, err)
		total, err := index.CountRaw(auditFilters())

		// then
		require.NoError(t, err)
		require.Len(t, page, 2)
		assert.Equal(t, "b", page[0].Details.GetString(bundle.RelationKeyId))
		assert.Equal(t, "a", page[1].Details.GetString(bundle.RelationKeyId))
		assert.Equal(t, 3, total)
	})

	t.Run("objects deleted in one change page in a stable order", func(t *testing.T) {
		// they share a timestamp, so only the id tiebreak keeps paging from repeating or skipping
		// given
		index := spaceindex.NewStoreFixture(t)
		seed(t, index, "c", 2000)
		seed(t, index, "a", 2000)
		seed(t, index, "b", 2000)

		// when
		first, err := index.QueryRaw(auditFilters(), 2, 0)
		require.NoError(t, err)
		second, err := index.QueryRaw(auditFilters(), 2, 2)

		// then
		require.NoError(t, err)
		require.Len(t, first, 2)
		require.Len(t, second, 1)
		assert.Equal(t, "a", first[0].Details.GetString(bundle.RelationKeyId))
		assert.Equal(t, "b", first[1].Details.GetString(bundle.RelationKeyId))
		assert.Equal(t, "c", second[0].Details.GetString(bundle.RelationKeyId))
	})
}
