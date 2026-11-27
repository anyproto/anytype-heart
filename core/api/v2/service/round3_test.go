package v2service

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// candidateAddresses pulls every "(key X, id Y)" / "(stored key X, id Y)" an
// ambiguity message advertises.
var candidateAddresses = regexp.MustCompile(`\((?:stored )?key ([^,()]+), id ([^)]+)\)`)

// An ambiguity refusal's advertised address must resolve back to the holder
// it is offered for. A slug another property's STORED key shadows is not
// such an address: exact stored keys win, so a retry with it would write to
// the other property.
func TestV2AmbiguityAddressesResolveToTheirHolder(t *testing.T) {
	t.Run("properties", func(t *testing.T) {
		// given: two properties named alike with slugs alpha and beta, and a
		// legacy property whose stored key is alpha
		fx := newV2Fixture(t)
		for _, p := range []struct{ id, key, slug, name string }{
			{"rel-shared-a", "6a7663db61fab21cd4b9e5a1", "alpha", "Shared label"},
			{"rel-shared-b", "6a7663db61fab21cd4b9e5b1", "beta", "Shared label"},
			{"rel-legacy", "alpha", "", "Legacy"},
		} {
			fx.addRelation(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:           domain.String(p.id),
				bundle.RelationKeyRelationKey:  domain.String(p.key),
				bundle.RelationKeyApiObjectKey: domain.String(p.slug),
				bundle.RelationKeyName:         domain.String(p.name),
			})
		}
		entries, err := fx.liveProperties(testSpaceId)
		require.NoError(t, err)

		// when
		_, ok, ambiguous := fx.resolvePropertyInput("Shared label", entries)

		// then: every advertised key reaches the advertised id
		require.False(t, ok)
		require.Len(t, ambiguous, 2)
		for _, candidate := range ambiguous {
			match := candidateAddresses.FindStringSubmatch(candidate)
			require.NotNil(t, match, candidate)
			entry, resolved, again := fx.resolvePropertyInput(match[1], entries)
			require.True(t, resolved, "%s does not resolve", match[1])
			require.Empty(t, again)
			assert.Equal(t, match[2], entry.Id, "%q resolves to another property", match[1])
		}
	})

	t.Run("types", func(t *testing.T) {
		// given: the same shape in the type namespace
		fx := newV2Fixture(t)
		for _, tp := range []struct{ id, key, slug, name string }{
			{"type-shared-a", "6a7663db61fab21cd4b9e6a1", "alpha", "Errand"},
			{"type-shared-b", "6a7663db61fab21cd4b9e6b1", "beta", "Errand"},
			{"type-legacy", "alpha", "", "Legacy"},
		} {
			fx.addType(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:           domain.String(tp.id),
				bundle.RelationKeyUniqueKey:    domain.String("ot-" + tp.key),
				bundle.RelationKeyApiObjectKey: domain.String(tp.slug),
				bundle.RelationKeyName:         domain.String(tp.name),
			})
		}
		entries, err := fx.liveTypes(testSpaceId)
		require.NoError(t, err)

		// when
		_, ok, ambiguous, err := fx.resolveTypeInput(testSpaceId, "Errand", entries)

		// then
		require.NoError(t, err)
		require.False(t, ok)
		require.Len(t, ambiguous, 2)
		for _, candidate := range ambiguous {
			match := candidateAddresses.FindStringSubmatch(candidate)
			require.NotNil(t, match, candidate)
			entry, resolved, again, err := fx.resolveTypeInput(testSpaceId, match[1], entries)
			require.NoError(t, err)
			require.True(t, resolved, "%s does not resolve", match[1])
			require.Empty(t, again)
			assert.Equal(t, match[2], entry.Id, "%q resolves to another type", match[1])
		}
	})
}

// A refused type PATCH creates nothing. A declared option name two existing
// options share is the resolver's ambiguity refusal — and the guard, which
// runs before any mint, used to reduce names to booleans, so the batch
// passed it and an earlier op's property was minted before the refusal.
func TestV2TypeOpsAmbiguousDeclaredOptionCreatesNothing(t *testing.T) {
	// given: harvest_stage holds two options named Won; no create
	// expectation is wired, so minting op 0's property fails the test
	fx := newTypeOpsFixture(t)
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:             domain.String("rel-harvest"),
		bundle.RelationKeyRelationKey:    domain.String("6a8f2c1d9e4b7a3f5c2d8e11"),
		bundle.RelationKeyApiObjectKey:   domain.String("harvest_stage"),
		bundle.RelationKeyName:           domain.String("Harvest stage"),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
	})
	for _, id := range []string{"opt-won-a", "opt-won-b"} {
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String(id),
			bundle.RelationKeyRelationKey:    domain.String("6a8f2c1d9e4b7a3f5c2d8e11"),
			bundle.RelationKeyName:           domain.String("Won"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
		}})
	}

	// when: a real run, with consent
	_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "", opsBody(
		`{"op":"add_property","property":"Brand new","format":"text"}`,
		`{"op":"add_property","property":"harvest_stage","options":[{"name":"Won"}]}`,
	), false, true)

	// then
	apiErr := v2Err(t, err)
	assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
	assert.Equal(t, `option name "Won" is ambiguous for property "harvest_stage"`, apiErr.Message)
	require.Len(t, apiErr.Issues, 1)
	assert.Equal(t, "/ops/1/options", apiErr.Issues[0].Path)
}

// The option guard resolves the property as the apply will. It compared
// exact strings, so a folded spelling of an existing property looked like a
// property the request would create, and was refused for consent.
func TestV2TypeOpsOptionGuardResolvesLikeTheApply(t *testing.T) {
	for _, spelling := range []string{"harvestStage", "HARVEST_STAGE", "Harvest stage"} {
		t.Run(spelling, func(t *testing.T) {
			// given: harvest_stage already holds Early
			fx := newTypeOpsFixture(t)
			fx.addRelation(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:             domain.String("rel-harvest"),
				bundle.RelationKeyRelationKey:    domain.String("6a8f2c1d9e4b7a3f5c2d8e11"),
				bundle.RelationKeyApiObjectKey:   domain.String("harvest_stage"),
				bundle.RelationKeyName:           domain.String("Harvest stage"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
			})
			fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:             domain.String("opt-early"),
				bundle.RelationKeyRelationKey:    domain.String("6a8f2c1d9e4b7a3f5c2d8e11"),
				bundle.RelationKeyName:           domain.String("Early"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			}})

			// the dry run reads the type for its dataview prune plan
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, typeOpsTypeId).Return(typeReadWithViews(), nil).Maybe()

			// when: the whole-type body, whose definitions carry the caller's
			// spelling; a dry run without consent — nothing would be created
			result, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
				[]byte(`{"type_settings":{"property_definitions":[{"property":"`+spelling+`","options":[{"name":"Early"}]}]}}`), true, false)

			// then
			require.NoError(t, err)
			if result.Created != nil {
				assert.Empty(t, result.Created.Options, "no option is created")
				assert.Empty(t, result.Created.Properties, "no property is created")
			}
		})
	}
}

// Two spellings that one property answers to in a type document are found
// by the importer's own binding, not a second resolver that folds otherwise.
func TestV2TypeCreateDuplicateSpellingsBindLikeTheImporter(t *testing.T) {
	addRisk := func(t *testing.T, fx *v2Fixture) {
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-risk"),
			bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e7a1"),
			bundle.RelationKeyApiObjectKey: domain.String("risk_flag"),
			bundle.RelationKeyName:         domain.String("Risk"),
		})
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-other"),
			bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e7b1"),
			bundle.RelationKeyApiObjectKey: domain.String("other_flag"),
			bundle.RelationKeyName:         domain.String("RISK_FLAG"),
		})
	}

	t.Run("two spellings bound to one property name both, not its stored key", func(t *testing.T) {
		// given: RISK_FLAG is other_flag's display name
		fx := newV2Fixture(t)
		addRisk(t, fx)
		want := v2model.Issue{
			Path:    "/properties/other_flag",
			Message: `"RISK_FLAG" and "other_flag" both address one property — keep one`,
		}

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Gadget","RISK_FLAG":"x","other_flag":"y"}}`), false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, want, apiErr.Issues[0])
	})

	t.Run("two spellings bound to two properties are not a duplicate", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		addRisk(t, fx)

		// when: risk_flag is one property's slug, RISK_FLAG another's name
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Gadget","risk_flag":"x","RISK_FLAG":"y"}}`), true, true)

		// then: whatever else the dry run says, it is not a duplicate refusal
		if err != nil {
			assert.NotEqual(t, "duplicate property key", v2Err(t, err).Message)
		}
	})
}

// The full-document create channel locates an option refusal inside a
// dataview block's view by the block, view and leaf the caller wrote.
func TestCreateOptionRefusalInsideABlockView(t *testing.T) {
	// given
	fx, _ := spellingFixture(t)

	// when: no create consent
	_, err := fx.CreateObject(context.Background(), testSpaceId, []byte(`{"formatVersion":"2.0","type":"page",`+
		`"properties":{"name":"Doc"},"blocks":[{"id":"dvOne1","type":"dataview",`+
		`"properties":[{"property":"stage","format":"select"}],`+
		`"views":[{"id":"viewA1","name":"A","filters":[{"property":"stage","condition":"equal","value":"Missing"}]}]}]}`), false, false)

	// then
	apiErr := v2Err(t, err)
	require.Len(t, apiErr.Issues, 1)
	assert.Equal(t, "/blocks/0/views/0/filters/0/value", apiErr.Issues[0].Path)
	assert.Contains(t, apiErr.Issues[0].Message, `property "stage" has no option named "Missing"`)
}

// Compact-filter refusals are addressed at the string; structured ones carry
// the leaf's index.
func TestV2FilterRefusalPaths(t *testing.T) {
	shadowFixture := func(t *testing.T) *v2Fixture {
		// a custom property squatting on the built-in due_date slug, beside
		// the installed built-in: due_date is ambiguous
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-squatter"),
			bundle.RelationKeyRelationKey:    domain.String("6a7663db61fab21cd4b9e107"),
			bundle.RelationKeyApiObjectKey:   domain.String("due_date"),
			bundle.RelationKeyName:           domain.String("Due"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
		})
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-duedate"),
			bundle.RelationKeyRelationKey:    domain.String("dueDate"),
			bundle.RelationKeyName:           domain.String("Due date"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
		})
		return fx
	}

	t.Run("an ambiguous key in a compact search filter", func(t *testing.T) {
		// given
		fx := shadowFixture(t)

		// when
		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId,
			v2model.SearchRequest{Filter: `due_date IS EMPTY`}, 0, 25)

		// then
		apiErr := v2Err(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/filter", apiErr.Issues[0].Path)
	})

	t.Run("an ambiguous key in a compact query filter", func(t *testing.T) {
		// given: a type listing both holders of due_date
		fx := shadowFixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String("type-errand"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-errand"),
			bundle.RelationKeyName:                 domain.String("Errand"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-squatter", "rel-duedate"}),
		})

		// when
		_, err := fx.CreateQuery(context.Background(), testSpaceId, v2model.CreateQueryRequest{
			Name: "Due", Type: "errand", Filter: `due_date IS EMPTY`,
		}, false, true)

		// then
		apiErr := v2Err(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/filter", apiErr.Issues[0].Path)
	})

	t.Run("an unknown type in a structured filter value", func(t *testing.T) {
		// given: chore is a live type, no_such_type is not
		fx := newV2Fixture(t)
		fx.addTaskType(t)

		// when
		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId, v2model.SearchRequest{
			Filters: json.RawMessage(`[{"property":"type","condition":"in","value":["chore"]},{"property":"type","condition":"in","value":["no_such_type"]}]`),
		}, 0, 25)

		// then
		apiErr := v2Err(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/filters/1/value", apiErr.Issues[0].Path)
	})
}

// PropertyId's ambiguity is the typed 400 every channel gives, addressed at
// the definition that named the property — it was a bare error, so a 500.
func TestV2TypeDefinitionAmbiguityIsA400AtTheDefinition(t *testing.T) {
	// given: two properties answer to twin_key
	fx := newV2Fixture(t)
	for i, id := range []string{"rel-twin1", "rel-twin2"} {
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String(id),
			bundle.RelationKeyRelationKey:  domain.String([]string{"6a7663db61fab21cd4b9e007", "6a7663db61fab21cd4b9e008"}[i]),
			bundle.RelationKeyApiObjectKey: domain.String("twin_key"),
			bundle.RelationKeyName:         domain.String("Twin"),
		})
	}

	// when
	_, err := fx.CreateType(context.Background(), testSpaceId,
		[]byte(`{"kind":"object_type","properties":{"name":"Twin user"},"type_settings":{"api_key":"twinuser","property_definitions":[{"property":"twin_key"}]}}`), false, true)

	// then
	apiErr := v2Err(t, err)
	assert.Equal(t, http.StatusBadRequest, apiErr.Status)
	assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
	require.Len(t, apiErr.Issues, 1)
	assert.Equal(t, "/type_settings/property_definitions/0/property", apiErr.Issues[0].Path)
	assert.Contains(t, apiErr.Issues[0].Message, "rel-twin1")
	assert.Contains(t, apiErr.Issues[0].Message, "rel-twin2")
}
