package v2service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// assertNoStoredKey fails when a refusal's message, hint or any reference it
// renders carries a 24-hex stored key: a space-minted property or type is
// addressed by its slug, and its stored key is an id the caller was never
// served (GO-7615).
func assertNoStoredKey(t *testing.T, apiErr *v2model.Error) {
	t.Helper()
	texts := []string{apiErr.Message}
	for _, iss := range apiErr.Issues {
		texts = append(texts, iss.Message, iss.Hint)
		for _, ref := range iss.SeeAlso {
			texts = append(texts, ref.String())
		}
	}
	for _, text := range texts {
		assert.NotRegexp(t, `[0-9a-f]{24}`, text, "a stored bson key reached the caller")
	}
}

func TestV2SlugCollisionRepairsNameTheServedKey(t *testing.T) {
	t.Run("recreating a custom property by its slug points at that slug", func(t *testing.T) {
		// given: manual_property is space-minted, stored under a bson id
		fx := slugSpaceFixture(t)
		update := v2model.RefUpdateProperty(testSpaceId, "manual_property")
		want := v2model.Issue{
			Path:    "/key",
			Message: `key "manual_property" is taken by property "Manual property"`,
			Hint:    "update it with " + update.String() + ", or pick a different key",
			SeeAlso: []v2model.Ref{update},
		}

		// when
		_, err := fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Key: "manual_property", Name: "Another", Format: "text"}, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, want, apiErr.Issues[0])
		assertNoStoredKey(t, apiErr)
	})

	t.Run("a name that derives a taken slug is told to use that slug", func(t *testing.T) {
		// given
		fx := slugSpaceFixture(t)
		want := v2model.Issue{
			Path:    "/name",
			Message: `key "manual_property" is taken by property "Manual property"`,
			Hint:    `use the existing property "manual_property", or pass an explicit different key`,
		}

		// when: no key, so the slug derives from the name
		_, err := fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Name: "Manual property", Format: "text"}, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, want, apiErr.Issues[0])
		assertNoStoredKey(t, apiErr)
	})

	t.Run("recreating a custom type by its slug points at that slug", func(t *testing.T) {
		// given: meeting_note is space-minted, stored under a bson id
		fx := slugSpaceFixture(t)
		update := v2model.RefUpdateType(testSpaceId, "meeting_note")
		want := v2model.Issue{
			Path:    "/type_settings/api_key",
			Message: `key "meeting_note" is taken by type "Meeting note" in space ` + testSpaceId,
			Hint:    "update it with " + update.String() + ", or pick a different key",
			SeeAlso: []v2model.Ref{update},
		}

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Another"},"type_settings":{"api_key":"meeting_note"}}`), false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, want, apiErr.Issues[0])
		assertNoStoredKey(t, apiErr)
	})
}

func TestV2TypeOpsOptionRefusalsNameTheOpsProperty(t *testing.T) {
	t.Run("a new option on an existing custom select, without consent", func(t *testing.T) {
		// given: harvest_stage exists, holds no "Late", and is stored under a bson id
		fx := newTypeOpsFixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-harvest"),
			bundle.RelationKeyRelationKey:    domain.String("6a8f2c1d9e4b7a3f5c2d8e11"),
			bundle.RelationKeyApiObjectKey:   domain.String("harvest_stage"),
			bundle.RelationKeyName:           domain.String("Harvest stage"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
		})
		options := v2model.RefListPropertyOptions(testSpaceId, "harvest_stage")
		resend := v2model.Resend("create_missing_options", "true")
		want := v2model.Issue{
			Path:    "/ops/0.options",
			Message: `property "harvest_stage" has no option named "Late", and this request did not ask to create one`,
			Hint:    "check the spelling against " + options.String() + ", or resend with " + resend.String() + " to create it",
			SeeAlso: []v2model.Ref{options, resend},
		}

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
			opsBody(`{"op":"add_property","property":"harvest_stage","options":[{"name":"Late"}]}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, want, apiErr.Issues[0])
		assertNoStoredKey(t, apiErr)
	})

	t.Run("options on an existing custom text property", func(t *testing.T) {
		// given: water_needs is long text, stored under a bson id
		fx := newTypeOpsFixture(t)
		want := v2model.Issue{
			Path: "/ops/0.options",
			Message: `options apply to select and multi_select properties, and "water_needs" is ` +
				anyblockjson.FormatName(model.RelationFormat_longtext),
		}

		// when: consent granted, so only the format rule can refuse this
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
			opsBody(`{"op":"add_property","property":"water_needs","options":[{"name":"Lots"}]}`), false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, want, apiErr.Issues[0])
		assertNoStoredKey(t, apiErr)
	})
}

func TestV2CompactFilterTypoListsServedKeys(t *testing.T) {
	t.Run("search", func(t *testing.T) {
		// given
		fx := slugQueryFixture(t)

		// when
		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId,
			v2model.SearchRequest{Filter: `manual_propertyy = "x"`}, 0, 25)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].Message, `unknown property key "manual_propertyy"`)
		assert.Contains(t, apiErr.Issues[0].Message, "manual_property,", "the slug is listed")
		assertNoStoredKey(t, apiErr)
	})

	t.Run("query create", func(t *testing.T) {
		// given: a type that lists the space-minted manual_property
		fx := slugSpaceFixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String("type-errand"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-errand"),
			bundle.RelationKeyName:                 domain.String("Errand"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-manual"}),
		})

		// when — no creator expectation: reaching the create path fails the test
		_, err := fx.CreateQuery(context.Background(), testSpaceId, v2model.CreateQueryRequest{
			Name: "Errands", Type: "errand", Filter: `manual_propertyy = "x"`,
		}, false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].Message, `unknown property key "manual_propertyy"`)
		assert.Contains(t, apiErr.Issues[0].Message, "manual_property", "the slug is listed")
		assertNoStoredKey(t, apiErr)
	})
}

func TestV2CreateRefusalsQuoteTheTypeAsSent(t *testing.T) {
	t.Run("a custom type posted to the templates endpoint", func(t *testing.T) {
		// given: meeting_note is space-minted, stored under a bson id
		fx := slugSpaceFixture(t)
		want := v2model.Issue{Path: "/type", Message: `expected type "template", got "meeting_note"`}

		// when
		_, err := fx.CreateTemplate(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"meeting_note","template_for":"meeting_note"}`), false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, want, apiErr.Issues[0])
		assertNoStoredKey(t, apiErr)
	})

	t.Run("collection_items on a custom type", func(t *testing.T) {
		// given
		fx := slugSpaceFixture(t)
		create := v2model.RefCreateCollection(testSpaceId)
		want := v2model.Issue{
			Path:    "/collection_items",
			Message: `collection_items requires type "collection", got "meeting_note"`,
			Hint:    "create a collection with " + create.String(),
			SeeAlso: []v2model.Ref{create},
		}

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"meeting_note","collection_items":["obj1"]}`), false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, want, apiErr.Issues[0])
		assertNoStoredKey(t, apiErr)
	})
}
