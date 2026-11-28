package v2service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// errandFixture is slugSpaceFixture plus a type "errand" that recommends the
// live manual_property and an ARCHIVED space-minted property: nothing strips
// a removed relation from a type's lists, so a candidate list built from the
// type's keys must skip it rather than fall back to its stored key.
func errandFixture(t *testing.T) *v2Fixture {
	fx := slugSpaceFixture(t)
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("rel-gone"),
		bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e1c1"),
		bundle.RelationKeyApiObjectKey: domain.String("gone_prop"),
		bundle.RelationKeyName:         domain.String("Gone"),
		bundle.RelationKeyIsArchived:   domain.Bool(true),
	})
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:                   domain.String("type-errand"),
		bundle.RelationKeyUniqueKey:            domain.String("ot-errand"),
		bundle.RelationKeyName:                 domain.String("Errand"),
		bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-manual", "rel-gone"}),
	})
	return fx
}

func TestV2CompactFilterCandidates(t *testing.T) {
	const wantQueryList = `unknown property key "manual_propertyy" — known property keys: ` +
		`created_date, creator, last_modified_date, last_opened_date, manual_property, name, type`

	t.Run("query create lists served keys only, and no removed property", func(t *testing.T) {
		// given
		fx := errandFixture(t)

		// when — no creator expectation: reaching the create path fails the test
		_, err := fx.CreateQuery(context.Background(), testSpaceId, v2model.CreateQueryRequest{
			Name: "Errands", Type: "errand", Filter: `manual_propertyy = "x"`,
		}, false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, `parse error at offset 0 near "manual_propertyy": `+wantQueryList, apiErr.Issues[0].Message)
	})

	t.Run("type-scoped search lists no removed property", func(t *testing.T) {
		// given
		fx := errandFixture(t)

		// when
		_, _, _, _, err := fx.SearchObjects(context.Background(), testSpaceId,
			v2model.SearchRequest{Type: "errand", Filter: `manual_propertyy = "x"`}, 0, 25)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assertNoStoredKey(t, apiErr)
		assert.NotContains(t, apiErr.Issues[0].Message, "gone_prop", "a removed property is not a candidate")
	})

	t.Run("query create accepts a system key's served spelling", func(t *testing.T) {
		// given: the type recommends no date property of its own
		fx := errandFixture(t)
		fx.expectCreate("newSet")
		fx.expectEtagRead("newSet")

		// when
		_, err := fx.CreateQuery(context.Background(), testSpaceId, v2model.CreateQueryRequest{
			Name: "Recent", Type: "errand", Filter: `created_date >= "2026-07-01"`,
		}, false, true)

		// then
		require.NoError(t, err)
	})

	t.Run("a view filter string accepts a system key's served spelling", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.expectMutate(editRead(t, editSetDoc), "headB")

		// when
		_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"filter":"created_date >= \"2026-07-01\""}}`), "", false, true)

		// then
		require.NoError(t, err)
	})

	t.Run("a view filter string typo lists served system keys", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.expectMutate(editRead(t, editSetDoc), "headB")

		// when
		_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"filter":"severityy = \"High\""}}`), "", false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].Message, "created_date")
		assert.NotContains(t, apiErr.Issues[0].Message, "createdDate")
		assert.NotContains(t, apiErr.Issues[0].Message, "lastModifiedDate")
	})

	t.Run("a structured query typo under keys=name lists names", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.addTaskType(t) // type "chore" recommending "severity"

		// when
		_, err := fx.CreateQuery(CtxWithNameKeys(context.Background()), testSpaceId, v2model.CreateQueryRequest{
			Name: "Chores", Type: "chore",
			Filters: json.RawMessage(`[{"property":"Nope","condition":"empty"}]`),
		}, false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].Message, "known properties of the type: Severity")
	})
}

func TestV2RefusalsQuoteTheTermSent(t *testing.T) {
	t.Run("two spellings of one property in a type document", func(t *testing.T) {
		// given
		fx, _ := spellingFixture(t)
		want := v2model.Issue{
			Path:    "/properties/stage",
			Message: `"Stage" and "stage" both address one property — keep one`,
		}

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Gadget","stage":"x","Stage":"y"}}`), false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, want, apiErr.Issues[0])
	})

	t.Run("a restricted type, by an uppercase spelling", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"IMAGE","properties":{"name":"n"}}`), false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, `creation of "IMAGE" objects is restricted`, apiErr.Issues[0].Message)
	})

	t.Run("a removed type, by its display name", func(t *testing.T) {
		// given: the space removed the bundled task type
		fx := newV2Fixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:            domain.String("drv-ot-task"),
			bundle.RelationKeyUniqueKey:     domain.String("ot-task"),
			bundle.RelationKeyName:          domain.String("Task"),
			bundle.RelationKeyIsUninstalled: domain.Bool(true),
		})

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"Task","properties":{"name":"n"}}`), false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, `type "Task" was removed from this space — nothing new is created in a removed type`, apiErr.Issues[0].Message)
	})

	t.Run("a type key collision quotes the key as submitted", func(t *testing.T) {
		// given: meeting_note is space-minted
		fx := slugSpaceFixture(t)
		update := v2model.RefUpdateType(testSpaceId, "meeting_note")

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Another"},"type_settings":{"api_key":"meetingNote"}}`), false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, `key "meetingNote" is taken by type "Meeting note" in space `+testSpaceId, apiErr.Issues[0].Message)
		assert.Equal(t, []v2model.Ref{update}, apiErr.Issues[0].SeeAlso, "the route takes the normalized slug")
	})

	t.Run("a property key collision quotes the key as submitted", func(t *testing.T) {
		// given
		fx := slugSpaceFixture(t)
		update := v2model.RefUpdateProperty(testSpaceId, "manual_property")

		// when
		_, err := fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Key: "manualProperty", Name: "Another", Format: "text"}, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, `key "manualProperty" is taken by property "Manual property"`, apiErr.Issues[0].Message)
		assert.Equal(t, []v2model.Ref{update}, apiErr.Issues[0].SeeAlso, "the route takes the normalized slug")
	})

	t.Run("options declared on a property the batch creates offer only the resend", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		resend := v2model.Resend("create_missing_options", "true")
		want := v2model.Issue{
			Path:    "/ops/0/options",
			Message: `property "Sales/Stage" has no option named "Won", and this request did not ask to create one`,
			Hint:    "resend with " + resend.String() + " to create it",
			SeeAlso: []v2model.Ref{resend},
		}

		// when: no consent
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
			opsBody(`{"op":"add_property","property":"Sales/Stage","format":"select","options":[{"name":"Won"}]}`), false, false)

		// then: no options route for a property that does not exist yet
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, want, apiErr.Issues[0])
	})

	t.Run("an unresolvable query source is described, not quoted", func(t *testing.T) {
		// given: the query reads from a type that is gone
		fx := newV2Fixture(t)
		read := apicore.ObjectRead{Snapshot: &model.SmartBlockSnapshotBase{Details: &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeySetOf.String(): pbtypes.StringList([]string{"ot-6a7663db61fab21cd4b9e3c1"}),
		}}}}

		// when
		_, err := fx.querySourceFilters(testSpaceId, "query1", read)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, `query "query1" reads from a type or property that no longer exists, so it has no rows to list`, apiErr.Message)
		assertNoStoredKey(t, apiErr)
		assert.NotContains(t, apiErr.Message, "ot-")
	})
}
