package v2service

// Corpse (uninstalled) property/type ADDRESSABILITY — what each surface does
// with an entity the UI deleted while its values still sit on objects.
//
// The store shape matters and the fixtures here model all THREE (§8.41):
//
//   - "flag-only": {isUninstalled:true} — never persisted by a live index
//     (isDeleted is a source:local relation re-derived on load), but it IS
//     the shape of an export/snapshot of a corpse, and it exercises the
//     explicit isUninstalled filters (the §7.5-req-2 corpse policy) in
//     isolation.
//   - "prod": {isUninstalled:true, isDeleted:true} — the steady state.
//     delete.go's deleteDerivedObject sets isUninstalled and the same Apply
//     injects isDeleted=true (smartblock/detailsinject.go, since GO-1978);
//     after the next space load the row carries full details plus BOTH
//     flags, and a device that received the delete by SYNC has this shape
//     immediately (it never passes through a tombstone). Every plain store
//     query injects `isDeleted != true` (database.go addDefaultFilters), so
//     a prod corpse is hidden from queries even where nothing filters
//     isUninstalled.
//   - "tombstone": {id, spaceId, isDeleted} and NOTHING else — what
//     BeforeDelete leaves in the index (spaceindex.DeleteObject) from the
//     moment of the delete until the next space load, i.e. normally the
//     rest of the app session on the deleting device. No relationKey, no
//     resolvedLayout: every key-filtered query — including the corpse
//     probes' own — misses it on its FIRST filter. Only the derived id
//     (ADDRESSING §2.4: id = f(space, kind, key)) can find it, which is
//     what the §8.41 probes do; the fixture stubs derive `drv-rel-<key>` /
//     `drv-ot-<key>` (newV2FixtureBare) and the tombstone rows sit at
//     those ids, exactly as production rows sit at the real derived ids.
//
// A flag-only fixture cannot catch behavior that depends on the injected
// isDeleted default (the §8.40 lesson), and a full-detail fixture of either
// kind cannot catch behavior that depends on a field EXISTING — the
// tombstone has none, which is how the first two §8.40 fixes were both dead
// for the rest of the session that follows every UI delete (§8.41).
//
// Fixture discipline: the corpse is BSON-keyed (24-hex) WITH a stored
// apiObjectKey slug — the one shape that can tell the two vocabularies
// apart. If any surface started emitting or resolving the corpse's slug, or
// resolving its BSON as a live address, these assertions flip; a readable or
// bundled corpse key could not detect either.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	// stored key and slug of the corpse property every test here shares
	corpseBsonKey = "6a7663db61fab21cd4b9e201"
	corpseSlug    = "warranty_until"
	// internal key and slug of the corpse type
	corpseTypeBsonKey = "6a7663db61fab21cd4b9e301"
	corpseTypeSlug    = "old_meeting"
)

// The corpse rows sit at their DERIVED ids — the fixture stub's derivation
// (newV2FixtureBare) mirrors production, where a derived object's id is a
// pure function of (space, kind, key). This is what lets the tombstone leg
// find the row the way the real probes do.
const (
	corpsePropertyId = "drv-rel-" + corpseBsonKey
	corpseTypeId     = "drv-ot-" + corpseTypeBsonKey
)

// corpseShape is one of the three store shapes a corpse can have — see the
// file header.
type corpseShape int

const (
	corpseFlagOnly corpseShape = iota
	corpseProd
	corpseTombstone
)

// corpseShapes runs a subtest against every store shape a corpse can have.
func corpseShapes(t *testing.T, run func(t *testing.T, shape corpseShape)) {
	t.Run("flag-only shape", func(t *testing.T) { run(t, corpseFlagOnly) })
	t.Run("prod shape", func(t *testing.T) { run(t, corpseProd) })
	t.Run("tombstone shape", func(t *testing.T) { run(t, corpseTombstone) })
}

// addTombstone registers the {id, spaceId, isDeleted} row DeleteObject
// leaves — deliberately NOT through addRelation/addType, which would inject
// a resolvedLayout the real tombstone does not have.
func (fx *v2Fixture) addTombstone(t *testing.T, id string) {
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:        domain.String(id),
		bundle.RelationKeySpaceId:   domain.String(testSpaceId),
		bundle.RelationKeyIsDeleted: domain.Bool(true),
	}})
}

// addCorpseProperty registers the BSON-keyed, slug-bearing corpse relation.
func (fx *v2Fixture) addCorpseProperty(t *testing.T, shape corpseShape) {
	if shape == corpseTombstone {
		fx.addTombstone(t, corpsePropertyId)
		return
	}
	obj := objectstore.TestObject{
		bundle.RelationKeyId:            domain.String(corpsePropertyId),
		bundle.RelationKeyRelationKey:   domain.String(corpseBsonKey),
		bundle.RelationKeyApiObjectKey:  domain.String(corpseSlug),
		bundle.RelationKeyName:          domain.String("Warranty until"),
		bundle.RelationKeyIsUninstalled: domain.Bool(true),
	}
	if shape == corpseProd {
		obj[bundle.RelationKeyIsDeleted] = domain.Bool(true)
	}
	fx.addRelation(t, testSpaceId, obj)
}

func (fx *v2Fixture) addCorpseType(t *testing.T, shape corpseShape) {
	if shape == corpseTombstone {
		fx.addTombstone(t, corpseTypeId)
		return
	}
	obj := objectstore.TestObject{
		bundle.RelationKeyId:            domain.String(corpseTypeId),
		bundle.RelationKeyUniqueKey:     domain.String("ot-" + corpseTypeBsonKey),
		bundle.RelationKeyApiObjectKey:  domain.String(corpseTypeSlug),
		bundle.RelationKeyName:          domain.String("Old meeting"),
		bundle.RelationKeyIsUninstalled: domain.Bool(true),
	}
	if shape == corpseProd {
		obj[bundle.RelationKeyIsDeleted] = domain.Bool(true)
	}
	fx.addType(t, testSpaceId, obj)
}

// corpseHeldRead is a live read of an object still carrying the corpse's
// value under the stored BSON key, typed by the corpse type.
func corpseHeldRead() apicore.ObjectRead {
	return apicore.ObjectRead{
		SbType: model.SmartBlockType_Page,
		Snapshot: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				"id":          pbtypes.String("obj1"),
				"name":        pbtypes.String("Doc"),
				corpseBsonKey: pbtypes.String("2027-01-01"),
			}},
			ObjectTypes: []string{"ot-" + corpseTypeBsonKey},
			Blocks: []*model.Block{
				{Id: "obj1", Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			},
		},
		Heads: []string{"headA"},
	}
}

// TestV2CorpseHeldValueReadsUnderStoredKey: GET serves a corpse-held value
// under the raw 24-hex stored key — never the corpse's stored slug, and the
// object's corpse TYPE spells its internal key too. The corpse vacated the
// slug namespace (§8-OQ2), so its slug may already label a NEW live entity;
// emitting it here would mislabel the value. This test is the read half the
// proposed read-emit/write-refuse split would change — if PropertySlug ever
// starts emitting a corpse's stored slug, the first assertion flips.
// Revert check: drop the isUninstalled filter in storeresolver's loadKeyMaps
// and the flag-only subtest serves "warranty_until" instead of the BSON.
func TestV2CorpseHeldValueReadsUnderStoredKey(t *testing.T) {
	corpseShapes(t, func(t *testing.T, shape corpseShape) {
		// given
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, shape)
		fx.addCorpseType(t, shape)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(corpseHeldRead(), nil)

		// when
		body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", ObjectQuery{})

		// then
		require.NoError(t, err)
		doc := decodeBody(t, body)
		props, _ := doc["properties"].(map[string]any)
		assert.Equal(t, "2027-01-01", props[corpseBsonKey], "the value is served, under the stored key")
		assert.NotContains(t, props, corpseSlug, "a corpse's slug is never a served spelling")
		assert.Equal(t, corpseTypeBsonKey, doc["type"], "a corpse type spells its internal key in the envelope")
	})
}

// TestV2CorpseNeverListsNorResolves: the §7.5-req-2 corpse policy on the
// request namespace, with the BSON+slug fixture the original corpsePolicy
// tests lack (their corpse keys are readable words with no stored slug, so
// they cannot see a slug leaking). Covers listings and route addressing by
// stored key, slug and name.
// Revert check: drop the isUninstalled filter in livePropertyFilters
// (keys.go) and every flag-only assertion here fails.
func TestV2CorpseNeverListsNorResolves(t *testing.T) {
	corpseShapes(t, func(t *testing.T, shape corpseShape) {
		// given
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, shape)
		fx.addCorpseType(t, shape)

		// then: not listed
		rows, total, _, err := fx.ListProperties(context.Background(), testSpaceId, 0, 25)
		require.NoError(t, err)
		assert.Empty(t, rows)
		assert.Zero(t, total)
		typeRows, _, _, err := fx.ListTypes(context.Background(), testSpaceId, 0, 25)
		require.NoError(t, err)
		assert.Empty(t, typeRows)

		// and: no spelling addresses it on routes
		for _, input := range []string{corpseBsonKey, corpseSlug, "Warranty until"} {
			_, err := fx.requireLiveProperty(testSpaceId, input, errKeys{})
			requireNotFoundError(t, err)
		}
		for _, input := range []string{corpseTypeBsonKey, corpseTypeSlug} {
			_, _, err := fx.GetType(context.Background(), testSpaceId, input, ObjectQuery{})
			requireNotFoundError(t, err)
		}
	})
}

// TestV2CorpseHeldValueIsUnqueryable: the addressability hole, executed on
// every query channel. The value an object still carries CANNOT be filtered,
// sorted or selected by ANY spelling — the slug vacated (policy) and the
// stored BSON key is refused by the same live-only validation. Together with
// TestV2CorpseHeldValueReadsUnderStoredKey this is the "data intact,
// unreachable through the vocabulary" state this file exists to make
// explicit: per-object reads show the value, the query surface cannot reach
// it. Round-trip testing cannot see this — bytes survive, addresses don't.
func TestV2CorpseHeldValueIsUnqueryable(t *testing.T) {
	ctx := context.Background()
	newFx := func(t *testing.T, shape corpseShape) *v2Fixture {
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, shape)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:               domain.String("note1"),
			bundle.RelationKeyName:             domain.String("Standup"),
			domain.RelationKey(corpseBsonKey):  domain.String("2027-01-01"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyLastModifiedDate: domain.Int64(1000),
		}})
		return fx
	}
	requireBadRequest := func(t *testing.T, err error) {
		t.Helper()
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
	}

	corpseShapes(t, func(t *testing.T, shape corpseShape) {
		t.Run("structured filter, both spellings", func(t *testing.T) {
			fx := newFx(t, shape)
			for _, key := range []string{corpseBsonKey, corpseSlug} {
				_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{
					Filters: json.RawMessage(`[{"property":"` + key + `","condition":"equal","value":"2027-01-01"}]`)}, 0, 25)
				requireBadRequest(t, err)
			}
		})
		t.Run("compact filter string", func(t *testing.T) {
			fx := newFx(t, shape)
			_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{
				Filter: corpseBsonKey + ` = "2027-01-01"`}, 0, 25)
			requireBadRequest(t, err)
		})
		t.Run("sorts", func(t *testing.T) {
			fx := newFx(t, shape)
			_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{
				Sorts: json.RawMessage(`[{"property":"` + corpseBsonKey + `","direction":"asc"}]`)}, 0, 25)
			requireBadRequest(t, err)
		})
		t.Run("fields selection", func(t *testing.T) {
			fx := newFx(t, shape)
			_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{
				Fields: []string{corpseBsonKey}}, 0, 25)
			requireBadRequest(t, err)
		})
	})
}

// TestV2CloneToleranceSurvivesTheProdShape (was
// TestV2CloneToleranceProdCorpseGap, which pinned the gap this fixes).
//
// The advertised loop "a pasted read body creates a copy" is protected by
// propertyKeyHeldByAnyRelation (keys.go) → relationObjectHoldingKey, whose
// query used to suppress only the injected isArchived default. A REAL
// uninstalled corpse also carries isDeleted=true (see the file header), so
// against the prod shape the tolerance found nothing and the create 400'd on
// a document the API itself served — the archived case worked, the
// uninstalled case, which is the common one, did not. §8.40 fixed that with
// the explicit no-op `isDeleted Condition None` clause; §8.41 then found the
// SAME loop dead again for the whole tombstone window — the query keys on
// relationKey, a field the tombstone does not have — and added the
// derived-id fallback. All three shapes now round-trip.
//
// How this fixture can fail: the corpse is BSON-keyed WITH a stored slug, so
// dropping either no-op clause fails the prod leg; dropping the derived-id
// fallback fails the tombstone leg alone; and any resolution of the corpse's
// SLUG instead of its stored key fails the second subtest.
// Revert checks (executed): removing the `isDeleted Condition None` filter
// from relationObjectHoldingKey fails the prod-shape leg — the create 400s
// with "unknown property keys"; removing the derivedRelationRow fallback
// fails the tombstone leg the same way while both full-detail legs stay
// green.
func TestV2CloneToleranceSurvivesTheProdShape(t *testing.T) {
	cloneBody := []byte(`{"formatVersion":"2.0","type":"page","properties":{"name":"Fresh","` + corpseBsonKey + `":"x"}}`)

	t.Run("the clone loop round-trips on EVERY shape", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			// given
			fx := newV2Fixture(t)
			fx.addCorpseProperty(t, shape)
			captured := fx.expectCreate("clone1")
			fx.expectEtagRead("clone1")

			// when — the bytes a GET of a corpse-held object serves
			_, err := fx.CreateObject(context.Background(), testSpaceId, cloneBody, false, true)

			// then — the value lands under the STORED key it was served under
			require.NoError(t, err)
			require.NotNil(t, *captured)
			assert.Equal(t, "x", (*captured).Details.Fields[corpseBsonKey].GetStringValue())
		})
	})

	t.Run("the corpse SLUG is refused on create in both shapes", func(t *testing.T) {
		// the tolerance is a round-trip escape for the STORED key only; the
		// slug vacated the namespace and must not be an address
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newV2Fixture(t)
			fx.addCorpseProperty(t, shape)

			_, err := fx.CreateObject(context.Background(), testSpaceId,
				[]byte(`{"formatVersion":"2.0","type":"page","properties":{"name":"Fresh","`+corpseSlug+`":"x"}}`), false, true)

			apiErr := v2Err(t, err)
			assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		})
	})
}

// TestV2PatchCorpseKeyChannels: PATCH's only corpse escape is the document
// itself, and it survives the prod shape — checkKey (stateops.go) passes any
// key already on the document without consulting the store, so a value the
// object legitimately carries stays editable and removable by its stored
// key. Everything else refuses: the same key off-document, and the corpse's
// slug in every case (the slug is severed by uninstall even for in-document
// values — canonicalization no longer maps it to the stored key).
// Revert check: dropping the in-document escape (`inDoc` in checkKey) fails
// the first subtest (executed). The unset subtest pins the cleanup channel:
// it fails only under a strictly live-only unset (checkKey WITHOUT the
// in-document escape) — merely routing unset through today's checkKey keeps
// it green, because the escape covers it.
func TestV2PatchCorpseKeyChannels(t *testing.T) {
	ctx := context.Background()
	corpseDoc := `{"formatVersion":"2.0","id":"obj1","type":"page","properties":{"name":"Doc","` + corpseBsonKey + `":"2027-01-01"},"blocks":[{"id":"blockOne1","type":"paragraph","text":"hi"}]}`
	cleanDocNoCorpse := `{"formatVersion":"2.0","id":"obj1","type":"page","properties":{"name":"Doc"},"blocks":[{"id":"blockOne1","type":"paragraph","text":"hi"}]}`

	t.Run("set by stored key, value on the document: applies (prod shape)", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, corpseProd)
		captured := fx.expectMutate(editRead(t, corpseDoc), "headB")

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"`+corpseBsonKey+`":"2030-12-31"}}`), "", false, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, "2030-12-31", (*captured).CombinedDetails().GetString(domain.RelationKey(corpseBsonKey)))
	})

	t.Run("set by stored key, value NOT on the document: 400", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, corpseProd)
		fx.expectMutate(editRead(t, cleanDocNoCorpse), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"`+corpseBsonKey+`":"2030-12-31"}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
	})

	t.Run("unset by stored key removes the corpse-held value", func(t *testing.T) {
		// the one cleanup channel a caller has left
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, corpseProd)
		captured := fx.expectMutate(editRead(t, corpseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","unset":["`+corpseBsonKey+`"]}`), "", false, true)

		require.NoError(t, err)
		_, present := (*captured).CombinedDetails().TryString(domain.RelationKey(corpseBsonKey))
		assert.False(t, present, "unset removes the value")
	})

	t.Run("the corpse slug is refused even with the value on the document", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, corpseProd)
		fx.expectMutate(editRead(t, corpseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"`+corpseSlug+`":"2030-12-31"}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
	})
}

// TestV2ViewOpsCorpseKeys: update_view applies the same two-tier rule — a
// corpse key already ON the dataview (a column, filter or sort the surface
// already shows) stays editable and referencable (§8.17: an edit must not
// reject what the surface already shows), while introducing that key to a
// view that does not carry it is refused like any unknown key.
// Revert check: dropping the preKnown escape in validateViewKeys fails the
// in-view subtests.
func TestV2ViewOpsCorpseKeys(t *testing.T) {
	ctx := context.Background()
	corpseViewDocBody := `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
		`{"id":"dataview","type":"dataview",` +
		`"properties":[{"property":"name","format":"text"},{"property":"` + corpseBsonKey + `","format":"date"}],` +
		`"views":[{"id":"viewAll1","name":"All",` +
		`"filters":[{"property":"` + corpseBsonKey + `","condition":"equal","value":"x"}],` +
		`"columns":[{"property":"name"},{"property":"` + corpseBsonKey + `","width":100}]}]}]}`
	plainViewDocBody := `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
		`{"id":"dataview","type":"dataview",` +
		`"properties":[{"property":"name","format":"text"}],` +
		`"views":[{"id":"viewAll1","name":"All","columns":[{"property":"name"}]}]}]}`

	t.Run("a view already showing the corpse key stays editable", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, corpseProd)
		captured := fx.expectMutate(editRead(t, corpseViewDocBody), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","view":"viewAll1","set":{"name":"Renamed"}}`), "", false, true)

		require.NoError(t, err)
		require.NotNil(t, *captured)
	})

	t.Run("groupBy an in-view corpse key is accepted", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, corpseProd)
		fx.expectMutate(editRead(t, corpseViewDocBody), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","view":"viewAll1","set":{"group_by":"`+corpseBsonKey+`"}}`), "", false, true)

		require.NoError(t, err)
	})

	t.Run("introducing the corpse key to a view without it is refused", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, corpseProd)
		for _, op := range []string{
			`{"op":"update_view","view":"viewAll1","set":{"group_by":"` + corpseBsonKey + `"}}`,
			`{"op":"update_view","view":"viewAll1","columns":{"` + corpseBsonKey + `":{"width":80}}}`,
		} {
			fx.expectMutate(editRead(t, plainViewDocBody), "headB")
			_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(op), "", false, true)
			apiErr := v2Err(t, err)
			assert.Equal(t, http.StatusBadRequest, apiErr.Status, "op %s", op)
		}
	})
}

// TestV2CorpseSquatterVacatesBundledSlug: a corpse whose stored slug equals
// a bundled DERIVED slug does not shadow the bundled property — the corpse
// vacated the namespace, so `due_date` resolves cleanly to bundled dueDate
// with no ambiguity. (A LIVE squatter is the loud §7.5a-6 shadow,
// TestV2ShadowedBundledSlugIsLoud.)
// Revert check: dropping the isUninstalled filter in livePropertyFilters
// turns this resolution ambiguous and the test fails.
func TestV2CorpseSquatterVacatesBundledSlug(t *testing.T) {
	// given: an UNINSTALLED squatter of due_date
	fx := newV2Fixture(t)
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:            domain.String("rel-squatter"),
		bundle.RelationKeyRelationKey:   domain.String(corpseBsonKey),
		bundle.RelationKeyApiObjectKey:  domain.String("due_date"),
		bundle.RelationKeyName:          domain.String("Due Date"),
		bundle.RelationKeyIsUninstalled: domain.Bool(true),
	})
	entries, err := fx.liveProperties(testSpaceId)
	require.NoError(t, err)

	// when
	entry, ok, ambiguous := fx.resolvePropertyInput("due_date", entries)

	// then
	require.True(t, ok)
	assert.Empty(t, ambiguous)
	assert.Equal(t, "dueDate", entry.Key)
}

// removedBundledPropertyId is where the removed-dueDate fixture rows live —
// the derived id (`drv-rel-` is the fixture stub's derivation function), so
// the tombstone leg exercises the same lookup production does.
const removedBundledPropertyId = "drv-rel-dueDate"

// addRemovedBundledProperty registers a REMOVED bundled relation (dueDate)
// in the given store shape; removal flavor for the full-detail shapes is
// isUninstalled (the UI delete). See addArchivedBundledProperty for the v2
// DELETE flavor.
func (fx *v2Fixture) addRemovedBundledProperty(t *testing.T, shape corpseShape) {
	if shape == corpseTombstone {
		fx.addTombstone(t, removedBundledPropertyId)
		return
	}
	obj := objectstore.TestObject{
		bundle.RelationKeyId:            domain.String(removedBundledPropertyId),
		bundle.RelationKeyRelationKey:   domain.String("dueDate"),
		bundle.RelationKeyName:          domain.String("Due date"),
		bundle.RelationKeyIsUninstalled: domain.Bool(true),
	}
	if shape == corpseProd {
		obj[bundle.RelationKeyIsDeleted] = domain.Bool(true)
	}
	fx.addRelation(t, testSpaceId, obj)
}

// requireRemovalRefusal asserts a 400 that names the removal AND its repair
// (§8.34: a refusal a caller cannot act on is itself a defect). slug is the
// served spelling the message must carry.
func requireRemovalRefusal(t *testing.T, err error, slug string) {
	t.Helper()
	apiErr := v2Err(t, err)
	assert.Equal(t, http.StatusBadRequest, apiErr.Status)
	require.NotEmpty(t, apiErr.Issues)
	assert.Contains(t, apiErr.Issues[0].Message, `"`+slug+`"`, "the refusal spells the served slug")
	assert.Contains(t, apiErr.Issues[0].Message, "removed from this space")
	assert.NotEmpty(t, apiErr.Issues[0].Hint, "the repair is named")
	assert.NotContains(t, apiErr.Issues[0].Hint, "restore it in the app",
		"the repair must be actionable for a headless caller (§8.41)")
}

// TestV2UninstalledBundledPropertyRefusesWrites (was
// TestV2UninstalledBundledPropertyWriteAsymmetry, which pinned the
// half-applied policy this fixes).
//
// For a BUNDLED relation the corpse policy used to half-apply: uninstalling
// dueDate removed it from listings and 404'd its routes, but the bundled
// vocabulary (resolution chain step 3, propertyKeyExistsIn's
// bundle.HasRelation arm) kept `due_date` a valid DOCUMENT key in every
// space — so a create landed new data in the property the user deleted, and
// the reinstall would light it back up. Now every write channel consults the
// bundled-removal probes and refuses with a repair.
//
// The distinction that makes this safe is NEVER-INSTALLED vs REMOVED: a
// bundled relation nobody installed has no relation object at all — not even
// a tombstone — is invisible to every removal probe, and keeps working
// exactly as before (it is the common case in a fresh space, and conflating
// the two would break ordinary writes everywhere).
//
// Fixture notes: the removed entity here is bundled, so it CANNOT be
// BSON-keyed with a stored slug — its key is `dueDate` by definition and its
// slug `due_date` is derived in code, never stored; that is precisely the
// class this refusal is about. dueDate is also deliberately a key whose slug
// DIFFERS from it — TestV2RemovedBundledSlugEqualsKeyClass covers the 41
// bundled relations where slug == key, the class that testing only dueDate
// hid for a full review round (§8.41-2). The BSON-keyed corpse with a stored
// slug rides along in the last subtest, which proves the refusal targets the
// bundled class only and leaves the §8.29 tolerance intact.
//
// All THREE store shapes run: flag-only would pass even with the isDeleted
// default unhandled (only the prod leg proves the removal set suppresses
// it), and both full-detail shapes would pass even if the probes keyed on
// fields a tombstone lacks (only the tombstone leg proves the derived-id
// fallback, §8.41-1).
// Revert checks (executed): dropping the removedPropertyIssue arm from
// validatePropertyKeys fails the create subtest; dropping it from stateops'
// checkKey fails the PATCH subtest; dropping the derived-id fallback in
// bundledPropertyRemoved fails BOTH tombstone legs while every full-detail
// leg stays green.
func TestV2UninstalledBundledPropertyRefusesWrites(t *testing.T) {
	ctx := context.Background()
	newFx := func(t *testing.T, shape corpseShape) *v2Fixture {
		fx := newV2Fixture(t)
		fx.addRemovedBundledProperty(t, shape)
		return fx
	}

	t.Run("the route side is corpse-aware: 404", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newFx(t, shape)
			_, err := fx.requireLiveProperty(testSpaceId, "due_date", errKeys{})
			requireNotFoundError(t, err)
		})
	})

	t.Run("create refuses to land a value on it", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newFx(t, shape)

			_, err := fx.CreateObject(ctx, testSpaceId,
				[]byte(`{"formatVersion":"2.0","type":"page","properties":{"name":"n","due_date":"2027-01-01"}}`), false, true)

			requireRemovalRefusal(t, err, "due_date")
			// §8.41-10 coherence: the envelope names what happened (the key
			// is KNOWN and removed — "unknown" was a lie the issue text then
			// contradicted), and the issue path spells the key as the CALLER
			// sent it, not as canonicalization rewrote it (/properties/dueDate
			// for a request that said due_date was unactionable)
			apiErr := v2Err(t, err)
			assert.Contains(t, apiErr.Message, "removed property keys")
			assert.NotContains(t, apiErr.Message, "unknown")
			assert.Equal(t, "/properties/due_date", apiErr.Issues[0].Path)
		})
	})

	t.Run("PATCH refuses it off-document, keeps the in-document cleanup", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			cleanDoc := `{"formatVersion":"2.0","id":"obj1","type":"page","properties":{"name":"Doc"},"blocks":[{"id":"blockOne1","type":"paragraph","text":"hi"}]}`
			holdingDoc := `{"formatVersion":"2.0","id":"obj1","type":"page","properties":{"name":"Doc","dueDate":"2027-01-01"},"blocks":[{"id":"blockOne1","type":"paragraph","text":"hi"}]}`

			t.Run("off-document set: refused", func(t *testing.T) {
				fx := newFx(t, shape)
				fx.expectMutate(editRead(t, cleanDoc), "headB")

				_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
					patchBody(`{"op":"set_properties","set":{"due_date":"2030-12-31"}}`), "", false, true)

				requireRemovalRefusal(t, err, "due_date")
			})

			t.Run("unset of a value the document carries: still the cleanup channel", func(t *testing.T) {
				fx := newFx(t, shape)
				captured := fx.expectMutate(editRead(t, holdingDoc), "headB")

				_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
					patchBody(`{"op":"set_properties","unset":["due_date"]}`), "", false, true)

				require.NoError(t, err)
				_, present := (*captured).CombinedDetails().TryString(bundle.RelationKeyDueDate)
				assert.False(t, present, "removing a removed property's leftover value must stay possible")
			})
		})
	})

	t.Run("a view cannot gain a column for it either", func(t *testing.T) {
		// §8.40 claimed this channel needed no removal check because "the
		// bundled slug stops resolving" — true ONLY for the slug≠key class
		// this fixture happens to be in; the slug==key class sailed through
		// (§8.41-2, see TestV2RemovedBundledSlugEqualsKeyClass). The channel
		// now runs the explicit removal gate, and the refusal says REMOVED
		// with the repair, not "unknown key" with a did-you-mean.
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newFx(t, shape)
			plainViewDoc := `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
				`{"id":"dataview","type":"dataview","properties":[{"property":"name","format":"text"}],` +
				`"views":[{"id":"viewAll1","name":"All","columns":[{"property":"name"}]}]}]}`
			fx.expectMutate(editRead(t, plainViewDoc), "headB")

			_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
				patchBody(`{"op":"update_view","view":"viewAll1","columns":{"due_date":{"width":80}}}`), "", false, true)

			requireRemovalRefusal(t, err, "due_date")
		})
	})

	t.Run("a NEVER-installed bundled property still works", func(t *testing.T) {
		// the whole distinction: no relation object exists for dueDate here —
		// not even a tombstone at the derived id — so install-on-write is
		// untouched, the common case in a fresh space. Both slug classes: a
		// slug≠key relation (dueDate) and a slug==key one (description),
		// because the removal probes consult per-class code paths (§8.41-2).
		fx := newV2Fixture(t)
		captured := fx.expectCreate("obj-due")
		fx.expectEtagRead("obj-due")

		_, err := fx.CreateObject(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"page","properties":{"name":"n","due_date":"2027-01-01","description":"d"}}`), false, true)

		require.NoError(t, err)
		require.NotNil(t, *captured)
		_, landed := (*captured).Details.Fields["dueDate"]
		assert.True(t, landed, "a bundled property nobody removed installs on write")
		_, landedSame := (*captured).Details.Fields["description"]
		assert.True(t, landedSame, "the slug==key class installs on write too")
	})

	t.Run("an ARCHIVED bundled property refuses writes the same way", func(t *testing.T) {
		// §8.40 pinned archived-accepts as an open question so that widening
		// the removal set "cannot pass silently". §8.41 answers it: the API's
		// OWN delete verb (DELETE /properties → ObjectSetIsArchived) creates
		// this state, and a property whose route 404s must not keep accepting
		// writes — the incoherence the whole corpse round exists to remove.
		// This is the conscious edit that flips the old pin.
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:          domain.String(removedBundledPropertyId),
			bundle.RelationKeyRelationKey: domain.String("dueDate"),
			bundle.RelationKeyName:        domain.String("Due date"),
			bundle.RelationKeyIsArchived:  domain.Bool(true),
		})

		_, err := fx.CreateObject(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"page","properties":{"name":"n","due_date":"2027-01-01"}}`), false, true)

		requireRemovalRefusal(t, err, "due_date")
	})

	t.Run("the BSON-keyed custom corpse stays tolerated", func(t *testing.T) {
		// the refusal is bundled-only: a custom corpse's stored key can never
		// be reinstalled, so §8.29's clone tolerance still carries its value
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newFx(t, shape)
			fx.addCorpseProperty(t, shape)
			captured := fx.expectCreate("obj-both")
			fx.expectEtagRead("obj-both")

			_, err := fx.CreateObject(ctx, testSpaceId,
				[]byte(`{"formatVersion":"2.0","type":"page","properties":{"name":"n","`+corpseBsonKey+`":"x"}}`), false, true)

			require.NoError(t, err)
			assert.Equal(t, "x", (*captured).Details.Fields[corpseBsonKey].GetStringValue())
		})
	})
}

// TestV2CorpseSlugReAimsAfterRecreate: the executed consequence of the
// vacate lean (§8-OQ2) that round-trip testing cannot see. Property P held
// slug S and was uninstalled; a NEW property Q minted S (the namespace was
// free — delete-then-recreate is the (a) strategy's headline win). A
// document exported while P was LIVE spells S; sent back now, S
// canonicalizes onto Q's stored key: same bytes, different property. This is
// inherent to vacate-and-remint and pinned here as CURRENT, DOCUMENTED
// behavior — and it is the strongest argument against the proposed
// read-emit half: if reads emitted a corpse's slug, every post-uninstall
// export would join this re-aim class instead of pinning the stored key.
// Revert check: dropping the isUninstalled filter in livePropertyFilters
// makes S ambiguous (corpse + Q) on the flag-only leg and the
// canonicalization errors instead (the prod leg stays green there — the
// store's injected isDeleted default excludes prod corpses on its own, which
// is exactly why both shapes are run).
func TestV2CorpseSlugReAimsAfterRecreate(t *testing.T) {
	corpseShapes(t, func(t *testing.T, shape corpseShape) {
		// given
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, shape)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-recreated"),
			bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e202"),
			bundle.RelationKeyApiObjectKey: domain.String(corpseSlug),
			bundle.RelationKeyName:         domain.String("Warranty until"),
		})

		// when — a document exported before the uninstall, naming the slug
		body, _, err := fx.canonicalizeDocumentKeys(testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"page","properties":{"`+corpseSlug+`":"x"}}`))

		// then — the value now binds the RECREATED property's stored key
		require.NoError(t, err)
		var doc struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(body, &doc))
		assert.Contains(t, doc.Properties, "6a7663db61fab21cd4b9e202")
		assert.NotContains(t, doc.Properties, corpseSlug)
	})
}

// TestV2TypePropertiesCorpseEchoResolvesToItsHolder (was
// TestV2TypePropertiesCorpseEchoMintsDuplicate, which pinned the gap).
//
// GET /types/{key} serves typeProperties resolved BY ID (storeresolver's
// GetRelationById falls back to an unfiltered point lookup), so a corpse in
// recommendedRelations is served under its stored BSON key with its name —
// see the first subtest, and that read is DELIBERATELY unchanged: the type
// document mirrors the list the type actually stores, so dropping corpse
// entries would make the documented read-modify-write loop silently DELETE
// the type's reference to them (typeProperties is a whole-list replace).
//
// PATCHing that served list back used to walk creatingResolvers.PropertyId,
// which excludes corpses by design, and mint a brand-new property
// duplicating the corpse's name under a snake-cased-hex slug — once per
// PATCH, forever. Now PropertyId consults relationObjectHoldingKey after the
// live chain misses: a stored key held by a relation object resolves to that
// relation and never mints. The round trip is an identity.
//
// How this fixture can fail: the corpse is BSON-keyed with a stored slug, so
// a resolver that started answering the SLUG here (the vacated namespace)
// would keep the mint assertion green but change the resolved id, which the
// recommendedRelations assertion catches; a readable corpse key could not
// tell a mint from a resolve at all. The TOMBSTONE legs are the §8.41
// additions: the GET leg fails if the read stops recovering the entry from
// the surviving tree (it silently vanishes and the loop deletes the
// reference), and the PATCH leg fails if the holder probe loses its
// derived-id fallback (the 6a7663… duplicate mints again — in that window
// only, which is why no two-shape fixture ever saw it).
// Revert checks (executed): removing the relationObjectHoldingKey arm from
// PropertyId fails the PATCH subtest on every shape — a mint reappears;
// removing only the derived-id fallback inside relationObjectHoldingKey
// fails the PATCH subtest's tombstone leg alone; removing the
// seedTombstonedTypeProperties call in GetObject fails the GET subtest's
// tombstone leg alone.
func TestV2TypePropertiesCorpseEchoResolvesToItsHolder(t *testing.T) {
	newFx := func(t *testing.T, shape corpseShape) *v2Fixture {
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, shape)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String("type-live"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-livetype"),
			bundle.RelationKeyName:                 domain.String("Live type"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{corpsePropertyId}),
		})
		if shape == corpseTombstone {
			// the read half recovers a tombstoned entry from the LIVE object —
			// the tree survives a UI delete (ADDRESSING §2.4-5); the index row
			// alone spells nothing
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, corpsePropertyId).Return(apicore.ObjectRead{
				SbType: model.SmartBlockType_STRelation,
				Snapshot: &model.SmartBlockSnapshotBase{
					Details: &types.Struct{Fields: map[string]*types.Value{
						"id":             pbtypes.String(corpsePropertyId),
						"relationKey":    pbtypes.String(corpseBsonKey),
						"apiObjectKey":   pbtypes.String(corpseSlug),
						"name":           pbtypes.String("Warranty until"),
						"relationFormat": pbtypes.Int64(int64(model.RelationFormat_longtext)),
						"isUninstalled":  pbtypes.Bool(true),
					}},
				},
				Heads: []string{"headR"},
			}, nil).Maybe()
		}
		return fx
	}
	liveTypeRead := func() apicore.ObjectRead {
		return apicore.ObjectRead{
			SbType: model.SmartBlockType_STType,
			Snapshot: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					"id":                   pbtypes.String("type-live"),
					"uniqueKey":            pbtypes.String("ot-livetype"),
					"name":                 pbtypes.String("Live type"),
					"recommendedRelations": pbtypes.StringList([]string{corpsePropertyId}),
				}},
				ObjectTypes: []string{"ot-objectType"},
			},
			Heads: []string{"headA"},
		}
	}

	t.Run("GET type serves the corpse row in typeProperties", func(t *testing.T) {
		// the by-id fallback escapes even the injected isDeleted default
		// (GetRelationById reads unfiltered details), and that is the
		// behaviour the write half is built around — see the header
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			// given
			fx := newFx(t, shape)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "type-live").Return(liveTypeRead(), nil)

			// when
			body, _, err := fx.GetType(context.Background(), testSpaceId, "livetype", ObjectQuery{})

			// then — served under the stored BSON key, with the corpse's name
			require.NoError(t, err)
			var doc struct {
				TypeSettings struct {
					PropertyDefinitions []struct {
						Property    string `json:"property"`
						InternalKey string `json:"internal_key"`
						Name        string `json:"name"`
					} `json:"property_definitions"`
				} `json:"type_settings"`
			}
			require.NoError(t, json.Unmarshal(body, &doc))
			defs := doc.TypeSettings.PropertyDefinitions
			require.Len(t, defs, 1)
			// §2e: the entry names the property by its document-facing
			// spelling, and carries the stored key beside it — a BSON-keyed
			// corpse has no slug, so both land on the stored key
			assert.Equal(t, corpseBsonKey, defs[0].Property)
			assert.Equal(t, "Warranty until", defs[0].Name)
		})
	})

	t.Run("PATCHing that list back resolves to the holder — no mint", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			// given
			fx := newFx(t, shape)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "type-live").Return(liveTypeRead(), nil).Maybe()
			var minted []*pb.RpcObjectCreateRelationRequest
			fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).RunAndReturn(
				func(ctx context.Context, req *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse {
					minted = append(minted, req)
					return &pb.RpcObjectCreateRelationResponse{
						Error:    &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
						ObjectId: "rel-minted-dup", Key: "6a7663db61fab21cd4b9e999",
					}
				}).Maybe()
			var applied []*model.Detail
			fx.mwMock.EXPECT().ObjectSetDetails(mock.Anything, mock.Anything).RunAndReturn(
				func(ctx context.Context, req *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse {
					applied = req.Details
					return &pb.RpcObjectSetDetailsResponse{
						Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL}}
				})

			// when — exactly the typeProperties GET just served
			result, err := fx.UpdateType(context.Background(), testSpaceId, "livetype",
				[]byte(`{"type_settings":{"property_definitions":[{"property":"`+corpseBsonKey+`","name":"Warranty until","format":"text"}]}}`), false, true)

			// then — nothing is minted and the list still points at the very
			// relation object the GET resolved it from: a round-trip identity
			require.NoError(t, err)
			assert.Empty(t, minted, "an echoed stored key is never a mint request")
			assert.Nil(t, result.Created, "no property side effect is reported either")
			var recommended []string
			for _, detail := range applied {
				if detail.Key == bundle.RelationKeyRecommendedRelations.String() {
					recommended = pbtypes.GetStringListValue(detail.Value)
				}
			}
			assert.Equal(t, []string{corpsePropertyId}, recommended)
		})
	})

	t.Run("the corpse SLUG in typeProperties still mints — the namespace vacated", func(t *testing.T) {
		// the resolve is KEY-ONLY: a corpse's slug is free for re-minting
		// (§8-OQ2), so naming it declares a NEW property, never the corpse
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newFx(t, shape)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "type-live").Return(liveTypeRead(), nil).Maybe()
			var minted []*pb.RpcObjectCreateRelationRequest
			fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).RunAndReturn(
				func(ctx context.Context, req *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse {
					minted = append(minted, req)
					return &pb.RpcObjectCreateRelationResponse{
						Error:    &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
						ObjectId: "rel-fresh", Key: "6a7663db61fab21cd4b9e777",
					}
				})
			fx.mwMock.EXPECT().ObjectSetDetails(mock.Anything, mock.Anything).Return(&pb.RpcObjectSetDetailsResponse{
				Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL}}).Maybe()

			_, err := fx.UpdateType(context.Background(), testSpaceId, "livetype",
				[]byte(`{"type_settings":{"property_definitions":[{"property":"`+corpseSlug+`","name":"Warranty until","format":"text"}]}}`), false, true)

			require.NoError(t, err)
			require.Len(t, minted, 1)
			assert.Equal(t, corpseSlug,
				minted[0].Details.Fields[bundle.RelationKeyApiObjectKey.String()].GetStringValue())
		})
	})
}

// TestV2RemovedBundledSlugEqualsKeyClass covers the bundled-key class the
// review round could not see: 41 of 194 bundled relations have
// ApiSlug(key) == key (`tag`, `status`, `description`, …), and every §8.40
// verification used dueDate — one of the keys where they DIFFER. The
// difference is load-bearing on the view channel: view documents spell
// slugs, so a slug≠key removal was refused there by accident (the slug
// stopped resolving) while the slug==key class landed writes on removed
// properties across columns, groupBy, filters and sorts — executed at 40/40
// accepted before this fix (§8.41-2).
//
// How these fixtures can fail: the removed relations are `description`
// (create/PATCH legs) and `tag` (view legs) — both slug==key, so any gate
// that only works when canonicalization changes the spelling (the §8.40
// assumption) passes dueDate's test and fails these. Three shapes each; the
// tombstone leg fails if the removal verdict loses its derived-id fallback.
// Revert checks (executed): dropping the refusesRemovedBundled arm from
// viewops' validateViewKeys fails every view leg here while the dueDate
// column test stays green (its refusal comes from the slug fallback);
// dropping the removedPropertyIssue arm from stateops' checkKey fails the
// PATCH leg.
func TestV2RemovedBundledSlugEqualsKeyClass(t *testing.T) {
	ctx := context.Background()
	addRemoved := func(t *testing.T, fx *v2Fixture, key string, shape corpseShape) {
		if shape == corpseTombstone {
			fx.addTombstone(t, "drv-rel-"+key)
			return
		}
		obj := objectstore.TestObject{
			bundle.RelationKeyId:            domain.String("drv-rel-" + key),
			bundle.RelationKeyRelationKey:   domain.String(key),
			bundle.RelationKeyName:          domain.String(key),
			bundle.RelationKeyIsUninstalled: domain.Bool(true),
		}
		if key == "tag" {
			obj[bundle.RelationKeyRelationFormat] = domain.Int64(int64(model.RelationFormat_tag))
		}
		if shape == corpseProd {
			obj[bundle.RelationKeyIsDeleted] = domain.Bool(true)
		}
		fx.addRelation(t, testSpaceId, obj)
	}

	t.Run("create refuses a removed slug==key property", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newV2Fixture(t)
			addRemoved(t, fx, "description", shape)

			_, err := fx.CreateObject(ctx, testSpaceId,
				[]byte(`{"formatVersion":"2.0","type":"page","properties":{"name":"n","description":"x"}}`), false, true)

			requireRemovalRefusal(t, err, "description")
		})
	})

	t.Run("PATCH refuses it off-document", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newV2Fixture(t)
			addRemoved(t, fx, "description", shape)
			cleanDoc := `{"formatVersion":"2.0","id":"obj1","type":"page","properties":{"name":"Doc"},"blocks":[{"id":"blockOne1","type":"paragraph","text":"hi"}]}`
			fx.expectMutate(editRead(t, cleanDoc), "headB")

			_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
				patchBody(`{"op":"set_properties","set":{"description":"x"}}`), "", false, true)

			requireRemovalRefusal(t, err, "description")
		})
	})

	t.Run("update_view refuses it on every channel", func(t *testing.T) {
		// the executed §8.41-2 matrix: columns, groupBy, filters, sorts —
		// the four channels that accepted a removed `tag` 40 times out of 40
		plainViewDoc := `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
			`{"id":"dataview","type":"dataview","properties":[{"property":"name","format":"text"}],` +
			`"views":[{"id":"viewAll1","name":"All","columns":[{"property":"name"}]}]}]}`
		ops := map[string]string{
			"columns":  `{"op":"update_view","view":"viewAll1","columns":{"tag":{"width":80}}}`,
			"group_by": `{"op":"update_view","view":"viewAll1","set":{"group_by":"tag"}}`,
			"filters":  `{"op":"update_view","view":"viewAll1","set":{"filters":[{"property":"tag","condition":"empty"}]}}`,
			"sorts":    `{"op":"update_view","view":"viewAll1","set":{"sorts":[{"property":"tag","direction":"asc"}]}}`,
		}
		for channel, op := range ops {
			t.Run(channel, func(t *testing.T) {
				corpseShapes(t, func(t *testing.T, shape corpseShape) {
					fx := newV2Fixture(t)
					addRemoved(t, fx, "tag", shape)
					fx.expectMutate(editRead(t, plainViewDoc), "headB")

					_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(op), "", false, true)

					requireRemovalRefusal(t, err, "tag")
				})
			})
		}
	})

	t.Run("insert_view runs the same gate", func(t *testing.T) {
		// insert_view shares validateViewKeys with update_view — pinned so a
		// future split of the two paths cannot reopen one of them
		plainViewDoc := `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
			`{"id":"dataview","type":"dataview","properties":[{"property":"name","format":"text"}],` +
			`"views":[{"id":"viewAll1","name":"All","columns":[{"property":"name"}]}]}]}`
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newV2Fixture(t)
			addRemoved(t, fx, "tag", shape)
			fx.expectMutate(editRead(t, plainViewDoc), "headB")

			_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
				patchBody(`{"op":"insert_view","name":"Grouped","set":{"group_by":"tag"}}`), "", false, true)

			requireRemovalRefusal(t, err, "tag")
		})
	})

	t.Run("a view already showing the removed key stays editable (§8.17)", func(t *testing.T) {
		// the preKnown escape: an edit must not reject what the surface
		// already shows — the in-document twin of the PATCH escape
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newV2Fixture(t)
			addRemoved(t, fx, "tag", shape)
			holdingViewDoc := `{"formatVersion":"2.0","id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
				`{"id":"dataview","type":"dataview","properties":[{"property":"name","format":"text"},{"property":"tag","format":"multi_select"}],` +
				`"views":[{"id":"viewAll1","name":"All","columns":[{"property":"name"},{"property":"tag"}]}]}]}`
			fx.expectMutate(editRead(t, holdingViewDoc), "headB")

			_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
				patchBody(`{"op":"update_view","view":"viewAll1","set":{"group_by":"tag"}}`), "", false, true)

			require.NoError(t, err)
		})
	})
}

// TestV2RemovedBundledTypeRefusesWrites: the TYPE namespace has the same
// hole the property namespace had, previously untouched (§8.41-5): a
// bundled type key resolves through the bundled table forever, so with
// `task` uninstalled, GET /types/task 404'd while POST /objects
// {"type":"task"} created an object IN the removed type — and a reinstall
// lit the type back up with the new object already in it. Both bundled key
// classes run (`task`: slug==key; `diaryEntry`: slug `diary_entry` differs)
// and all three store shapes; rows sit at the fixture-derived ids
// (`drv-ot-<key>`) so the tombstone leg exercises the derived-id probe.
// Revert checks (executed): dropping the refuseRemovedType call from
// validateDocumentRefs fails the create and templateFor legs; dropping the
// tombstone arm from bundledTypeRemoved fails only the tombstone legs.
func TestV2RemovedBundledTypeRefusesWrites(t *testing.T) {
	ctx := context.Background()
	addRemovedType := func(t *testing.T, fx *v2Fixture, key string, shape corpseShape) {
		if shape == corpseTombstone {
			fx.addTombstone(t, "drv-ot-"+key)
			return
		}
		obj := objectstore.TestObject{
			bundle.RelationKeyId:            domain.String("drv-ot-" + key),
			bundle.RelationKeyUniqueKey:     domain.String("ot-" + key),
			bundle.RelationKeyName:          domain.String(key),
			bundle.RelationKeyIsUninstalled: domain.Bool(true),
		}
		if shape == corpseProd {
			obj[bundle.RelationKeyIsDeleted] = domain.Bool(true)
		}
		fx.addType(t, testSpaceId, obj)
	}
	requireRemovedType := func(t *testing.T, err error, slug string) {
		t.Helper()
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, `"`+slug+`"`, "the refusal spells the served slug")
		assert.Contains(t, apiErr.Issues[0].Message, "removed from this space")
		assert.NotEmpty(t, apiErr.Issues[0].Hint)
	}

	t.Run("POST objects refuses both spellings of a removed type", func(t *testing.T) {
		for _, spelling := range []struct{ key, input, slug string }{
			{"task", "task", "task"},
			{"diaryEntry", "diary_entry", "diary_entry"},
			{"diaryEntry", "diaryEntry", "diary_entry"},
		} {
			t.Run(spelling.input, func(t *testing.T) {
				corpseShapes(t, func(t *testing.T, shape corpseShape) {
					fx := newV2Fixture(t)
					addRemovedType(t, fx, spelling.key, shape)

					_, err := fx.CreateObject(ctx, testSpaceId,
						[]byte(`{"formatVersion":"2.0","type":"`+spelling.input+`","properties":{"name":"n"}}`), false, true)

					requireRemovedType(t, err, spelling.slug)

					// and the route side stays coherent: the type 404s
					_, _, err = fx.GetType(ctx, testSpaceId, spelling.input, ObjectQuery{})
					requireNotFoundError(t, err)
				})
			})
		}
	})

	t.Run("templateFor refuses a removed target type", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newV2Fixture(t)
			addRemovedType(t, fx, "task", shape)

			_, err := fx.CreateTemplate(ctx, testSpaceId,
				[]byte(`{"formatVersion":"2.0","type":"template","template_for":"task","properties":{"name":"Weekly"}}`), false, true)

			requireRemovedType(t, err, "task")
		})
	})

	t.Run("POST queries names the removal instead of unknown", func(t *testing.T) {
		// a query over a bundled-but-uninstalled type was ALREADY refused (a
		// query needs an installed type object), but as "unknown type key" with
		// a did-you-mean — a lie about a key the space knows and removed
		// (§8.41-10)
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newV2Fixture(t)
			addRemovedType(t, fx, "task", shape)

			_, err := fx.CreateQuery(ctx, testSpaceId, v2model.CreateQueryRequest{Name: "Tasks", Type: "task"}, false, true)

			requireRemovedType(t, err, "task")
		})
	})

	t.Run("a NEVER-installed bundled type still creates", func(t *testing.T) {
		// install-on-write is the common case in a fresh space — no type
		// object, not even a tombstone, exists for task here
		fx := newV2Fixture(t)
		captured := fx.expectCreate("obj-task")
		fx.expectEtagRead("obj-task")

		_, err := fx.CreateObject(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"task","properties":{"name":"n"}}`), false, true)

		require.NoError(t, err)
		require.NotNil(t, *captured)
		assert.Equal(t, []string{"ot-task"}, (*captured).ObjectTypes)
	})

	t.Run("an ARCHIVED bundled type refuses too", func(t *testing.T) {
		// DELETE /types archives; the API's own delete verb must not leave a
		// type that 404s on its route yet accepts new objects (§8.41-8)
		fx := newV2Fixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:         domain.String("drv-ot-task"),
			bundle.RelationKeyUniqueKey:  domain.String("ot-task"),
			bundle.RelationKeyName:       domain.String("Task"),
			bundle.RelationKeyIsArchived: domain.Bool(true),
		})

		_, err := fx.CreateObject(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"task","properties":{"name":"n"}}`), false, true)

		requireRemovedType(t, err, "task")
	})
}

// TestV2TypePropertiesRefusesRemovedBundledKey: the typeProperties channel
// was the one write channel the §8.40 refusal never reached (§8.41-4) —
// POST/PATCH /types with {"key":"due_date"} while dueDate was removed
// pointed the new type's recommendedRelations at the corpse, `created:
// null`, silently. (Before the §8.40 round this path was worse still: it
// REINSTALLED the deleted relation. That resurrection stays closed — the
// mocks here fail the test on any unexpected install or mint RPC.)
//
// The ONE escape is the type's own echo (echoPropertyIds): a type already
// referencing the removed relation's object gets its GET/PATCH loop back as
// an identity — refusing the echo would force-delete the reference, the
// §8.34 outcome the custom-corpse decision (§8.40-2) exists to prevent.
// Both bundled key classes run (due_date: slug≠key; tag: slug==key), three
// shapes each.
// Revert checks (executed): dropping the removal gate in PropertyId's
// bundled arm fails the two refusal subtests (the reference lands again);
// dropping the echoPropertyIds escape fails the echo subtest (it turns into
// a refusal).
func TestV2TypePropertiesRefusesRemovedBundledKey(t *testing.T) {
	ctx := context.Background()

	t.Run("POST types refuses a removed bundled key in typeProperties", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newV2Fixture(t)
			fx.addRemovedBundledProperty(t, shape)

			_, err := fx.CreateType(ctx, testSpaceId,
				[]byte(`{"properties":{"name":"Gadget"},"type_settings":{"api_key":"gadget","property_definitions":[{"property":"due_date","format":"date"}]}}`), false, true)

			apiErr := v2Err(t, err)
			assert.Equal(t, http.StatusBadRequest, apiErr.Status)
			require.NotEmpty(t, apiErr.Issues)
			assert.Contains(t, apiErr.Issues[0].Message, `"due_date"`)
			assert.Contains(t, apiErr.Issues[0].Message, "removed from this space")
		})
	})

	t.Run("PATCH types refuses adding a removed slug==key property", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newV2Fixture(t)
			if shape == corpseTombstone {
				fx.addTombstone(t, "drv-rel-tag")
			} else {
				obj := objectstore.TestObject{
					bundle.RelationKeyId:             domain.String("drv-rel-tag"),
					bundle.RelationKeyRelationKey:    domain.String("tag"),
					bundle.RelationKeyName:           domain.String("Tag"),
					bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
					bundle.RelationKeyIsUninstalled:  domain.Bool(true),
				}
				if shape == corpseProd {
					obj[bundle.RelationKeyIsDeleted] = domain.Bool(true)
				}
				fx.addRelation(t, testSpaceId, obj)
			}
			// the PATCHed type does NOT reference the tag corpse — no echo
			fx.addType(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:        domain.String("type-live"),
				bundle.RelationKeyUniqueKey: domain.String("ot-livetype"),
				bundle.RelationKeyName:      domain.String("Live type"),
			})

			_, err := fx.UpdateType(ctx, testSpaceId, "livetype",
				[]byte(`{"type_settings":{"property_definitions":[{"property":"tag"}]}}`), false, true)

			apiErr := v2Err(t, err)
			assert.Equal(t, http.StatusBadRequest, apiErr.Status)
			require.NotEmpty(t, apiErr.Issues)
			assert.Contains(t, apiErr.Issues[0].Message, `"tag"`)
			assert.Contains(t, apiErr.Issues[0].Message, "removed from this space")
		})
	})

	t.Run("the type's own echo resolves as an identity", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			// given — the type already references the removed relation
			fx := newV2Fixture(t)
			fx.addRemovedBundledProperty(t, shape)
			fx.addType(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:                   domain.String("type-live"),
				bundle.RelationKeyUniqueKey:            domain.String("ot-livetype"),
				bundle.RelationKeyName:                 domain.String("Live type"),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{removedBundledPropertyId}),
			})
			var applied []*model.Detail
			fx.mwMock.EXPECT().ObjectSetDetails(mock.Anything, mock.Anything).RunAndReturn(
				func(ctx context.Context, req *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse {
					applied = req.Details
					return &pb.RpcObjectSetDetailsResponse{
						Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL}}
				})
			fx.expectEtagRead("type-live")

			// when — the spelling the GET serves
			result, err := fx.UpdateType(ctx, testSpaceId, "livetype",
				[]byte(`{"type_settings":{"property_definitions":[{"property":"due_date","format":"date"}]}}`), false, true)

			// then — the reference survives, nothing minted, nothing installed
			require.NoError(t, err)
			assert.Nil(t, result.Created)
			var recommended []string
			for _, detail := range applied {
				if detail.Key == bundle.RelationKeyRecommendedRelations.String() {
					recommended = pbtypes.GetStringListValue(detail.Value)
				}
			}
			assert.Equal(t, []string{removedBundledPropertyId}, recommended)
		})
	})
}

// TestV2QueriesRefuseRemovedBundledProperty: POST /queries validated
// filter/sort keys against the queried type's recommended lists — resolved BY
// ID and never stripped of deleted relations, which is the DEFAULT state
// after any UI delete — so a new query could persist a filter on a removed property
// (§8.41-6). Both bundled key classes; three shapes. On the tombstone leg
// the recommended-list resolution itself cannot spell the key (the row has
// none), so the key falls out of the type's reference set and the refusal
// comes from the has-no-property branch — a 400 either way, pinned as such.
// Revert check (executed): dropping the removal gate in list_create's
// validateViewKeys turns the flag-only and prod legs green-through (the
// query is created) and both fail.
func TestV2QueriesRefuseRemovedBundledProperty(t *testing.T) {
	ctx := context.Background()
	newFx := func(t *testing.T, key string, format model.RelationFormat, shape corpseShape) *v2Fixture {
		fx := newV2Fixture(t)
		if shape == corpseTombstone {
			fx.addTombstone(t, "drv-rel-"+key)
		} else {
			obj := objectstore.TestObject{
				bundle.RelationKeyId:             domain.String("drv-rel-" + key),
				bundle.RelationKeyRelationKey:    domain.String(key),
				bundle.RelationKeyName:           domain.String(key),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(format)),
				bundle.RelationKeyIsUninstalled:  domain.Bool(true),
			}
			if shape == corpseProd {
				obj[bundle.RelationKeyIsDeleted] = domain.Bool(true)
			}
			fx.addRelation(t, testSpaceId, obj)
		}
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String("type-bug"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-bug"),
			bundle.RelationKeyName:                 domain.String("Bug"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"drv-rel-" + key}),
		})
		return fx
	}

	t.Run("filters on a removed slug≠key property", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newFx(t, "dueDate", model.RelationFormat_date, shape)

			_, err := fx.CreateQuery(ctx, testSpaceId, v2model.CreateQueryRequest{
				Name: "Late bugs", Type: "bug",
				Filters: json.RawMessage(`[{"property":"due_date","condition":"empty"}]`),
			}, false, true)

			apiErr := v2Err(t, err)
			assert.Equal(t, http.StatusBadRequest, apiErr.Status)
			if shape != corpseTombstone {
				require.NotEmpty(t, apiErr.Issues)
				assert.Contains(t, apiErr.Issues[0].Message, "removed from this space")
			}
		})
	})

	t.Run("sorts on a removed slug==key property", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := newFx(t, "tag", model.RelationFormat_tag, shape)

			_, err := fx.CreateQuery(ctx, testSpaceId, v2model.CreateQueryRequest{
				Name: "By tag", Type: "bug",
				Sorts: json.RawMessage(`[{"property":"tag","direction":"asc"}]`),
			}, false, true)

			apiErr := v2Err(t, err)
			assert.Equal(t, http.StatusBadRequest, apiErr.Status)
			if shape != corpseTombstone {
				require.NotEmpty(t, apiErr.Issues)
				assert.Contains(t, apiErr.Issues[0].Message, "removed from this space")
			}
		})
	})
}

// TestV2HoldingKeyPrefersLive pins the §8.41-11 selection rule: with a
// corpse AND a live relation both holding one stored key, the probe answers
// the LIVE one. The old Limit:1 query with no sort returned whichever row
// the store yielded first; callers happened to consult the probe only after
// the live chain missed, but that ordering was a convention, not a contract.
// How this fixture can fail: the corpse row's id ("a-…") sorts BEFORE the
// live row's ("z-…"), so both a first-row regression and an id-ordered
// tie-break without the liveness preference return the corpse.
func TestV2HoldingKeyPrefersLive(t *testing.T) {
	fx := newV2Fixture(t)
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:            domain.String("a-corpse"),
		bundle.RelationKeyRelationKey:   domain.String("sharedKey"),
		bundle.RelationKeyName:          domain.String("Corpse holder"),
		bundle.RelationKeyIsUninstalled: domain.Bool(true),
	})
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:          domain.String("z-live"),
		bundle.RelationKeyRelationKey: domain.String("sharedKey"),
		bundle.RelationKeyName:        domain.String("Live holder"),
	})

	id, held, err := fx.relationObjectHoldingKey(context.Background(), testSpaceId, "sharedKey")

	require.NoError(t, err)
	require.True(t, held)
	assert.Equal(t, "z-live", id, "a live holder outranks a corpse regardless of row order")
}
