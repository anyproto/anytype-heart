package v2service

// Corpse (uninstalled) property/type ADDRESSABILITY — what each surface does
// with an entity the UI deleted while its values still sit on objects.
//
// The store shape matters and the fixtures here model BOTH:
//
//   - "flag-only": {isUninstalled:true} — the shape every pre-existing corpse
//     test in this package uses. It exercises the explicit isUninstalled
//     filters (the §7.5-req-2 corpse policy) in isolation.
//   - "prod": {isUninstalled:true, isDeleted:true} — what a real UI delete
//     persists. delete.go's deleteDerivedObject sets isUninstalled and the
//     next Apply injects isDeleted=true (smartblock/detailsinject.go, since
//     GO-1978); BeforeDelete then tombstones the index row, and the next
//     space load resurrects it with full details plus BOTH flags (the corpse
//     tree still exists, its headsState row was deleted, so
//     reindexOutdatedObjects re-indexes it). Every plain store query injects
//     `isDeleted != true` (database.go addDefaultFilters), so a prod corpse
//     is hidden from queries even where nothing filters isUninstalled.
//
// A flag-only fixture therefore CANNOT catch behavior that depends on the
// injected isDeleted default — see TestV2CloneToleranceSurvivesTheProdShape,
// where the two shapes gave opposite answers on an advertised loop until the
// probe behind it suppressed both defaults (§8.40).
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

// corpseShapes runs a subtest against both store shapes a corpse can have.
func corpseShapes(t *testing.T, run func(t *testing.T, prodShape bool)) {
	t.Run("flag-only shape", func(t *testing.T) { run(t, false) })
	t.Run("prod shape", func(t *testing.T) { run(t, true) })
}

// addCorpseProperty registers the BSON-keyed, slug-bearing corpse relation.
func (fx *v2Fixture) addCorpseProperty(t *testing.T, prodShape bool) {
	obj := objectstore.TestObject{
		bundle.RelationKeyId:            domain.String("rel-corpse-bson"),
		bundle.RelationKeyRelationKey:   domain.String(corpseBsonKey),
		bundle.RelationKeyApiObjectKey:  domain.String(corpseSlug),
		bundle.RelationKeyName:          domain.String("Warranty until"),
		bundle.RelationKeyIsUninstalled: domain.Bool(true),
	}
	if prodShape {
		obj[bundle.RelationKeyIsDeleted] = domain.Bool(true)
	}
	fx.addRelation(t, testSpaceId, obj)
}

func (fx *v2Fixture) addCorpseType(t *testing.T, prodShape bool) {
	obj := objectstore.TestObject{
		bundle.RelationKeyId:            domain.String("type-corpse-bson"),
		bundle.RelationKeyUniqueKey:     domain.String("ot-" + corpseTypeBsonKey),
		bundle.RelationKeyApiObjectKey:  domain.String(corpseTypeSlug),
		bundle.RelationKeyName:          domain.String("Old meeting"),
		bundle.RelationKeyIsUninstalled: domain.Bool(true),
	}
	if prodShape {
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
	corpseShapes(t, func(t *testing.T, prodShape bool) {
		// given
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, prodShape)
		fx.addCorpseType(t, prodShape)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(corpseHeldRead(), nil)

		// when
		body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{})

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
	corpseShapes(t, func(t *testing.T, prodShape bool) {
		// given
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, prodShape)
		fx.addCorpseType(t, prodShape)

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
			_, err := fx.requireLiveProperty(testSpaceId, input)
			requireNotFoundError(t, err)
		}
		for _, input := range []string{corpseTypeBsonKey, corpseTypeSlug} {
			_, _, err := fx.GetType(context.Background(), testSpaceId, input, V2ObjectQuery{})
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
	newFx := func(t *testing.T, prodShape bool) *v2Fixture {
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, prodShape)
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

	corpseShapes(t, func(t *testing.T, prodShape bool) {
		t.Run("structured filter, both spellings", func(t *testing.T) {
			fx := newFx(t, prodShape)
			for _, key := range []string{corpseBsonKey, corpseSlug} {
				_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{
					Filters: json.RawMessage(`[{"property":"` + key + `","condition":"equal","value":"2027-01-01"}]`)}, 0, 25)
				requireBadRequest(t, err)
			}
		})
		t.Run("compact filter string", func(t *testing.T) {
			fx := newFx(t, prodShape)
			_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{
				Filter: corpseBsonKey + ` = "2027-01-01"`}, 0, 25)
			requireBadRequest(t, err)
		})
		t.Run("sorts", func(t *testing.T) {
			fx := newFx(t, prodShape)
			_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{
				Sorts: json.RawMessage(`[{"property":"` + corpseBsonKey + `","direction":"asc"}]`)}, 0, 25)
			requireBadRequest(t, err)
		})
		t.Run("fields selection", func(t *testing.T) {
			fx := newFx(t, prodShape)
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
// uninstalled case, which is the common one, did not. The fix is the
// explicit no-op `isDeleted Condition None` clause beside the isArchived
// one; both shapes now round-trip.
//
// How this fixture can fail: the corpse is BSON-keyed WITH a stored slug, so
// dropping either no-op clause fails the prod leg, and any resolution of the
// corpse's SLUG instead of its stored key fails the third subtest.
// Revert check (executed): removing the `isDeleted Condition None` filter
// from relationObjectHoldingKey fails the prod-shape leg — the create 400s
// with "unknown property keys".
func TestV2CloneToleranceSurvivesTheProdShape(t *testing.T) {
	cloneBody := []byte(`{"version":1,"type":"page","properties":{"name":"Fresh","` + corpseBsonKey + `":"x"}}`)

	t.Run("the clone loop round-trips on BOTH shapes", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, prodShape bool) {
			// given
			fx := newV2Fixture(t)
			fx.addCorpseProperty(t, prodShape)
			captured := fx.expectCreate("clone1")
			fx.expectEtagRead("clone1")

			// when — the bytes a GET of a corpse-held object serves
			_, err := fx.CreateObject(context.Background(), testSpaceId, cloneBody, false)

			// then — the value lands under the STORED key it was served under
			require.NoError(t, err)
			require.NotNil(t, *captured)
			assert.Equal(t, "x", (*captured).Details.Fields[corpseBsonKey].GetStringValue())
		})
	})

	t.Run("the corpse SLUG is refused on create in both shapes", func(t *testing.T) {
		// the tolerance is a round-trip escape for the STORED key only; the
		// slug vacated the namespace and must not be an address
		corpseShapes(t, func(t *testing.T, prodShape bool) {
			fx := newV2Fixture(t)
			fx.addCorpseProperty(t, prodShape)

			_, err := fx.CreateObject(context.Background(), testSpaceId,
				[]byte(`{"version":1,"type":"page","properties":{"name":"Fresh","`+corpseSlug+`":"x"}}`), false)

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
	corpseDoc := `{"version":1,"id":"obj1","type":"page","properties":{"name":"Doc","` + corpseBsonKey + `":"2027-01-01"},"blocks":[{"id":"blockOne1","type":"paragraph","text":"hi"}]}`
	cleanDocNoCorpse := `{"version":1,"id":"obj1","type":"page","properties":{"name":"Doc"},"blocks":[{"id":"blockOne1","type":"paragraph","text":"hi"}]}`

	t.Run("set by stored key, value on the document: applies (prod shape)", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, true)
		captured := fx.expectMutate(editRead(t, corpseDoc), "headB")

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","set":{"`+corpseBsonKey+`":"2030-12-31"}}`), "", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "2030-12-31", (*captured).CombinedDetails().GetString(domain.RelationKey(corpseBsonKey)))
	})

	t.Run("set by stored key, value NOT on the document: 400", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, true)
		fx.expectMutate(editRead(t, cleanDocNoCorpse), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","set":{"`+corpseBsonKey+`":"2030-12-31"}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
	})

	t.Run("unset by stored key removes the corpse-held value", func(t *testing.T) {
		// the one cleanup channel a caller has left
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, true)
		captured := fx.expectMutate(editRead(t, corpseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","unset":["`+corpseBsonKey+`"]}`), "", false)

		require.NoError(t, err)
		_, present := (*captured).CombinedDetails().TryString(domain.RelationKey(corpseBsonKey))
		assert.False(t, present, "unset removes the value")
	})

	t.Run("the corpse slug is refused even with the value on the document", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, true)
		fx.expectMutate(editRead(t, corpseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","set":{"`+corpseSlug+`":"2030-12-31"}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
	})
}

// TestV2ViewOpsCorpseKeys: updateView applies the same two-tier rule — a
// corpse key already ON the dataview (a column, filter or sort the surface
// already shows) stays editable and referencable (§8.17: an edit must not
// reject what the surface already shows), while introducing that key to a
// view that does not carry it is refused like any unknown key.
// Revert check: dropping the preKnown escape in validateViewKeys fails the
// in-view subtests.
func TestV2ViewOpsCorpseKeys(t *testing.T) {
	ctx := context.Background()
	corpseViewDocBody := `{"version":1,"id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
		`{"id":"dataview","type":"dataview",` +
		`"properties":[{"key":"name","format":"text"},{"key":"` + corpseBsonKey + `","format":"date"}],` +
		`"views":[{"id":"viewAll1","name":"All",` +
		`"filters":[{"property":"` + corpseBsonKey + `","condition":"equal","value":"x"}],` +
		`"columns":[{"property":"name"},{"property":"` + corpseBsonKey + `","width":100}]}]}]}`
	plainViewDocBody := `{"version":1,"id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
		`{"id":"dataview","type":"dataview",` +
		`"properties":[{"key":"name","format":"text"}],` +
		`"views":[{"id":"viewAll1","name":"All","columns":[{"property":"name"}]}]}]}`

	t.Run("a view already showing the corpse key stays editable", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, true)
		captured := fx.expectMutate(editRead(t, corpseViewDocBody), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","view":"viewAll1","set":{"name":"Renamed"}}`), "", false)

		require.NoError(t, err)
		require.NotNil(t, *captured)
	})

	t.Run("groupBy an in-view corpse key is accepted", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, true)
		fx.expectMutate(editRead(t, corpseViewDocBody), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","view":"viewAll1","set":{"groupBy":"`+corpseBsonKey+`"}}`), "", false)

		require.NoError(t, err)
	})

	t.Run("introducing the corpse key to a view without it is refused", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, true)
		for _, op := range []string{
			`{"op":"updateView","view":"viewAll1","set":{"groupBy":"` + corpseBsonKey + `"}}`,
			`{"op":"updateView","view":"viewAll1","columns":{"` + corpseBsonKey + `":{"width":80}}}`,
		} {
			fx.expectMutate(editRead(t, plainViewDocBody), "headB")
			_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(op), "", false)
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

// TestV2UninstalledBundledPropertyRefusesWrites (was
// TestV2UninstalledBundledPropertyWriteAsymmetry, which pinned the
// half-applied policy this fixes).
//
// For a BUNDLED relation the corpse policy used to half-apply: uninstalling
// dueDate removed it from listings and 404'd its routes, but the bundled
// vocabulary (resolution chain step 3, propertyKeyExistsIn's
// bundle.HasRelation arm) kept `due_date` a valid DOCUMENT key in every
// space — so a create landed new data in the property the user deleted, and
// the reinstall would light it back up. Now every write channel consults
// uninstalledBundledKeys and refuses with a repair.
//
// The distinction that makes this safe is NEVER-INSTALLED vs UNINSTALLED: a
// bundled relation nobody installed has no relation object at all, is absent
// from the removal set, and keeps working exactly as before (the third
// subtest — it is the common case in a fresh space, and conflating the two
// would break ordinary writes everywhere).
//
// Fixture notes: the removed entity here is bundled, so it CANNOT be
// BSON-keyed with a stored slug — its key is `dueDate` by definition and its
// slug `due_date` is derived in code, never stored; that is precisely the
// class this refusal is about. The BSON-keyed corpse with a stored slug rides
// along in the last subtest, which proves the refusal targets the bundled
// class only and leaves the §8.29 tolerance intact. Both store shapes run:
// the flag-only shape would pass even with the isDeleted default unhandled,
// so only the prod leg proves uninstalledBundledKeys suppresses it.
// Revert check (executed): dropping the removedPropertyIssue arm from
// validatePropertyKeys fails the create subtest, and dropping it from
// stateops' checkKey fails the PATCH subtest.
func TestV2UninstalledBundledPropertyRefusesWrites(t *testing.T) {
	ctx := context.Background()
	newFx := func(t *testing.T, prodShape bool) *v2Fixture {
		fx := newV2Fixture(t)
		obj := objectstore.TestObject{
			bundle.RelationKeyId:            domain.String("rel-duedate"),
			bundle.RelationKeyRelationKey:   domain.String("dueDate"),
			bundle.RelationKeyName:          domain.String("Due date"),
			bundle.RelationKeyIsUninstalled: domain.Bool(true),
		}
		if prodShape {
			obj[bundle.RelationKeyIsDeleted] = domain.Bool(true)
		}
		fx.addRelation(t, testSpaceId, obj)
		return fx
	}
	// requireRemovalRefusal asserts the 400 names the removal AND its repair
	// (§8.34: a refusal a caller cannot act on is itself a defect).
	requireRemovalRefusal := func(t *testing.T, err error) {
		t.Helper()
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, `"due_date"`, "the refusal spells the served slug")
		assert.Contains(t, apiErr.Issues[0].Message, "removed from this space")
		assert.NotEmpty(t, apiErr.Issues[0].Hint, "the repair is named")
	}

	t.Run("the route side is corpse-aware: 404", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, prodShape bool) {
			fx := newFx(t, prodShape)
			_, err := fx.requireLiveProperty(testSpaceId, "due_date")
			requireNotFoundError(t, err)
		})
	})

	t.Run("create refuses to land a value on it", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, prodShape bool) {
			fx := newFx(t, prodShape)

			_, err := fx.CreateObject(ctx, testSpaceId,
				[]byte(`{"version":1,"type":"page","properties":{"name":"n","due_date":"2027-01-01"}}`), false)

			requireRemovalRefusal(t, err)
		})
	})

	t.Run("PATCH refuses it off-document, keeps the in-document cleanup", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, prodShape bool) {
			cleanDoc := `{"version":1,"id":"obj1","type":"page","properties":{"name":"Doc"},"blocks":[{"id":"blockOne1","type":"paragraph","text":"hi"}]}`
			holdingDoc := `{"version":1,"id":"obj1","type":"page","properties":{"name":"Doc","dueDate":"2027-01-01"},"blocks":[{"id":"blockOne1","type":"paragraph","text":"hi"}]}`

			t.Run("off-document set: refused", func(t *testing.T) {
				fx := newFx(t, prodShape)
				fx.expectMutate(editRead(t, cleanDoc), "headB")

				_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
					patchBody(`{"op":"setProperties","set":{"due_date":"2030-12-31"}}`), "", false)

				requireRemovalRefusal(t, err)
			})

			t.Run("unset of a value the document carries: still the cleanup channel", func(t *testing.T) {
				fx := newFx(t, prodShape)
				captured := fx.expectMutate(editRead(t, holdingDoc), "headB")

				_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
					patchBody(`{"op":"setProperties","unset":["due_date"]}`), "", false)

				require.NoError(t, err)
				_, present := (*captured).CombinedDetails().TryString(bundle.RelationKeyDueDate)
				assert.False(t, present, "removing a removed property's leftover value must stay possible")
			})
		})
	})

	t.Run("a view cannot gain a column for it either", func(t *testing.T) {
		// no removal check was added to the view channel and none is needed:
		// view documents spell SLUGS (canonicalViewKey → servedKey), and the
		// bundled slug stops resolving the moment the relation is
		// uninstalled, so validateViewKeys refuses at its unknown-key branch.
		// Pinned because the channel's closure is the load-bearing fact, not
		// which branch closes it.
		corpseShapes(t, func(t *testing.T, prodShape bool) {
			fx := newFx(t, prodShape)
			plainViewDoc := `{"version":1,"id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
				`{"id":"dataview","type":"dataview","properties":[{"key":"name","format":"text"}],` +
				`"views":[{"id":"viewAll1","name":"All","columns":[{"property":"name"}]}]}]}`
			fx.expectMutate(editRead(t, plainViewDoc), "headB")

			_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
				patchBody(`{"op":"updateView","view":"viewAll1","columns":{"due_date":{"width":80}}}`), "", false)

			apiErr := v2Err(t, err)
			assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		})
	})

	t.Run("a NEVER-installed bundled property still works", func(t *testing.T) {
		// the whole distinction: no relation object exists for dueDate here,
		// so install-on-write is untouched — the common case in a fresh space
		fx := newV2Fixture(t)
		captured := fx.expectCreate("obj-due")
		fx.expectEtagRead("obj-due")

		_, err := fx.CreateObject(ctx, testSpaceId,
			[]byte(`{"version":1,"type":"page","properties":{"name":"n","due_date":"2027-01-01"}}`), false)

		require.NoError(t, err)
		require.NotNil(t, *captured)
		_, landed := (*captured).Details.Fields["dueDate"]
		assert.True(t, landed, "a bundled property nobody removed installs on write")
	})

	t.Run("an ARCHIVED bundled property is outside this refusal", func(t *testing.T) {
		// the boundary of the decision implemented here: it keys on
		// isUninstalled — the UI delete the user performed — and nothing else.
		// A v2 DELETE only archives, and whether that should refuse writes too
		// is an open question this round did not settle; the behavior is
		// pinned so answering it later is a conscious edit, and so widening
		// uninstalledBundledKeys past isUninstalled=true cannot pass silently.
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:          domain.String("rel-duedate-archived"),
			bundle.RelationKeyRelationKey: domain.String("dueDate"),
			bundle.RelationKeyName:        domain.String("Due date"),
			bundle.RelationKeyIsArchived:  domain.Bool(true),
		})
		captured := fx.expectCreate("obj-due-arch")
		fx.expectEtagRead("obj-due-arch")

		_, err := fx.CreateObject(ctx, testSpaceId,
			[]byte(`{"version":1,"type":"page","properties":{"name":"n","due_date":"2027-01-01"}}`), false)

		require.NoError(t, err)
		_, landed := (*captured).Details.Fields["dueDate"]
		assert.True(t, landed)
	})

	t.Run("the BSON-keyed custom corpse stays tolerated", func(t *testing.T) {
		// the refusal is bundled-only: a custom corpse's stored key can never
		// be reinstalled, so §8.29's clone tolerance still carries its value
		corpseShapes(t, func(t *testing.T, prodShape bool) {
			fx := newFx(t, prodShape)
			fx.addCorpseProperty(t, prodShape)
			captured := fx.expectCreate("obj-both")
			fx.expectEtagRead("obj-both")

			_, err := fx.CreateObject(ctx, testSpaceId,
				[]byte(`{"version":1,"type":"page","properties":{"name":"n","`+corpseBsonKey+`":"x"}}`), false)

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
	corpseShapes(t, func(t *testing.T, prodShape bool) {
		// given
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, prodShape)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-recreated"),
			bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e202"),
			bundle.RelationKeyApiObjectKey: domain.String(corpseSlug),
			bundle.RelationKeyName:         domain.String("Warranty until"),
		})

		// when — a document exported before the uninstall, naming the slug
		body, err := fx.canonicalizeDocumentKeys(testSpaceId,
			[]byte(`{"version":1,"type":"page","properties":{"`+corpseSlug+`":"x"}}`))

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
// tell a mint from a resolve at all.
// Revert check (executed): removing the relationObjectHoldingKey arm from
// PropertyId fails the second subtest on both shapes — a mint reappears.
func TestV2TypePropertiesCorpseEchoResolvesToItsHolder(t *testing.T) {
	newFx := func(t *testing.T, prodShape bool) *v2Fixture {
		fx := newV2Fixture(t)
		fx.addCorpseProperty(t, prodShape)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String("type-live"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-livetype"),
			bundle.RelationKeyName:                 domain.String("Live type"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-corpse-bson"}),
		})
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
					"recommendedRelations": pbtypes.StringList([]string{"rel-corpse-bson"}),
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
		corpseShapes(t, func(t *testing.T, prodShape bool) {
			// given
			fx := newFx(t, prodShape)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "type-live").Return(liveTypeRead(), nil)

			// when
			body, _, err := fx.GetType(context.Background(), testSpaceId, "livetype", V2ObjectQuery{})

			// then — served under the stored BSON key, with the corpse's name
			require.NoError(t, err)
			var doc struct {
				TypeProperties []struct {
					Key  string `json:"key"`
					Name string `json:"name"`
				} `json:"typeProperties"`
			}
			require.NoError(t, json.Unmarshal(body, &doc))
			require.Len(t, doc.TypeProperties, 1)
			assert.Equal(t, corpseBsonKey, doc.TypeProperties[0].Key)
			assert.Equal(t, "Warranty until", doc.TypeProperties[0].Name)
		})
	})

	t.Run("PATCHing that list back resolves to the holder — no mint", func(t *testing.T) {
		corpseShapes(t, func(t *testing.T, prodShape bool) {
			// given
			fx := newFx(t, prodShape)
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
				[]byte(`{"typeProperties":[{"key":"`+corpseBsonKey+`","name":"Warranty until","format":"text"}]}`), false)

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
			assert.Equal(t, []string{"rel-corpse-bson"}, recommended)
		})
	})

	t.Run("the corpse SLUG in typeProperties still mints — the namespace vacated", func(t *testing.T) {
		// the resolve is KEY-ONLY: a corpse's slug is free for re-minting
		// (§8-OQ2), so naming it declares a NEW property, never the corpse
		corpseShapes(t, func(t *testing.T, prodShape bool) {
			fx := newFx(t, prodShape)
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
				[]byte(`{"typeProperties":[{"key":"`+corpseSlug+`","name":"Warranty until","format":"text"}]}`), false)

			require.NoError(t, err)
			require.Len(t, minted, 1)
			assert.Equal(t, corpseSlug,
				minted[0].Details.Fields[bundle.RelationKeyApiObjectKey.String()].GetStringValue())
		})
	})
}
