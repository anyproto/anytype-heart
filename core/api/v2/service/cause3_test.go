package v2service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Review cause 3: output learned slugs, input did not — the listings
// advertised served spellings the query channels rejected, and GET emitted
// keys PUT rejected. Every channel below takes the advertised spelling.

// slugQueryFixture is slugSpaceFixture plus one object carrying a value
// under the BSON stored key — the row a slug-spelled query must find.
func slugQueryFixture(t *testing.T) *v2Fixture {
	fx := slugSpaceFixture(t)
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:               domain.String("note1"),
		bundle.RelationKeyName:             domain.String("Standup"),
		bundle.RelationKeyType:             domain.String("type-meeting"),
		domain.RelationKey(slugPropKey):    domain.String("hello"),
		bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
		bundle.RelationKeyLastModifiedBy:   domain.String("x"),
		bundle.RelationKeyLastOpenedDate:   domain.Int64(1),
		bundle.RelationKeyLastUsedDate:     domain.Int64(1),
		bundle.RelationKeyCreatedDate:      domain.Int64(1),
		bundle.RelationKeyLastModifiedDate: domain.Int64(1000),
	}})
	return fx
}

func TestV2SearchSpeaksServedSpellings(t *testing.T) {
	ctx := context.Background()

	t.Run("a structured filter on the served slug finds the stored-key value", func(t *testing.T) {
		// before: the filter bound RelationKey "manual_property" — a
		// spelling the store never matches — silently zero rows
		fx := slugQueryFixture(t)

		rows, total, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{
			Filters: json.RawMessage(`[{"property":"manual_property","condition":"equal","value":"hello"}]`),
		}, 0, 25)

		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, rows, 1)
		assert.Equal(t, "note1", rows[0].Id)
	})

	t.Run("the compact filter string takes the served slug too", func(t *testing.T) {
		fx := slugQueryFixture(t)

		rows, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{
			Filter: `manual_property = "hello"`,
		}, 0, 25)

		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "note1", rows[0].Id)
	})

	t.Run("fields= takes the served slug and emits under it", func(t *testing.T) {
		fx := slugQueryFixture(t)

		rows, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{
			Fields: []string{"manual_property"},
		}, 0, 25)

		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.NotNil(t, rows[0].Properties)
		assert.Equal(t, "hello", rows[0].Properties["manual_property"],
			"the value reads from the stored key and emits under the requested spelling")
	})

	t.Run("sorts take the served slug", func(t *testing.T) {
		fx := slugQueryFixture(t)

		_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{
			Sorts: json.RawMessage(`[{"property":"manual_property","direction":"asc"}]`),
		}, 0, 25)

		require.NoError(t, err)
	})

	t.Run("a type filter LEAF resolves the slug and rejects a corpse", func(t *testing.T) {
		// the reviewed inconsistency: the same spelling worked at top level
		// and 400'd one level down, and a UI-deleted type was a usable
		// query scope
		fx := slugQueryFixture(t)

		rows, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{
			Filters: json.RawMessage(`[{"property":"type","condition":"equal","value":"meeting_note"}]`),
		}, 0, 25)
		require.NoError(t, err)
		require.Len(t, rows, 1)

		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:            domain.String("type-corpse"),
			bundle.RelationKeyUniqueKey:     domain.String("ot-corpsetype"),
			bundle.RelationKeyName:          domain.String("Gone"),
			bundle.RelationKeyIsUninstalled: domain.Bool(true),
		})
		_, _, _, _, err = fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{
			Filters: json.RawMessage(`[{"property":"type","condition":"equal","value":"corpsetype"}]`),
		}, 0, 25)
		var apiErr *v2model.Error
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
	})

	t.Run("the did-you-mean candidates speak the served spelling", func(t *testing.T) {
		fx := slugQueryFixture(t)

		_, _, _, _, err := fx.SearchObjects(ctx, testSpaceId, v2model.SearchRequest{
			Filters: json.RawMessage(`[{"property":"manual_prop","condition":"equal","value":"x"}]`),
		}, 0, 25)

		var apiErr *v2model.Error
		require.ErrorAs(t, err, &apiErr)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Hint+apiErr.Issues[0].Message, "manual_property",
			"the candidate list must advertise the spelling the channel accepts")
		assert.NotContains(t, apiErr.Issues[0].Message, slugPropKey,
			"never advertise the BSON spelling once the slug serves")
	})
}

func TestV2ListFieldsSpeakServedSpellings(t *testing.T) {
	fx := slugQueryFixture(t)
	assert.NoError(t, fx.validateListFields(testSpaceId, []string{"manual_property"}))
	assert.NoError(t, fx.validateListFields(testSpaceId, []string{slugPropKey}), "the stored spelling stays valid")
	err := fx.validateListFields(testSpaceId, []string{"not_a_key"})
	require.Error(t, err)
}

func TestV2TypeKeyExistsIsCorpseAndChainAware(t *testing.T) {
	fx := slugSpaceFixture(t)
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:            domain.String("type-corpse"),
		bundle.RelationKeyUniqueKey:     domain.String("ot-corpsetype"),
		bundle.RelationKeyName:          domain.String("Gone"),
		bundle.RelationKeyIsUninstalled: domain.Bool(true),
	})

	t.Run("creating an object of a UI-deleted type is refused", func(t *testing.T) {
		// before: typeKeyExists resolved the corpse and the create passed
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"corpsetype","properties":{"name":"zombie"}}`), false, true)

		var apiErr *v2model.Error
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
	})

	t.Run("the slug spelling gates through", func(t *testing.T) {
		captured := fx.expectCreate("obj-slugtyped")
		fx.expectEtagRead("obj-slugtyped")

		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"meeting_note","properties":{"name":"ok"}}`), false, true)

		require.NoError(t, err)
		require.NotNil(t, *captured)
	})
}

// corpseKeyCloneFixture holds a UI-deleted relation whose key an object
// still carries, plus a LIVE relation spelled one character away — the
// near-miss that made the refusal actively harmful (the did-you-mean steered
// the caller to move the value onto an unrelated property).
func corpseKeyCloneFixture(t *testing.T) *v2Fixture {
	fx := newV2Fixture(t)
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:            domain.String("rel-corpse"),
		bundle.RelationKeyRelationKey:   domain.String("corpse_key"),
		bundle.RelationKeyName:          domain.String("Deleted in UI"),
		bundle.RelationKeyIsUninstalled: domain.Bool(true),
	})
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:          domain.String("rel-nearmiss"),
		bundle.RelationKeyRelationKey: domain.String("corpse_kez"),
		bundle.RelationKeyName:        domain.String("Near miss"),
	})
	return fx
}

func TestV2CreateToleratesCorpseHeldKeys(t *testing.T) {
	// Review cause 3 was framed as a GET→PUT concern and its tolerance was
	// retired with PUT (§8.27) on the grounds that create is live-only and
	// PATCH names only what it edits. BOTH halves were wrong (§8.29): PATCH
	// has its own in-document escape (checkKey passes any key already on the
	// document), and create is the channel a read body is pasted into — "a
	// pasted read body creates a copy". Live-only there broke the one loop
	// it is advertised for.
	t.Run("a read body holding a corpse key creates a copy", func(t *testing.T) {
		// given
		fx := corpseKeyCloneFixture(t)
		captured := fx.expectCreate("clone1")
		fx.expectEtagRead("clone1")

		// when — the bytes a GET of such an object serves
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"page","properties":{"name":"Fresh","corpse_key":"x"}}`), false, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, *captured)
		assert.Equal(t, "x", (*captured).Details.Fields["corpse_key"].GetStringValue(),
			"the value is carried, not dropped")
	})

	t.Run("a key no relation holds at all is still refused", func(t *testing.T) {
		// the tolerance is a round-trip escape, not an address: only a key
		// SOME relation object holds passes
		fx := corpseKeyCloneFixture(t)

		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"page","properties":{"name":"Fresh","never_existed":"x"}}`), false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/properties/never_existed", apiErr.Issues[0].Path)
	})

	t.Run("no did-you-mean steers a corpse-held key onto a near-miss live property", func(t *testing.T) {
		// the actively harmful half: "did you mean corpse_kez?" invited the
		// caller to write the value onto an unrelated property
		fx := corpseKeyCloneFixture(t)
		fx.expectCreate("clone2")
		fx.expectEtagRead("clone2")

		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"page","properties":{"name":"Fresh","corpse_key":"x"}}`), false, true)

		require.NoError(t, err)
	})

	t.Run("an ARCHIVED relation's key is tolerated too", func(t *testing.T) {
		// the two ways a relation dies are not the same query. The fixture
		// above uses isUninstalled (UI delete), which no store default
		// filters; the explicit no-op `isArchived Condition:None` in
		// propertyKeyHeldByAnyRelation exists solely to suppress the store's
		// INJECTED isArchived:false default — so without that clause an
		// archived claimant is invisible and this create 400s. It was the one
		// arm of the tolerance nothing pinned.
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:          domain.String("rel-archived"),
			bundle.RelationKeyRelationKey: domain.String("archived_key"),
			bundle.RelationKeyName:        domain.String("Archived"),
			bundle.RelationKeyIsArchived:  domain.Bool(true),
		})
		captured := fx.expectCreate("clone3")
		fx.expectEtagRead("clone3")

		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"page","properties":{"name":"Fresh","archived_key":"x"}}`), false, true)

		require.NoError(t, err)
		require.NotNil(t, *captured)
		assert.Equal(t, "x", (*captured).Details.Fields["archived_key"].GetStringValue())
	})
}

func TestV2FieldAliasSurvivesACorpseClaimant(t *testing.T) {
	// one uninstalled mimeType relation used to deactivate the alias
	// space-wide — silently dropping the field from every file row/filter
	fx := newV2Fixture(t)
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:            domain.String("rel-mime-corpse"),
		bundle.RelationKeyRelationKey:   domain.String("mimeType"),
		bundle.RelationKeyName:          domain.String("Old custom mimeType"),
		bundle.RelationKeyIsUninstalled: domain.Bool(true),
	})

	aliases := fx.activeFieldAliases(testSpaceId)
	assert.Equal(t, bundle.RelationKeyFileMimeType, aliases["mimeType"],
		"a corpse must not deactivate the alias")

	// a LIVE claimant still wins
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:          domain.String("rel-mime-live"),
		bundle.RelationKeyRelationKey: domain.String("mimeType"),
		bundle.RelationKeyName:        domain.String("Custom mimeType"),
	})
	aliases = fx.activeFieldAliases(testSpaceId)
	_, active := aliases["mimeType"]
	assert.False(t, active, "a live property claiming the spelling deactivates the alias")
}

func TestV2CreateSetCanonicalizesViewKeys(t *testing.T) {
	// the set DOCUMENT persists filter keys — a served slug landing there
	// would bind a dataview filter the store never matches, silently
	fx := slugSpaceFixture(t)
	// the type recommends the slug-keyed property, so it is in the R9 set
	// (a full record — AddObjects replaces by id)
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:                   domain.String("type-meeting"),
		bundle.RelationKeyUniqueKey:            domain.String("ot-" + slugTypeKey),
		bundle.RelationKeyApiObjectKey:         domain.String("meeting_note"),
		bundle.RelationKeyName:                 domain.String("Meeting note"),
		bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-manual"}),
	})
	captured := fx.expectCreate("set1")
	fx.expectEtagRead("set1")

	result, err := fx.CreateSet(context.Background(), testSpaceId, v2model.CreateSetRequest{
		Name: "My set", Type: "meeting_note",
		Filters: json.RawMessage(`[{"property":"manual_property","condition":"equal","value":"hello"}]`),
	}, false, true)

	require.NoError(t, err)
	require.NotNil(t, *captured)
	_ = result
	// the dataview block's filter must bind the STORED key
	var filterKeys []string
	for _, block := range (*captured).Blocks {
		if dv := block.GetDataview(); dv != nil {
			for _, view := range dv.Views {
				for _, filter := range view.Filters {
					filterKeys = append(filterKeys, filter.RelationKey)
				}
			}
		}
	}
	assert.Contains(t, filterKeys, slugPropKey)
	assert.NotContains(t, filterKeys, "manual_property")
}

func TestV2FilterStringTakesBundledSlugs(t *testing.T) {
	// the compact string validates keys BEFORE canonicalization, so the
	// acceptance set must carry the bundled derived slug too — due_date in
	// a filter string must work exactly as it does on routes and documents
	fx := slugQueryFixture(t)
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:             domain.String("rel-duedate"),
		bundle.RelationKeyRelationKey:    domain.String("dueDate"),
		bundle.RelationKeyName:           domain.String("Due date"),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
	})

	_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, v2model.SearchRequest{
		Filter: `due_date IS EMPTY`,
	}, 0, 25)

	require.NoError(t, err)
}
