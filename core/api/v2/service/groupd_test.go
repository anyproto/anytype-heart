package v2service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/any-block/codec/anyblockjson"
	"github.com/gogo/protobuf/types"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// Round-two eval F11, F16, F18, F19, F20: writes say what they did and
// what they could not; reads do not narrate the export's bookkeeping.

const editChoreDoc = `{"formatVersion":"2.0","id":"obj1","type":"chore","properties":{"name":"Doc","severity":["High"]}}`

func TestV2FavoriteFlag(t *testing.T) {
	ctx := context.Background()
	favoriteOk := &pb.RpcObjectListSetIsFavoriteResponse{Error: &pb.RpcObjectListSetIsFavoriteResponseError{Code: pb.RpcObjectListSetIsFavoriteResponseError_NULL}}

	t.Run("set is_favorite runs the favorite RPC after the edit commits, not a detail write", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")
		fx.mwMock.EXPECT().ObjectListSetIsFavorite(mock.Anything, &pb.RpcObjectListSetIsFavoriteRequest{
			ObjectIds: []string{"obj1"}, IsFavorite: true,
		}).Return(favoriteOk).Once()

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"is_favorite":true}}`), "", false, true)

		require.NoError(t, err)
		assert.Equal(t, 1, result.DiffStats.PropertiesChanged, "the receipt counts the transition")
		assert.True(t, (*captured).CombinedDetails().GetBool(bundle.RelationKeyIsFavorite),
			"the local detail is written too, so a read that follows sees the flag before the home reconcile")
	})

	t.Run("setting the flag an object already has, or clearing one it lacks, runs no RPC", func(t *testing.T) {
		// the RPC removes a home link and fails on an absent one: the
		// transition is checked against the object's current state first
		fx := newV2Fixture(t)
		favorited := editRead(t, `{"formatVersion":"2.0","id":"obj1","type":"page","properties":{"name":"Doc","is_favorite":true}}`)
		// the live state carries the local detail; NewDocFromSnapshot strips it
		fx.expectMutateState(favorited, func(st *state.State) {
			st.SetLocalDetail(bundle.RelationKeyIsFavorite, domain.Bool(true))
		}, "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"is_favorite":true}}`), "", false, true)

		require.NoError(t, err)
		assert.Equal(t, 0, result.DiffStats.PropertiesChanged, "no transition, nothing changed")

		fx2 := newV2Fixture(t)
		fx2.expectMutate(editRead(t, editBaseDoc), "headB")
		_, err = fx2.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","unset":["is_favorite"]}`), "", false, true)
		require.NoError(t, err, "clearing a clear flag is a no-op, not an error after the commit")
	})

	t.Run("unset is_favorite runs the RPC with false", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, `{"formatVersion":"2.0","id":"obj1","type":"page","properties":{"name":"Doc","is_favorite":true}}`), "headB")
		fx.mwMock.EXPECT().ObjectListSetIsFavorite(mock.Anything, &pb.RpcObjectListSetIsFavoriteRequest{
			ObjectIds: []string{"obj1"}, IsFavorite: false,
		}).Return(favoriteOk).Once()

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","unset":["is_favorite"]}`), "", false, true)

		require.NoError(t, err)
	})

	t.Run("a dry run does not favorite, and its receipt matches the real run's", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(editRead(t, editBaseDoc), nil)

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"is_favorite":true}}`), "", true, true)

		require.NoError(t, err)
		assert.True(t, result.DryRun)
		assert.Equal(t, 1, result.DiffStats.PropertiesChanged)
	})

	t.Run("a non-boolean flag is refused at the field", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"is_favorite":"yes"}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "ops[0].set.is_favorite", apiErr.Issues[0].Path)
	})

	t.Run("an RPC failure is an error, not a silent 200", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc), "headB")
		fx.mwMock.EXPECT().ObjectListSetIsFavorite(mock.Anything, mock.Anything).Return(&pb.RpcObjectListSetIsFavoriteResponse{
			Error: &pb.RpcObjectListSetIsFavoriteResponseError{Code: pb.RpcObjectListSetIsFavoriteResponseError_UNKNOWN_ERROR, Description: "home is read-only"},
		}).Once()

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"is_favorite":true}}`), "", false, true)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "home is read-only")
		assert.Contains(t, err.Error(), "was committed", "the caller learns the edit landed and the flag did not")
	})
}

// newOffTypeFixture is the F16 setting: "chore" and the space-minted
// "errand" (stored key a bson id) both list severity; spiciness and labels
// are on no type.
func newOffTypeFixture(t *testing.T) *v2Fixture {
	fx := newV2Fixture(t)
	fx.addSelectProperty(t)
	fx.addTaskType(t) // "chore" recommends severity
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("rel-spice"),
		bundle.RelationKeyRelationKey:  domain.String("6a8f2c1d9e4b7a3f5c2d8e77"),
		bundle.RelationKeyApiObjectKey: domain.String("spiciness"),
		bundle.RelationKeyName:         domain.String("Spiciness"),
	})
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:             domain.String("rel-labels"),
		bundle.RelationKeyRelationKey:    domain.String("6a8f2c1d9e4b7a3f5c2d8e78"),
		bundle.RelationKeyApiObjectKey:   domain.String("labels"),
		bundle.RelationKeyName:           domain.String("Labels"),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
	})
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String("opt-mild"),
		bundle.RelationKeyRelationKey:    domain.String("6a8f2c1d9e4b7a3f5c2d8e78"),
		bundle.RelationKeyName:           domain.String("mild"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
	}})
	// a space-minted type: stored key a bson id, served as "errand"
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:                   domain.String("type-errand"),
		bundle.RelationKeyUniqueKey:            domain.String("ot-6a8f2c1d9e4b7a3f5c2d8e88"),
		bundle.RelationKeyApiObjectKey:         domain.String("errand"),
		bundle.RelationKeyName:                 domain.String("Errand"),
		bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-severity"}),
	})
	return fx
}

func TestV2WritesSayWhatTheyLeaveInvisible(t *testing.T) {
	ctx := context.Background()
	setup := newOffTypeFixture

	t.Run("a value on a property the type does not list is stored with a warning naming the repair", func(t *testing.T) {
		fx := setup(t)
		captured := fx.expectMutate(editRead(t, editChoreDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"spiciness":"mild"}}`), "", false, true)

		require.NoError(t, err)
		assert.Equal(t, "mild", (*captured).CombinedDetails().GetString(domain.RelationKey("6a8f2c1d9e4b7a3f5c2d8e77")), "stored")
		require.Len(t, result.Warnings, 1)
		assert.Equal(t, "ops[0].set.spiciness", result.Warnings[0].Path)
		assert.Contains(t, result.Warnings[0].Message, `property "spiciness" is not on type "chore"`)
		assert.Contains(t, result.Warnings[0].Message, "type-scoped search")
		assert.Equal(t, []v2model.Ref{v2model.RefGetOpSchema("add_property")}, result.Warnings[0].SeeAlso)
	})

	t.Run("the warning fires for a space-minted type served by its slug, and for add", func(t *testing.T) {
		fx := setup(t)
		// the object stores errand by its stored key, as a real one does; the
		// warning still names it by its slug
		fx.expectMutate(editRead(t, `{"formatVersion":"2.0","id":"obj1","type":"6a8f2c1d9e4b7a3f5c2d8e88","properties":{"name":"Doc"}}`), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","add":{"labels":["mild"]}}`), "", false, true)

		require.NoError(t, err)
		require.Len(t, result.Warnings, 1, "%v", result.Warnings)
		assert.Equal(t, "ops[0].add.labels", result.Warnings[0].Path)
		assert.Contains(t, result.Warnings[0].Message, `is not on type "errand"`)
	})

	t.Run("a create carrying such a property warns the same way", func(t *testing.T) {
		fx := setup(t)
		fx.expectCreate("new1")
		fx.expectEtagRead("new1")

		result, err := fx.CreateObject(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"chore","properties":{"name":"X","spiciness":"mild"}}`), false, true)

		require.NoError(t, err)
		require.Len(t, result.Warnings, 1, "%v", result.Warnings)
		assert.Equal(t, "/properties/spiciness", result.Warnings[0].Path)
		assert.Equal(t, []v2model.Ref{v2model.RefGetOpSchema("add_property")}, result.Warnings[0].SeeAlso)
	})

	t.Run("a listed property, a hidden system one, and one the object already carries warn nothing", func(t *testing.T) {
		fx := setup(t)
		fx.expectMutate(editRead(t, `{"formatVersion":"2.0","id":"obj1","type":"chore","properties":{"name":"Doc","spiciness":"hot"}}`), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"severity":["High"],"name":"Renamed","spiciness":"mild"}}`), "", false, true)

		require.NoError(t, err)
		assert.Empty(t, result.Warnings)
	})

	t.Run("a query with no filter says it lists the whole type", func(t *testing.T) {
		fx := setup(t)
		fx.expectCreate("q1")
		fx.expectEtagRead("q1")

		result, err := fx.CreateQuery(ctx, testSpaceId, v2model.CreateQueryRequest{Name: "Chores needing attention", Type: "chore"}, false, true)

		require.NoError(t, err)
		require.Len(t, result.Warnings, 1)
		assert.Equal(t, "/filter", result.Warnings[0].Path)
		assert.Contains(t, result.Warnings[0].Message, `lists every live object of type "chore"`)
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("filters")}, result.Warnings[0].SeeAlso)
	})

	t.Run("an explicit empty filters array is no filter either, and views point the repair at themselves", func(t *testing.T) {
		fx := setup(t)
		fx.expectCreate("q1")
		fx.expectEtagRead("q1")
		result, err := fx.CreateQuery(ctx, testSpaceId, v2model.CreateQueryRequest{Name: "Q", Type: "chore", Filters: json.RawMessage(`[]`)}, false, true)
		require.NoError(t, err)
		require.Len(t, result.Warnings, 1)

		fx2 := setup(t)
		fx2.expectCreate("q2")
		fx2.expectEtagRead("q2")
		result, err = fx2.CreateQuery(ctx, testSpaceId, v2model.CreateQueryRequest{Name: "Q", Type: "chore", Views: json.RawMessage(`[{"name":"All"}]`)}, false, true)
		require.NoError(t, err)
		require.Len(t, result.Warnings, 1)
		assert.Equal(t, "/views", result.Warnings[0].Path)
		assert.Contains(t, result.Warnings[0].Hint, "give a view filters")
	})

	t.Run("a filtered query, by string or by view, warns nothing", func(t *testing.T) {
		for name, req := range map[string]v2model.CreateQueryRequest{
			"string": {Name: "Q", Type: "chore", Filter: `severity = "High"`},
			"view":   {Name: "Q", Type: "chore", Views: json.RawMessage(`[{"name":"V","filters":[{"property":"severity","condition":"equal","value":["High"]}]}]`)},
		} {
			t.Run(name, func(t *testing.T) {
				fx := setup(t)
				fx.expectCreate("q1")
				fx.expectEtagRead("q1")

				result, err := fx.CreateQuery(ctx, testSpaceId, req, false, true)

				require.NoError(t, err)
				assert.Empty(t, result.Warnings)
			})
		}
	})

	t.Run("a view-key refusal spells the served key, never the stored one", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.CreateQuery(ctx, testSpaceId, v2model.CreateQueryRequest{Name: "Q", Type: "chore",
			Sorts: json.RawMessage(`[{"property":"spiciness"}]`)}, true, true)

		issue := issueAt(t, err, "/sorts/0/property")
		assert.Contains(t, issue.Message, `type "chore" has no property "spiciness"`)
		assert.NotContains(t, issue.Message, "6a8f2c1d9e4b7a3f5c2d8e77")
		assert.NotContains(t, issue.Hint, "6a8f2c1d9e4b7a3f5c2d8e77")
	})
}

// APIV2-035: a template's values are what objects of its target type start
// with, so the F16 warning judges them against template_for, never against
// the type `template`, and names that target.
func TestV2TemplatePropertiesJudgedAgainstTarget(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) *v2Fixture {
		fx := newOffTypeFixture(t)
		// the space's template type lists none of chore's properties, so a
		// template judged against it warns on every one
		fx.addTemplateType(t)
		return fx
	}
	const choreTemplateDoc = `{"formatVersion":"2.0","id":"obj1","kind":"template","type":"template","template_for":"chore","properties":{"name":"Weekly"}}`
	spicinessOffChore := func(path string) v2model.Issue {
		return v2model.Issue{
			Path:    path,
			Message: `property "spiciness" is not on type "chore", the type this template is for. Objects created from the template take the value. That type's search and queries refuse the key. Its default columns omit it.`,
			SeeAlso: []v2model.Ref{v2model.RefGetOpSchema("add_property")},
		}
	}
	assertWarning := func(t *testing.T, want v2model.Issue, got []v2model.Issue) {
		t.Helper()
		require.Len(t, got, 1, "%v", got)
		assert.Equal(t, want.Path, got[0].Path)
		assert.Equal(t, want.Message, got[0].Message)
		assert.Equal(t, want.SeeAlso, got[0].SeeAlso)
		assert.Contains(t, got[0].Hint, `list it on type "chore"`, "the repair names the type to add it to, not the template's own")
	}

	t.Run("a template create setting its target's properties warns nothing", func(t *testing.T) {
		// given
		fx := setup(t)
		fx.expectCreate("tpl1")
		fx.expectEtagRead("tpl1")

		// when
		result, err := fx.CreateTemplate(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","template_for":"chore","properties":{"name":"Weekly","severity":["High"]}}`), false, true)

		// then
		require.NoError(t, err)
		assert.Empty(t, result.Warnings)
	})

	t.Run("a template create setting a property its target does not list warns, naming the target", func(t *testing.T) {
		// given
		fx := setup(t)
		fx.expectCreate("tpl1")
		fx.expectEtagRead("tpl1")
		want := spicinessOffChore("/properties/spiciness")

		// when
		result, err := fx.CreateTemplate(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","template_for":"chore","properties":{"name":"Weekly","severity":["High"],"spiciness":"mild"}}`), false, true)

		// then
		require.NoError(t, err)
		assertWarning(t, want, result.Warnings)
	})

	t.Run("a template for a space-minted type is judged against it and names it by its slug, on a dry run too", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		result, err := fx.CreateTemplate(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","template_for":"errand","properties":{"name":"Weekly","severity":["High"],"spiciness":"mild"}}`), true, true)

		// then
		require.NoError(t, err)
		require.Len(t, result.Warnings, 1, "severity is on errand; %v", result.Warnings)
		assert.Equal(t, "/properties/spiciness", result.Warnings[0].Path)
		assert.Contains(t, result.Warnings[0].Message, `is not on type "errand", the type this template is for`)
		assert.Contains(t, result.Warnings[0].Message, "Objects created from the template would take the value", "a preview says what the write would do")
		assert.NotContains(t, result.Warnings[0].Message, "6a8f2c1d9e4b7a3f5c2d8e88")
	})

	t.Run("under ?keys=name a template create names its target by display name", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		result, err := fx.CreateTemplate(CtxWithNameKeys(ctx), testSpaceId,
			[]byte(`{"formatVersion":"2.0","template_for":"errand","properties":{"name":"Weekly","spiciness":"mild"}}`), true, true)

		// then
		require.NoError(t, err)
		require.Len(t, result.Warnings, 1, "%v", result.Warnings)
		assert.Contains(t, result.Warnings[0].Message, `is not on type "Errand", the type this template is for`)
		assert.Contains(t, result.Warnings[0].Hint, `list it on type "Errand"`)
		assert.NotContains(t, result.Warnings[0].Message, `"errand"`)
	})

	t.Run("under ?keys=name set_properties on a template names its target by display name", func(t *testing.T) {
		// given
		fx := setup(t)
		fx.expectMutate(editRead(t, `{"formatVersion":"2.0","id":"obj1","kind":"template","type":"template","template_for":"6a8f2c1d9e4b7a3f5c2d8e88","properties":{"name":"Weekly"}}`), "headB")

		// when
		result, err := fx.PatchObject(CtxWithNameKeys(ctx), testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"spiciness":"mild"}}`), "", false, true)

		// then
		require.NoError(t, err)
		require.Len(t, result.Warnings, 1, "%v", result.Warnings)
		assert.Contains(t, result.Warnings[0].Message, `is not on type "Errand", the type this template is for`)
		assert.Contains(t, result.Warnings[0].Hint, `list it on type "Errand"`)
	})

	t.Run("a target whose slug collides is named by display name, never by its stored key", func(t *testing.T) {
		// given: a second live type also serves the slug "errand", so the
		// served spelling of either falls back to its stored key
		fx := setup(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("type-errand-twin"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-6a8f2c1d9e4b7a3f5c2d8e89"),
			bundle.RelationKeyApiObjectKey: domain.String("errand"),
			bundle.RelationKeyName:         domain.String("Errand twin"),
		})
		fx.expectMutate(editRead(t, `{"formatVersion":"2.0","id":"obj1","kind":"template","type":"template","template_for":"6a8f2c1d9e4b7a3f5c2d8e88","properties":{"name":"Weekly"}}`), "headB")

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"spiciness":"mild"}}`), "", false, true)

		// then
		require.NoError(t, err)
		require.Len(t, result.Warnings, 1, "%v", result.Warnings)
		assert.Contains(t, result.Warnings[0].Message, `is not on type "Errand", the type this template is for`)
		assert.NotContains(t, result.Warnings[0].Message, "6a8f2c1d9e4b7a3f5c2d8e88")
		assert.NotContains(t, result.Warnings[0].Hint, "6a8f2c1d9e4b7a3f5c2d8e88")
	})

	t.Run("a target with no minted slug is named by display name, never by its stored key", func(t *testing.T) {
		// given: a space-minted type whose row carries no api slug
		fx := setup(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:        domain.String("type-dispatch"),
			bundle.RelationKeyUniqueKey: domain.String("ot-6a8f2c1d9e4b7a3f5c2d8e87"),
			bundle.RelationKeyName:      domain.String("Dispatch"),
		})
		fx.expectMutate(editRead(t, `{"formatVersion":"2.0","id":"obj1","kind":"template","type":"template","template_for":"6a8f2c1d9e4b7a3f5c2d8e87","properties":{"name":"Weekly"}}`), "headB")

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"spiciness":"mild"}}`), "", false, true)

		// then
		require.NoError(t, err)
		require.Len(t, result.Warnings, 1, "%v", result.Warnings)
		assert.Contains(t, result.Warnings[0].Message, `is not on type "Dispatch", the type this template is for`)
		assert.NotContains(t, result.Warnings[0].Message, "6a8f2c1d9e4b7a3f5c2d8e87")
		assert.NotContains(t, result.Warnings[0].Hint, "6a8f2c1d9e4b7a3f5c2d8e87")
	})

	t.Run("two types answering to one template_for are listed by stored key", func(t *testing.T) {
		// given: the store's id order is the reverse of the key order
		fx := setup(t)
		for _, twin := range []struct{ id, key string }{{"type-z-first", "ot-aaa"}, {"type-a-second", "ot-zzz"}} {
			fx.addType(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:        domain.String(twin.id),
				bundle.RelationKeyUniqueKey: domain.String(twin.key),
				bundle.RelationKeyName:      domain.String("Rota"),
			})
		}

		// when
		_, err := fx.CreateTemplate(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","template_for":"Rota","properties":{"name":"Weekly"}}`), true, true)

		// then
		issue := issueAt(t, err, "/template_for")
		require.Contains(t, issue.Message, "key aaa")
		require.Contains(t, issue.Message, "key zzz")
		assert.Less(t, strings.Index(issue.Message, "key aaa"), strings.Index(issue.Message, "key zzz"))
	})

	t.Run("a plain create preview says the value would be stored", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		result, err := fx.CreateObject(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"chore","properties":{"name":"X","spiciness":"mild"}}`), true, true)

		// then
		require.NoError(t, err)
		require.Len(t, result.Warnings, 1, "%v", result.Warnings)
		assert.Contains(t, result.Warnings[0].Message, `is not on type "chore" — the value would be stored and served on the object`)
	})

	t.Run("a template for a type this space cannot resolve warns nothing", func(t *testing.T) {
		// given: task is bundled, so it passes the type gate, but this space
		// holds no row for it and so no list to judge against
		fx := setup(t)
		fx.expectCreate("tpl1")
		fx.expectEtagRead("tpl1")

		// when
		result, err := fx.CreateTemplate(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","template_for":"task","properties":{"name":"Weekly","spiciness":"mild"}}`), false, true)

		// then
		require.NoError(t, err)
		assert.Empty(t, result.Warnings)
	})

	t.Run("a template's own system property warns nothing", func(t *testing.T) {
		// given: the name-prefill setting belongs to templates, and no target
		// type lists it
		fx := setup(t)

		// when
		result, err := fx.CreateTemplate(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","template_for":"chore","properties":{"name":"Weekly","template_name_prefill_type":1}}`), true, true)

		// then
		require.NoError(t, err)
		assert.Empty(t, result.Warnings)
	})

	t.Run("set_properties on a template setting its target's properties warns nothing", func(t *testing.T) {
		// given
		fx := setup(t)
		fx.expectMutate(editRead(t, choreTemplateDoc), "headB")

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"severity":["High"]}}`), "", false, true)

		// then
		require.NoError(t, err)
		assert.Empty(t, result.Warnings)
	})

	t.Run("set_properties on a template setting a property its target does not list warns, naming the target", func(t *testing.T) {
		// given
		fx := setup(t)
		fx.expectMutate(editRead(t, choreTemplateDoc), "headB")
		want := spicinessOffChore("ops[0].set.spiciness")

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"severity":["High"],"spiciness":"mild"}}`), "", false, true)

		// then
		require.NoError(t, err)
		assertWarning(t, want, result.Warnings)
	})

	t.Run("set_properties on a template of a space-minted type names the target by its slug", func(t *testing.T) {
		// given: the template's target is stored by errand's bson key, which
		// the read serves as the slug
		fx := setup(t)
		fx.expectMutate(editRead(t, `{"formatVersion":"2.0","id":"obj1","kind":"template","type":"template","template_for":"6a8f2c1d9e4b7a3f5c2d8e88","properties":{"name":"Weekly"}}`), "headB")

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"severity":["High"],"spiciness":"mild"}}`), "", false, true)

		// then
		require.NoError(t, err)
		require.Len(t, result.Warnings, 1, "severity is on errand; %v", result.Warnings)
		assert.Equal(t, "ops[0].set.spiciness", result.Warnings[0].Path)
		assert.Contains(t, result.Warnings[0].Message, `is not on type "errand", the type this template is for`)
		assert.NotContains(t, result.Warnings[0].Message, "6a8f2c1d9e4b7a3f5c2d8e88")
	})

	t.Run("set_properties on a template whose target is not known warns nothing", func(t *testing.T) {
		// given: no template_for, so nothing says which type the values are for
		fx := setup(t)
		fx.expectMutate(editRead(t, `{"formatVersion":"2.0","id":"obj1","kind":"template","type":"template","properties":{"name":"Weekly"}}`), "headB")

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"spiciness":"mild"}}`), "", false, true)

		// then
		require.NoError(t, err)
		assert.Empty(t, result.Warnings)
	})

	t.Run("a plain create keeps the plain warning", func(t *testing.T) {
		// given
		fx := setup(t)
		fx.expectCreate("new1")
		fx.expectEtagRead("new1")
		want := v2model.Issue{
			Path:    "/properties/spiciness",
			Message: `property "spiciness" is not on type "chore" — the value is stored and served on the object, but type-scoped search and queries over "chore" refuse the key and the type's default columns omit it`,
			Hint:    "list it on the type with the add_property op (GET /v2/schemas/ops/add_property)",
			SeeAlso: []v2model.Ref{v2model.RefGetOpSchema("add_property")},
		}

		// when
		result, err := fx.CreateObject(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"chore","properties":{"name":"X","spiciness":"mild"}}`), false, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, []v2model.Issue{want}, result.Warnings)
	})
}

// GO-7614: a property name two properties answer to is settled by the type
// that lists one of them (resolvePropertyInputIn). A template's own type
// lists neither, so a template is scoped by the type it is for, by the rule
// offTypeJudge applies.
// The "Effort" setting: two properties share the display name. Sprint lists
// the points one; the template type lists neither.
const (
	effortPointsKey   = "6a8f2c1d9e4b7a3f5c2d8e90"
	effortHoursKey    = "6a8f2c1d9e4b7a3f5c2d8e91"
	sprintTemplateDoc = `{"formatVersion":"2.0","id":"obj1","kind":"template","type":"template","template_for":"sprint","properties":{"name":"Weekly"}}`
)

// newEffortFixture registers the template type and both Effort properties,
// but no sprint type: addSprintType adds it, in the shape a case needs.
func newEffortFixture(t *testing.T) *v2Fixture {
	fx := newV2Fixture(t)
	fx.addTemplateType(t)
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("rel-effort-points"),
		bundle.RelationKeyRelationKey:  domain.String(effortPointsKey),
		bundle.RelationKeyApiObjectKey: domain.String("effort_points"),
		bundle.RelationKeyName:         domain.String("Effort"),
	})
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("rel-effort-hours"),
		bundle.RelationKeyRelationKey:  domain.String(effortHoursKey),
		bundle.RelationKeyApiObjectKey: domain.String("effort_hours"),
		bundle.RelationKeyName:         domain.String("Effort"),
	})
	return fx
}

// addSprintType registers sprint (stored key "sprint", id type-sprint),
// listing the points Effort, with extra details on top.
func (fx *v2Fixture) addSprintType(t *testing.T, extra ...objectstore.TestObject) {
	row := objectstore.TestObject{
		bundle.RelationKeyId:                   domain.String("type-sprint"),
		bundle.RelationKeyUniqueKey:            domain.String("ot-sprint"),
		bundle.RelationKeyName:                 domain.String("Sprint"),
		bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-effort-points"}),
	}
	for _, more := range extra {
		for key, value := range more {
			row[key] = value
		}
	}
	fx.addType(t, testSpaceId, row)
}

// templateRead is the read a template comes back as: its type detail is the
// template type's id, and its target rides as the second type key.
func templateRead(t *testing.T, doc string) apicore.ObjectRead {
	read := editRead(t, doc)
	read.Snapshot.Details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(testTemplateTypeId)
	return read
}

func TestV2TemplateNamesResolveAgainstTarget(t *testing.T) {
	const (
		pointsKey = effortPointsKey
		hoursKey  = effortHoursKey
	)
	ctx := context.Background()
	setup := func(t *testing.T) *v2Fixture {
		fx := newEffortFixture(t)
		fx.addSprintType(t)
		return fx
	}

	t.Run("a template create writes the property its target lists", func(t *testing.T) {
		// given
		fx := setup(t)
		captured := fx.expectCreate("tpl1")
		fx.expectEtagRead("tpl1")

		// when
		_, err := fx.CreateTemplate(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","template_for":"sprint","properties":{"name":"Weekly","Effort":"3"}}`), false, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, *captured)
		details := (*captured).Details
		assert.Equal(t, "3", pbtypes.GetString(details, pointsKey))
		assert.False(t, pbtypes.HasField(details, hoursKey))
	})

	t.Run("under ?keys=name a template for a type named only by its display name resolves against it", func(t *testing.T) {
		// given: sprint's display name is not any spelling of its key
		fx := newEffortFixture(t)
		fx.addSprintType(t, objectstore.TestObject{bundle.RelationKeyName: domain.String("Iteration")})
		captured := fx.expectCreate("tpl1")
		fx.expectEtagRead("tpl1")

		// when
		_, err := fx.CreateTemplate(CtxWithNameKeys(ctx), testSpaceId,
			[]byte(`{"formatVersion":"2.0","template_for":"Iteration","properties":{"name":"Weekly","Effort":"3"}}`), false, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, *captured)
		assert.Equal(t, []string{"ot-template", "ot-sprint"}, (*captured).ObjectTypes)
		assert.Equal(t, "3", pbtypes.GetString((*captured).Details, pointsKey))
	})

	t.Run("a template create's property block binds the property its target lists", func(t *testing.T) {
		// given
		fx := setup(t)
		captured := fx.expectCreate("tpl1")
		fx.expectEtagRead("tpl1")

		// when
		_, err := fx.CreateTemplate(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","template_for":"sprint","properties":{"name":"Weekly"},"blocks":[{"type":"property","property":"Effort"}]}`), false, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, *captured)
		var keys []string
		for _, b := range (*captured).Blocks {
			if rel := b.GetRelation(); rel != nil {
				keys = append(keys, rel.Key)
			}
		}
		assert.Equal(t, []string{pointsKey}, keys)
	})

	t.Run("set_properties on a template writes the property its target lists", func(t *testing.T) {
		// given
		fx := setup(t)
		captured := fx.expectMutate(templateRead(t, sprintTemplateDoc), "headB")

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"Effort":"3"}}`), "", false, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, *captured)
		details := (*captured).CombinedDetails()
		assert.Equal(t, "3", details.GetString(domain.RelationKey(pointsKey)))
		assert.False(t, details.Has(domain.RelationKey(hoursKey)))
	})

	t.Run("a template whose target is not known keeps the name ambiguous", func(t *testing.T) {
		// given: no template_for, so the scope stays the template's own type
		fx := setup(t)
		fx.expectMutate(templateRead(t, `{"formatVersion":"2.0","id":"obj1","kind":"template","type":"template","properties":{"name":"Weekly"}}`))

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"Effort":"3"}}`), "", false, true)

		// then
		assert.Equal(t, v2model.CodeAmbiguousInput, v2Err(t, err).Code)
		issue := issueAt(t, err, "ops[0].set.Effort")
		require.Contains(t, issue.Message, pointsKey)
		require.Contains(t, issue.Message, hoursKey)
		assert.Less(t, strings.Index(issue.Message, pointsKey), strings.Index(issue.Message, hoursKey),
			"the holders are listed by stored key, not by store order")
	})
}

// GO-7614: `kind: "template"` names a template, whose type is `template`. On
// another type the create would resolve property names against the target
// and then build an object of that other type, so the generic create holds
// the same line POST /templates does, before any property term resolves.
// GO-7614: one identity for a template's target. targetObjectType decides,
// the second type key answers only when the detail is absent, and only a
// LIVE row found by its exact stored key decides a name's scope or an
// off-type warning. A read names the same target.
func TestV2TemplateTargetIdentity(t *testing.T) {
	ctx := context.Background()
	// a template stored with typeKeys and, when targetId is set, the
	// targetObjectType detail
	storedTemplate := func(t *testing.T, targetId string, typeKeys ...string) apicore.ObjectRead {
		read := templateRead(t, `{"formatVersion":"2.0","id":"obj1","kind":"template","type":"template","properties":{"name":"Weekly"}}`)
		read.Snapshot.ObjectTypes = typeKeys
		if targetId != "" {
			read.Snapshot.Details.Fields[bundle.RelationKeyTargetObjectType.String()] = pbtypes.String(targetId)
		}
		return read
	}
	// backlog lists the hours Effort: a template retargeted from backlog to
	// sprint still carries backlog as its second type key
	addBacklog := func(t *testing.T, fx *v2Fixture) {
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String("type-backlog"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-backlog"),
			bundle.RelationKeyName:                 domain.String("Backlog"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-effort-hours"}),
		})
	}
	setEffort := func(t *testing.T, fx *v2Fixture, read apicore.ObjectRead, op string) (*v2model.EditResult, **state.State, error) {
		t.Helper()
		captured := fx.expectMutate(read, "headB")
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(op), "", false, true)
		return result, captured, err
	}
	templateFor := func(t *testing.T, fx *v2Fixture, read apicore.ObjectRead) string {
		t.Helper()
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil).Maybe()
		body, _, err := fx.GetObject(ctx, testSpaceId, "obj1", ObjectQuery{})
		require.NoError(t, err)
		var doc struct {
			TemplateFor string `json:"template_for"`
		}
		require.NoError(t, json.Unmarshal(body, &doc))
		return doc.TemplateFor
	}

	t.Run("a retargeted template resolves names against targetObjectType, not the second type key", func(t *testing.T) {
		// given
		fx := newEffortFixture(t)
		fx.addSprintType(t)
		addBacklog(t, fx)

		// when
		_, captured, err := setEffort(t, fx, storedTemplate(t, "type-sprint", "ot-template", "ot-backlog"),
			`{"op":"set_properties","set":{"Effort":"3"}}`)

		// then
		require.NoError(t, err)
		details := (*captured).CombinedDetails()
		assert.Equal(t, "3", details.GetString(domain.RelationKey(effortPointsKey)))
		assert.False(t, details.Has(domain.RelationKey(effortHoursKey)))
	})

	t.Run("a retargeted template is judged and named by targetObjectType", func(t *testing.T) {
		// given
		fx := newEffortFixture(t)
		fx.addSprintType(t)
		addBacklog(t, fx)

		// when: hours is on the old target, not on sprint
		result, _, err := setEffort(t, fx, storedTemplate(t, "type-sprint", "ot-template", "ot-backlog"),
			`{"op":"set_properties","set":{"effort_hours":"3"}}`)

		// then
		require.NoError(t, err)
		require.Len(t, result.Warnings, 1, "%v", result.Warnings)
		assert.Contains(t, result.Warnings[0].Message, `is not on type "sprint"`)
	})

	t.Run("a retargeted template reads back naming targetObjectType", func(t *testing.T) {
		// given
		fx := newEffortFixture(t)
		fx.addSprintType(t)
		addBacklog(t, fx)

		// when
		got := templateFor(t, fx, storedTemplate(t, "type-sprint", "ot-template", "ot-backlog"))

		// then
		assert.Equal(t, "sprint", got)
	})

	t.Run("a template whose target rides only in targetObjectType resolves, warns and reads by it", func(t *testing.T) {
		// given
		fx := newEffortFixture(t)
		fx.addSprintType(t)
		read := storedTemplate(t, "type-sprint", "ot-template")

		// when
		result, captured, err := setEffort(t, fx, read, `{"op":"set_properties","set":{"Effort":"3","effort_hours":"4"}}`)

		// then
		require.NoError(t, err)
		assert.Equal(t, "3", (*captured).CombinedDetails().GetString(domain.RelationKey(effortPointsKey)))
		require.Len(t, result.Warnings, 1, "%v", result.Warnings)
		assert.Equal(t, "ops[0].set.effort_hours", result.Warnings[0].Path)
		assert.Equal(t, "sprint", templateFor(t, fx, read))
	})

	t.Run("an uninstalled target decides nothing: the template's own type settles names and nothing warns", func(t *testing.T) {
		// given: the template type lists the hours Effort, the uninstalled
		// sprint row the points one
		fx := newEffortFixture(t)
		fx.addSprintType(t, objectstore.TestObject{bundle.RelationKeyIsUninstalled: domain.Bool(true)})
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String(testTemplateTypeId),
			bundle.RelationKeyName:                 domain.String("Template"),
			bundle.RelationKeyUniqueKey:            domain.String(bundle.TypeKeyTemplate.URL()),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-effort-hours"}),
		})

		// when
		result, captured, err := setEffort(t, fx, templateRead(t, sprintTemplateDoc), `{"op":"set_properties","set":{"Effort":"3"}}`)

		// then
		require.NoError(t, err)
		details := (*captured).CombinedDetails()
		assert.Equal(t, "3", details.GetString(domain.RelationKey(effortHoursKey)), "the template type's own holder wins")
		assert.False(t, details.Has(domain.RelationKey(effortPointsKey)))
		assert.Empty(t, result.Warnings)
	})

	t.Run("a missing target is never matched to another type that answers to its key", func(t *testing.T) {
		// given: no sprint row; another live type is served under the slug
		// "sprint" and lists nothing
		fx := newEffortFixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("type-squatter"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-6a8f2c1d9e4b7a3f5c2d8e99"),
			bundle.RelationKeyApiObjectKey: domain.String("sprint"),
			bundle.RelationKeyName:         domain.String("Squatter"),
		})

		// when
		result, _, err := setEffort(t, fx, templateRead(t, sprintTemplateDoc), `{"op":"set_properties","set":{"effort_hours":"3"}}`)

		// then
		require.NoError(t, err)
		assert.Empty(t, result.Warnings)
	})

	t.Run("an unknown target keeps the template type's own holder as the winner", func(t *testing.T) {
		// given: nothing names a target; the template type lists one holder
		fx := newEffortFixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String(testTemplateTypeId),
			bundle.RelationKeyName:                 domain.String("Template"),
			bundle.RelationKeyUniqueKey:            domain.String(bundle.TypeKeyTemplate.URL()),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-effort-hours"}),
		})

		// when
		_, captured, err := setEffort(t, fx, storedTemplate(t, "", "ot-template"), `{"op":"set_properties","set":{"Effort":"3"}}`)

		// then
		require.NoError(t, err)
		assert.Equal(t, "3", (*captured).CombinedDetails().GetString(domain.RelationKey(effortHoursKey)))
	})

	t.Run("a template's query_source property resolves against its target", func(t *testing.T) {
		// given: this slot reaches the importer's resolver as sent, so only the
		// create's resolver scope settles it
		fx := newEffortFixture(t)
		fx.addSprintType(t)
		captured := fx.expectCreate("tpl1")
		fx.expectEtagRead("tpl1")

		// when
		_, err := fx.CreateTemplate(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","template_for":"sprint","properties":{"name":"Weekly"},"query_source":{"properties":["Effort"]}}`), false, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, *captured)
		assert.Equal(t, []string{"rel-effort-points"}, pbtypes.GetStringList((*captured).Details, bundle.RelationKeySetOf.String()))
	})

	t.Run("a property block on a template binds the target's property, on a dry run and for real", func(t *testing.T) {
		// given
		fx := newEffortFixture(t)
		fx.addSprintType(t)
		read := templateRead(t, sprintTemplateDoc)
		op := `{"op":"insert_blocks","blocks":[{"type":"property","property":"Effort"}]}`
		relationKeys := func(st *state.State) []string {
			var keys []string
			st.Iterate(func(b simple.Block) bool {
				if rel := b.Model().GetRelation(); rel != nil {
					keys = append(keys, rel.Key)
				}
				return true
			})
			return keys
		}
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil).Once()

		// when: the dry run reaches no mutator
		preview, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(op), "", true, true)
		require.NoError(t, err)
		captured := fx.expectMutate(read, "headB")
		committed, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(op), "", false, true)

		// then
		require.NoError(t, err)
		assert.True(t, preview.DryRun)
		assert.Equal(t, committed.DiffStats, preview.DiffStats)
		assert.Equal(t, []string{effortPointsKey}, relationKeys(*captured))
		// and the state the dry run simulated binds the same property
		pc, err := fx.newPatchContext(testSpaceId, read)
		require.NoError(t, err)
		resolvers := fx.newCreatingResolvers(ctx, testSpaceId, true, true)
		resolvers.patch, resolvers.scope = pc, pc.objectScope
		edit, err := editFromRead("obj1", read)
		require.NoError(t, err)
		_, simulated, err := fx.applyPatchOps(ctx, testSpaceId, "obj1", []json.RawMessage{json.RawMessage(op)}, "", edit, resolvers, true)
		require.NoError(t, err)
		assert.Equal(t, []string{effortPointsKey}, relationKeys(simulated.st))
	})
}

// GO-7614: each key slot resolves in its own scope, in both passes of a
// PATCH. A view filter is the view's query source's; the object's own keys
// are its type's as it stands at that op, a set_type earlier in the batch
// included. The create-missing prewarm follows the same scopes in the same
// op order, so it never pre-creates an option on another property.
func TestV2PatchScopesFollowTheirSource(t *testing.T) {
	ctx := context.Background()
	const (
		stageBacklogKey  = "6a8f2c1d9e4b7a3f5c2d8ea1"
		stageSprintKey   = "6a8f2c1d9e4b7a3f5c2d8ea2"
		stagePipelineKey = "6a8f2c1d9e4b7a3f5c2d8ea3"
	)
	// three select properties named "Stage", one per type: backlog, sprint
	// (beside the points Effort) and pipeline; readyOn names the one that
	// holds an option "Ready"
	setup := func(t *testing.T, readyOn string) *v2Fixture {
		fx := newEffortFixture(t)
		for _, stage := range []struct{ id, key, slug string }{
			{"rel-stage-backlog", stageBacklogKey, "stage_backlog"},
			{"rel-stage-sprint", stageSprintKey, "stage_sprint"},
			{"rel-stage-pipeline", stagePipelineKey, "stage_pipeline"},
		} {
			fx.addRelation(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:             domain.String(stage.id),
				bundle.RelationKeyRelationKey:    domain.String(stage.key),
				bundle.RelationKeyApiObjectKey:   domain.String(stage.slug),
				bundle.RelationKeyName:           domain.String("Stage"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
			})
		}
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("opt-ready"),
			bundle.RelationKeyRelationKey:    domain.String(readyOn),
			bundle.RelationKeyName:           domain.String("Ready"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
		}})
		fx.addSprintType(t, objectstore.TestObject{
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-effort-points", "rel-stage-sprint"}),
		})
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String("type-backlog"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-backlog"),
			bundle.RelationKeyName:                 domain.String("Backlog"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-effort-hours", "rel-stage-backlog"}),
		})
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String("type-pipeline"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-pipeline"),
			bundle.RelationKeyName:                 domain.String("Pipeline"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-stage-pipeline"}),
		})
		return fx
	}
	// a template for sprint holding a view of pipeline objects
	viewTemplate := func(t *testing.T) apicore.ObjectRead {
		read := templateRead(t, `{"formatVersion":"2.0","id":"obj1","kind":"template","type":"template","template_for":"sprint","properties":{"name":"Weekly"},`+
			`"blocks":[{"id":"dataview","type":"dataview","views":[{"id":"view1","name":"All"}]}]}`)
		read.Snapshot.Details.Fields[bundle.RelationKeySetOf.String()] = pbtypes.StringList([]string{"type-pipeline"})
		return read
	}
	// an ordinary backlog object
	backlogRead := func(t *testing.T) apicore.ObjectRead {
		read := editRead(t, `{"formatVersion":"2.0","id":"obj1","type":"backlog","properties":{"name":"Doc"}}`)
		read.Snapshot.Details.Fields[bundle.RelationKeyType.String()] = pbtypes.String("type-backlog")
		return read
	}
	createdOptions := func(created *v2model.SideEffects) []v2model.CreatedOption {
		if created == nil {
			return nil
		}
		return created.Options
	}

	for _, consent := range []bool{false, true} {
		t.Run(fmt.Sprintf("a template's view filter resolves in the query source, creating nothing (create_missing_options=%v)", consent), func(t *testing.T) {
			// given: `status` is both the installed built-in's key and the
			// slug of the space's own Pipeline status. Sprint lists the
			// built-in, pipeline its own, and only the own one holds
			// Ready; the strict mock fails any option create
			const ownStatusKey = "6a8f2c1d9e4b7a3f5c2d8ea4"
			fx := setup(t, ownStatusKey)
			fx.addRelation(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:             domain.String("rel-status"),
				bundle.RelationKeyRelationKey:    domain.String("status"),
				bundle.RelationKeyName:           domain.String("Status"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
			})
			fx.addRelation(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:             domain.String("rel-own-status"),
				bundle.RelationKeyRelationKey:    domain.String(ownStatusKey),
				bundle.RelationKeyApiObjectKey:   domain.String("status"),
				bundle.RelationKeyName:           domain.String("Pipeline status"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
			})
			fx.addSprintType(t, objectstore.TestObject{
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-effort-points", "rel-status"}),
			})
			fx.addType(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:                   domain.String("type-pipeline"),
				bundle.RelationKeyUniqueKey:            domain.String("ot-pipeline"),
				bundle.RelationKeyName:                 domain.String("Pipeline"),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-own-status"}),
			})
			committed := fx.expectMutateState(viewTemplate(t), nil)

			// when
			result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
				patchBody(`{"op":"update_view","set":{"filters":[{"property":"status","condition":"equal","value":["Ready"]}]}}`), "", false, consent)

			// then
			require.NoError(t, err)
			assert.Empty(t, createdOptions(result.Created))
			dv := (*committed).Pick(state.DataviewBlockID).Model().GetDataview()
			require.Len(t, dv.Views, 1)
			require.Len(t, dv.Views[0].Filters, 1)
			assert.Equal(t, ownStatusKey, dv.Views[0].Filters[0].RelationKey)
		})
	}

	t.Run("an option after a retarget resolves on the new target's property, creating nothing", func(t *testing.T) {
		// given: sprint by the detail, backlog by the second key; only
		// backlog's Stage holds Ready
		fx := setup(t, stageBacklogKey)
		read := templateRead(t, `{"formatVersion":"2.0","id":"obj1","kind":"template","type":"template","properties":{"name":"Weekly"}}`)
		read.Snapshot.ObjectTypes = []string{"ot-template", "ot-backlog"}
		read.Snapshot.Details.Fields[bundle.RelationKeyTargetObjectType.String()] = pbtypes.String("type-sprint")
		captured := fx.expectMutate(read, "headB")

		// when: the prewarm must follow the unset as the apply does
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(
			`{"op":"set_properties","unset":["targetObjectType"]}`,
			`{"op":"set_properties","set":{"Stage":"Ready"}}`), "", false, false)

		// then
		require.NoError(t, err)
		assert.Empty(t, createdOptions(result.Created))
		assert.Equal(t, []string{"opt-ready"}, (*captured).CombinedDetails().GetStringList(domain.RelationKey(stageBacklogKey)))
	})

	// statusPair adds the installed built-in status and the space's own
	// Pipeline status, which also serves the slug `status`
	const ownStatusKey = "6a8f2c1d9e4b7a3f5c2d8ea4"
	statusPair := func(t *testing.T, fx *v2Fixture) {
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-status"),
			bundle.RelationKeyRelationKey:    domain.String("status"),
			bundle.RelationKeyName:           domain.String("Status"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
		})
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-own-status"),
			bundle.RelationKeyRelationKey:    domain.String(ownStatusKey),
			bundle.RelationKeyApiObjectKey:   domain.String("status"),
			bundle.RelationKeyName:           domain.String("Pipeline status"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
		})
	}

	t.Run("an option after set_type to a type the batch installs resolves on its property, creating nothing", func(t *testing.T) {
		// given: backlog lists the space's own status, the never-installed
		// task lists the built-in one, and only the built-in holds Ready
		fx := setup(t, "status")
		statusPair(t, fx)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String("type-backlog"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-backlog"),
			bundle.RelationKeyName:                 domain.String("Backlog"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-own-status"}),
		})
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(backlogRead(t), nil).Maybe()

		// when: a dry run, so nothing installs task
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"task"}`, `{"op":"set_properties","set":{"status":"Ready"}}`), "", true, false)

		// then
		require.NoError(t, err)
		assert.Empty(t, createdOptions(result.Created))
	})

	t.Run("a view filter after a query source change resolves in the new source, creating nothing", func(t *testing.T) {
		// given: the template's view starts over sprint (the built-in
		// status) and moves to pipeline (the space's own); only the own one
		// holds Ready
		fx := setup(t, ownStatusKey)
		statusPair(t, fx)
		fx.addSprintType(t, objectstore.TestObject{
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-effort-points", "rel-status"}),
		})
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String("type-pipeline"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-pipeline"),
			bundle.RelationKeyName:                 domain.String("Pipeline"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-own-status"}),
		})
		read := viewTemplate(t)
		read.Snapshot.Details.Fields[bundle.RelationKeySetOf.String()] = pbtypes.StringList([]string{"type-sprint"})
		committed := fx.expectMutateState(read, nil)

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(
			`{"op":"set_properties","set":{"setOf":["type-pipeline"]}}`,
			`{"op":"update_view","set":{"filters":[{"property":"status","condition":"equal","value":["Ready"]}]}}`), "", false, false)

		// then
		require.NoError(t, err)
		assert.Empty(t, createdOptions(result.Created))
		dv := (*committed).Pick(state.DataviewBlockID).Model().GetDataview()
		require.Len(t, dv.Views, 1)
		require.Len(t, dv.Views[0].Filters, 1)
		assert.Equal(t, ownStatusKey, dv.Views[0].Filters[0].RelationKey)
	})

	t.Run("an option after a remove of the target resolves on the target left behind", func(t *testing.T) {
		// given: sprint by the detail, backlog by the second key; only
		// backlog's Stage holds Ready
		fx := setup(t, stageBacklogKey)
		read := templateRead(t, `{"formatVersion":"2.0","id":"obj1","kind":"template","type":"template","properties":{"name":"Weekly"}}`)
		read.Snapshot.ObjectTypes = []string{"ot-template", "ot-backlog"}
		read.Snapshot.Details.Fields[bundle.RelationKeyTargetObjectType.String()] = pbtypes.String("type-sprint")
		captured := fx.expectMutate(read, "headB")

		// when: remove, not unset, takes the detail away
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(
			`{"op":"set_properties","remove":{"targetObjectType":["type-sprint"]}}`,
			`{"op":"set_properties","set":{"Stage":"Ready"}}`), "", false, false)

		// then
		require.NoError(t, err)
		assert.Empty(t, createdOptions(result.Created))
		assert.Equal(t, []string{"opt-ready"}, (*captured).CombinedDetails().GetStringList(domain.RelationKey(stageBacklogKey)))
	})

	t.Run("an option after a retarget by folded reference resolves on the type it names", func(t *testing.T) {
		// given: funnel's row id is not type-funnel, the spelling the op
		// folds it to; the template type lists backlog's Stage, funnel the
		// pipeline Stage, and only the latter holds Ready
		fx := setup(t, stagePipelineKey)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String("fn-1"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-funnel"),
			bundle.RelationKeyName:                 domain.String("Funnel"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-stage-pipeline"}),
		})
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String(testTemplateTypeId),
			bundle.RelationKeyName:                 domain.String("Template"),
			bundle.RelationKeyUniqueKey:            domain.String(bundle.TypeKeyTemplate.URL()),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-stage-backlog"}),
		})
		read := templateRead(t, `{"formatVersion":"2.0","id":"obj1","kind":"template","type":"template","properties":{"name":"Weekly"}}`)
		read.Snapshot.ObjectTypes = []string{"ot-template"}
		captured := fx.expectMutate(read, "headB")

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(
			`{"op":"set_properties","set":{"targetObjectType":["type-funnel"]}}`,
			`{"op":"set_properties","set":{"Stage":"Ready"}}`), "", false, false)

		// then
		require.NoError(t, err)
		assert.Empty(t, createdOptions(result.Created))
		details := (*captured).CombinedDetails()
		assert.Equal(t, "fn-1", details.GetString(bundle.RelationKeyTargetObjectType))
		assert.Equal(t, []string{"opt-ready"}, details.GetStringList(domain.RelationKey(stagePipelineKey)))
	})

	t.Run("set_properties after set_type writes the new type's property", func(t *testing.T) {
		// given
		fx := setup(t, stageSprintKey)
		typed := fx.expectMutateTyped(backlogRead(t), "headB")

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"sprint"}`, `{"op":"set_properties","set":{"Effort":"3"}}`), "", false, true)

		// then
		require.NoError(t, err)
		details := typed.state.CombinedDetails()
		assert.Equal(t, "3", details.GetString(domain.RelationKey(effortPointsKey)))
		assert.False(t, details.Has(domain.RelationKey(effortHoursKey)))
	})

	t.Run("an option after set_type resolves on the new type's property, creating nothing", func(t *testing.T) {
		// given: only sprint's Stage holds Ready
		fx := setup(t, stageSprintKey)
		typed := fx.expectMutateTyped(backlogRead(t), "headB")

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"sprint"}`, `{"op":"set_properties","set":{"Stage":"Ready"}}`), "", false, false)

		// then
		require.NoError(t, err)
		assert.Empty(t, createdOptions(result.Created))
		assert.Equal(t, []string{"opt-ready"}, typed.state.CombinedDetails().GetStringList(domain.RelationKey(stageSprintKey)))
	})
}

// GO-7614 round two: the target a write and a read use is the one the
// object has when they run. A retarget through set_properties is stored the
// way heart reads it, a later op in the batch and a retarget that lands
// before the lock both move the scope, and rendering keeps every slot the
// codec needs to report a loss.
func TestV2TemplateTargetFollowsTheObject(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) *v2Fixture {
		fx := newEffortFixture(t)
		fx.addSprintType(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String("type-backlog"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-backlog"),
			bundle.RelationKeyName:                 domain.String("Backlog"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-effort-hours"}),
		})
		return fx
	}
	// a template stored with typeKeys and, unless nil, targetObjectType
	stored := func(t *testing.T, sbType model.SmartBlockType, target *types.Value, typeKeys ...string) apicore.ObjectRead {
		read := templateRead(t, `{"formatVersion":"2.0","id":"obj1","kind":"template","type":"template","properties":{"name":"Weekly"}}`)
		read.SbType = sbType
		read.Snapshot.ObjectTypes = typeKeys
		if target != nil {
			read.Snapshot.Details.Fields[bundle.RelationKeyTargetObjectType.String()] = target
		}
		return read
	}
	getDoc := func(t *testing.T, fx *v2Fixture, read apicore.ObjectRead) map[string]json.RawMessage {
		t.Helper()
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil).Maybe()
		body, _, err := fx.GetObject(ctx, testSpaceId, "obj1", ObjectQuery{})
		require.NoError(t, err)
		fields, err := parseEnvelope(body)
		require.NoError(t, err)
		return fields
	}
	stringMember := func(fields map[string]json.RawMessage, member string) string {
		return envelopeString(fields, member)
	}
	committedEffort := func(t *testing.T, st *state.State) string {
		t.Helper()
		details := st.CombinedDetails()
		switch {
		case details.Has(domain.RelationKey(effortPointsKey)):
			return "points"
		case details.Has(domain.RelationKey(effortHoursKey)):
			return "hours"
		}
		return ""
	}
	sprint := pbtypes.String("type-sprint")

	t.Run("a retarget through set_properties is stored as one id", func(t *testing.T) {
		// given
		fx := setup(t)
		captured := fx.expectMutate(stored(t, model.SmartBlockType_Template, nil, "ot-template", "ot-backlog"), "headB")

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"targetObjectType":["type-sprint"]}}`), "", false, true)

		// then
		require.NoError(t, err)
		got := (*captured).Details().Get(bundle.RelationKeyTargetObjectType)
		assert.Equal(t, domain.String("type-sprint"), got)
	})

	t.Run("a template is for one type: a second target id is refused, by set and by add", func(t *testing.T) {
		for name, op := range map[string]string{
			"set": `{"op":"set_properties","set":{"targetObjectType":["type-sprint","type-backlog"]}}`,
			"add": `{"op":"set_properties","add":{"targetObjectType":["type-backlog"]}}`,
		} {
			t.Run(name, func(t *testing.T) {
				// given
				fx := setup(t)
				fx.expectMutate(stored(t, model.SmartBlockType_Template, sprint, "ot-template", "ot-sprint"), "headB")

				// when
				_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(op), "", false, true)

				// then
				issue := issueAt(t, err, "ops[0].targetObjectType")
				assert.Contains(t, issue.Message, "a template is for one type")
			})
		}
	})

	t.Run("a remove that takes nothing away keeps the target one id", func(t *testing.T) {
		// given
		fx := setup(t)
		captured := fx.expectMutate(stored(t, model.SmartBlockType_Template, sprint, "ot-template", "ot-sprint"), "headB")

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","remove":{"targetObjectType":["type-backlog"]}}`), "", false, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, domain.String("type-sprint"), (*captured).Details().Get(bundle.RelationKeyTargetObjectType))
	})

	t.Run("a property set beside a retarget is judged against the new target", func(t *testing.T) {
		// given: a template for backlog, which lists the hours Effort
		fx := setup(t)
		fx.expectMutate(stored(t, model.SmartBlockType_Template, nil, "ot-template", "ot-backlog"), "headB")

		// when: one op retargets to sprint and sets the hours Effort
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"targetObjectType":["type-sprint"],"effort_hours":"3"}}`), "", false, true)

		// then
		require.NoError(t, err)
		require.Len(t, result.Warnings, 1, "%v", result.Warnings)
		assert.Contains(t, result.Warnings[0].Message, `is not on type "sprint"`)
	})

	t.Run("a target stored as a one-element list is the target, for a read and for a write", func(t *testing.T) {
		// given
		fx := setup(t)
		read := stored(t, model.SmartBlockType_Template, pbtypes.StringList([]string{"type-sprint"}), "ot-template", "ot-backlog")
		captured := fx.expectMutate(read, "headB")

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(`{"op":"set_properties","set":{"Effort":"3"}}`), "", false, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, "points", committedEffort(t, *captured))
		assert.Equal(t, "sprint", stringMember(getDoc(t, fx, read), "template_for"))
	})

	t.Run("an op after a retarget in the same batch resolves against the new target", func(t *testing.T) {
		// given: sprint by the detail, backlog by the second key
		fx := setup(t)
		captured := fx.expectMutate(stored(t, model.SmartBlockType_Template, sprint, "ot-template", "ot-backlog"), "headB")

		// when: unsetting the detail leaves the second key in charge
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(
			`{"op":"set_properties","unset":["targetObjectType"]}`,
			`{"op":"set_properties","set":{"Effort":"3"}}`), "", false, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, "hours", committedEffort(t, *captured))
	})

	t.Run("a retarget that lands before the lock moves the scope", func(t *testing.T) {
		// given: the plan reads sprint; by the time the lock is taken the
		// object has been retargeted to backlog
		fx := setup(t)
		committed := fx.expectMutateState(stored(t, model.SmartBlockType_Template, sprint, "ot-template", "ot-backlog"), func(st *state.State) {
			st.SetDetail(bundle.RelationKeyTargetObjectType, domain.String("type-backlog"))
		}, "headB")

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"Effort":"3","effort_points":"4"}}`), "", false, true)

		// then: the name binds backlog's property, and the off-target one warns naming backlog
		require.NoError(t, err)
		assert.Equal(t, "3", (*committed).CombinedDetails().GetString(domain.RelationKey(effortHoursKey)))
		require.Len(t, result.Warnings, 1, "%v", result.Warnings)
		assert.Contains(t, result.Warnings[0].Message, `is not on type "backlog"`)
	})

	t.Run("a read keeps a type key past the target slot, so the codec still reports it", func(t *testing.T) {
		// given
		fx := setup(t)
		read := stored(t, model.SmartBlockType_Template, sprint, "ot-template", "ot-sprint", "ot-backlog")

		// when
		fields := getDoc(t, fx, read)

		// then
		var warnings []v2model.Issue
		require.NoError(t, json.Unmarshal(fields["warnings"], &warnings))
		paths := make([]string, 0, len(warnings))
		for _, w := range warnings {
			paths = append(paths, w.Path)
		}
		assert.Contains(t, paths, "/type")
	})

	t.Run("a PATCH of a template with a type key past the target slot is refused as lossy", func(t *testing.T) {
		// given
		fx := setup(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").
			Return(stored(t, model.SmartBlockType_Template, sprint, "ot-template", "ot-sprint", "ot-backlog"), nil).Maybe()

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(`{"op":"set_properties","set":{"name":"Renamed"}}`), "", true, true)

		// then
		assert.Equal(t, http.StatusUnprocessableEntity, v2Err(t, err).Status)
	})

	t.Run("a target detail this space cannot resolve leaves no stale template_for", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		fields := getDoc(t, fx, stored(t, model.SmartBlockType_Template, pbtypes.String("type-ghost"), "ot-template", "ot-backlog"))

		// then
		assert.Empty(t, stringMember(fields, "template_for"))
	})

	t.Run("a template whose first type key is not template reads as a template of its target", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		fields := getDoc(t, fx, stored(t, model.SmartBlockType_Template, nil, "ot-backlog", "ot-sprint"))

		// then
		assert.Equal(t, "template", stringMember(fields, "kind"))
		assert.Equal(t, "template", stringMember(fields, "type"))
		assert.Equal(t, "sprint", stringMember(fields, "template_for"))
	})

	t.Run("a page whose first type key is template is not scoped as a template", func(t *testing.T) {
		// given: the codec serves no template_for for a page, so its writes
		// cannot be judged by one either
		fx := setup(t)
		read := stored(t, model.SmartBlockType_Page, sprint, "ot-template")
		fx.expectMutate(read)

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(`{"op":"set_properties","set":{"Effort":"3"}}`), "", false, true)

		// then
		assert.Equal(t, v2model.CodeAmbiguousInput, v2Err(t, err).Code)
	})
}

// GO-7614 round two: a template that names its target only in its second
// type key is a template of that type for a create and for the listing,
// exactly as GET serves it.
func TestV2TemplateTargetFromTheSecondTypeKey(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) *v2Fixture {
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		// the index row carries no targetObjectType; the object does
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("tpl-keyed"),
			bundle.RelationKeyName:           domain.String("Keyed memo"),
			bundle.RelationKeyType:           domain.String(testTemplateTypeId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})
		read := editRead(t, `{"formatVersion":"2.0","id":"tpl-keyed","kind":"template","type":"template","template_for":"memo","properties":{"name":"Keyed memo"}}`)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "tpl-keyed").Return(read, nil).Maybe()
		return fx
	}

	t.Run("a create names the template and starts from it", func(t *testing.T) {
		// given
		fx := setup(t)
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(ctx, testSpaceId, []byte(`{"type":"memo","name":"Monday","template":"tpl-keyed"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "tpl-keyed", *applied)
		require.NotNil(t, result.Template)
		assert.Equal(t, "tpl-keyed", result.Template.Id)
	})

	t.Run("the type's template listing includes it", func(t *testing.T) {
		// given
		fx := setup(t)
		want := []v2model.TemplateRow{{Id: "tpl-keyed", Name: "Keyed memo", TemplateFor: "memo"}}

		// when
		rows, total, _, err := fx.ListTemplates(ctx, testSpaceId, "memo", 0, 10)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows)
		assert.Equal(t, 1, total)
	})
}

// errStoreFault is the failure faultyStore injects.
var errStoreFault = errors.New("injected store fault")

// faultyStore fails chosen reads of the space index, for the paths that must
// hold when a store read fails: a unique-key lookup, and the two relation
// reads behind a property definition (ListAllRelations, GetRelationById).
type faultyStore struct {
	objectstore.ObjectStore
	failUniqueKey     string
	failRelationReads bool
}

func (f *faultyStore) SpaceIndex(spaceId string) spaceindex.Store {
	return &faultyIndex{Store: f.ObjectStore.SpaceIndex(spaceId), f: f}
}

type faultyIndex struct {
	spaceindex.Store
	f *faultyStore
}

func (i *faultyIndex) GetObjectByUniqueKey(uk domain.UniqueKey) (*domain.Details, error) {
	if i.f.failUniqueKey != "" && uk.Marshal() == i.f.failUniqueKey {
		return nil, errStoreFault
	}
	return i.Store.GetObjectByUniqueKey(uk)
}

func (i *faultyIndex) ListAllRelations() (relationutils.Relations, error) {
	if i.f.failRelationReads {
		return nil, errStoreFault
	}
	return i.Store.ListAllRelations()
}

func (i *faultyIndex) GetRelationById(id string) (*model.Relation, error) {
	if i.f.failRelationReads {
		return nil, errStoreFault
	}
	return i.Store.GetRelationById(id)
}

// GO-7614 round two: what a store failure may and may not do to a create.
func TestV2CreateUnderStoreFaults(t *testing.T) {
	ctx := context.Background()

	t.Run("a failed type lookup refuses the create before any option is minted", func(t *testing.T) {
		// given: chore's row cannot be read; Blocker is a new severity
		// option, and the strict mock fails any option create
		fx := newOffTypeFixture(t)
		fx.store = &faultyStore{ObjectStore: fx.objectStore, failUniqueKey: "ot-chore"}

		// when
		_, err := fx.CreateObject(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"chore","template":"none","properties":{"name":"X","severity":["Blocker"]}}`), false, true)

		// then
		require.ErrorIs(t, err, errStoreFault)
	})

	t.Run("a property definition that cannot be read draws no off-type warning", func(t *testing.T) {
		// given: chore lists severity; the definition reads fail, the
		// live property listing does not
		fx := newOffTypeFixture(t)
		fx.store = &faultyStore{ObjectStore: fx.objectStore, failRelationReads: true}

		// when
		result, err := fx.CreateObject(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"chore","properties":{"name":"X","severity":["High"]}}`), true, true)

		// then
		require.NoError(t, err)
		assert.Empty(t, result.Warnings)
	})
}

func TestV2TemplateKindNeedsTemplateType(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) *v2Fixture {
		fx := newOffTypeFixture(t)
		fx.addTemplateType(t)
		return fx
	}

	t.Run("a template kind on another type is refused at /type", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		_, err := fx.CreateObject(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","kind":"template","type":"page","template_for":"chore","properties":{"name":"Weekly","severity":["High"]}}`), false, true)

		// then
		issue := issueAt(t, err, "/type")
		assert.Equal(t, `expected type "template", got "page"`, issue.Message)
	})

	t.Run("a template kind on the template type still creates a template", func(t *testing.T) {
		// given
		fx := setup(t)
		captured := fx.expectCreate("tpl1")
		fx.expectEtagRead("tpl1")

		// when
		result, err := fx.CreateObject(ctx, testSpaceId,
			[]byte(`{"formatVersion":"2.0","kind":"template","type":"template","template_for":"chore","properties":{"name":"Weekly","severity":["High"]}}`), false, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, "template", result.Type)
		require.NotNil(t, *captured)
		assert.Equal(t, []string{"ot-template", "ot-chore"}, (*captured).ObjectTypes)
		assert.Empty(t, result.Warnings)
	})
}

func TestV2ReceiptsCountWhatChanged(t *testing.T) {
	ctx := context.Background()

	t.Run("item ops count the members they actually changed", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editCollectionDoc), "headB") // holds memberA

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"add_items","items":["memberA","memberB","memberC"]}`, `{"op":"remove_items","items":["memberA","memberZ"]}`), "", false, true)

		require.NoError(t, err)
		assert.Equal(t, 2, result.DiffStats.ItemsAdded, "memberA was already present")
		assert.Equal(t, 1, result.DiffStats.ItemsRemoved, "memberZ was never there")
	})

	t.Run("a member added and removed in one batch counts in neither", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editCollectionDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"add_items","items":["memberB"]}`, `{"op":"remove_items","items":["memberB"]}`), "", false, true)

		require.NoError(t, err)
		assert.Equal(t, 0, result.DiffStats.ItemsAdded)
		assert.Equal(t, 0, result.DiffStats.ItemsRemoved)
	})

	t.Run("a created collection says how many members it holds", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{bundle.RelationKeyId: domain.String("m1"), bundle.RelationKeyName: domain.String("One")},
			{bundle.RelationKeyId: domain.String("m2"), bundle.RelationKeyName: domain.String("Two")},
		})
		fx.expectCreate("newCollection")
		fx.expectEtagRead("newCollection")

		result, err := fx.CreateCollection(ctx, testSpaceId, v2model.CreateCollectionRequest{Name: "L", Items: []string{"m1", "m2"}}, false)

		require.NoError(t, err)
		require.NotNil(t, result.Items)
		assert.Equal(t, 2, *result.Items)

		fx2 := newV2Fixture(t)
		fx2.expectCreate("emptyCollection")
		fx2.expectEtagRead("emptyCollection")
		empty, err := fx2.CreateCollection(ctx, testSpaceId, v2model.CreateCollectionRequest{Name: "E"}, false)
		require.NoError(t, err)
		require.NotNil(t, empty.Items, "zero is reported, not omitted")
		assert.Equal(t, 0, *empty.Items)
	})
}

func TestV2ReadWarningsAreNotExportBookkeeping(t *testing.T) {
	// keyed on the member the warning is about: the legends and the stored
	// type key are stripped from every served document
	assert.True(t, readWarningIsNoise(anyblockjson.Issue{Path: "/property_internal_keys", Message: `legend value: "backlinks" is internal — so no legend entry is written for "backlinks"; the term is spelled verbatim`}))
	assert.True(t, readWarningIsNoise(anyblockjson.Issue{Path: "/type_internal_key", Message: `the type has no stored key, so type_internal_key is not written; the type is spelled verbatim`}))
	assert.True(t, readWarningIsNoise(anyblockjson.Issue{Path: "/option_ids/x", Message: "anything"}))
	assert.False(t, readWarningIsNoise(anyblockjson.Issue{Path: "/blocks", Message: `block b1: nesting depth 40 exceeds the bound 32 — indent clamped`}))
	assert.False(t, readWarningIsNoise(anyblockjson.Issue{Path: "/properties/due_date", Message: "a warning about a served member is kept, whatever it says: spelled verbatim"}))

	// the one read degradation an exposed operation repairs carries it
	dated := readWarningIssue(anyblockjson.Issue{Path: "/properties/due_date", Message: "date 1e+13 has no RFC 3339 form (outside years 0000-9999), so it is written as a raw number; a value this large is usually milliseconds where seconds belong"})
	assert.Equal(t, []v2model.Ref{v2model.RefGetOpSchema("set_properties")}, dated.SeeAlso)
	assert.Empty(t, readWarningIssue(anyblockjson.Issue{Path: "/blocks", Message: "indent clamped"}).SeeAlso)
}
