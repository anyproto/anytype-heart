package v2service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestV2CreateSet(t *testing.T) {
	setup := func(t *testing.T) *v2Fixture {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.addTaskType(t) // type "chore" recommending "severity"
		return fx
	}

	t.Run("set lands with a fully-formed dataview block in one change set", func(t *testing.T) {
		// given
		fx := setup(t)
		captured := fx.expectCreate("newSet")
		fx.expectEtagRead("newSet")

		// when
		result, err := fx.CreateSet(context.Background(), testSpaceId, v2model.CreateSetRequest{
			Name:    "Open chores",
			Type:    "chore",
			Filters: json.RawMessage(`[{"property":"severity","condition":"in","value":["High"]}]`),
			Sorts:   json.RawMessage(`[{"property":"severity","direction":"desc"}]`),
		}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "newSet", result.Id)
		snapshot := *captured
		require.NotNil(t, snapshot)
		assert.Equal(t, []string{"ot-set"}, snapshot.ObjectTypes)
		assert.Equal(t, []string{"type-chore"}, pbtypes.GetStringList(snapshot.Details, bundle.RelationKeySetOf.String()))

		require.Len(t, snapshot.Blocks, 2, "root + dataview")
		dvBlock := snapshot.Blocks[1]
		assert.Equal(t, "dataview", dvBlock.Id, "the editor finds its dataview under the template block id")
		dv := dvBlock.GetDataview()
		require.NotNil(t, dv)
		require.Len(t, dv.Views, 1)
		view := dv.Views[0]
		assert.Equal(t, "All", view.Name)
		require.Len(t, view.Filters, 1)
		assert.Equal(t, "severity", view.Filters[0].RelationKey)
		// option NAME resolved to the existing option id (SPEC §3/§6.2)
		assert.Equal(t, []string{"opt-high"}, pbtypes.GetStringListValue(view.Filters[0].Value))
		require.Len(t, view.Sorts, 1)
		assert.Equal(t, "severity", view.Sorts[0].RelationKey)
		assert.Equal(t, model.BlockContentDataviewSort_Desc, view.Sorts[0].Type)
	})

	t.Run("filter naming a property the type lacks lists the actual keys (R9)", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		_, err := fx.CreateSet(context.Background(), testSpaceId, v2model.CreateSetRequest{
			Name:    "Broken",
			Type:    "chore",
			Filters: json.RawMessage(`[{"property":"sevirity","condition":"equal","value":true}]`),
		}, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/filters/0/property", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "severity", "the error lists the type's actual keys")
		assert.Contains(t, apiErr.Issues[0].Hint, "severity", "did-you-mean suggests the close key")
	})

	t.Run("unknown type key gets a did-you-mean 400", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		_, err := fx.CreateSet(context.Background(), testSpaceId,
			v2model.CreateSetRequest{Name: "X", Type: "chores"}, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/type", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "chore")
	})

	t.Run("a type filter gets a targeted message, not unknown-property (string form)", func(t *testing.T) {
		// given: the discovery-served grammar example uses `type IN (…)`,
		// which search accepts — a set is already type-scoped, and the error
		// must say that instead of "unknown property key"
		fx := setup(t)

		// when
		_, err := fx.CreateSet(context.Background(), testSpaceId, v2model.CreateSetRequest{
			Name:   "Open work",
			Type:   "chore",
			Filter: `type IN ("chore") AND severity IS EMPTY`,
		}, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].Message, `a set is already scoped to type "chore" — drop the type filter`)
		assert.Contains(t, apiErr.Issues[0].Hint, "POST /v2/spaces/{spaceId}/search")
	})

	t.Run("a type filter gets the targeted message in the structured form too", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		_, err := fx.CreateSet(context.Background(), testSpaceId, v2model.CreateSetRequest{
			Name:    "Open work",
			Type:    "chore",
			Filters: json.RawMessage(`[{"property":"type","condition":"in","value":["chore"]}]`),
		}, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/filters/0/property", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, `a set is already scoped to type "chore"`)
	})

	t.Run("filter and filters together are ambiguous_input (C6)", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		_, err := fx.CreateSet(context.Background(), testSpaceId, v2model.CreateSetRequest{
			Name: "X", Type: "chore",
			Filter:  `done = false`,
			Filters: json.RawMessage(`[]`),
		}, false)

		// then — note: `[]` is non-empty as raw JSON
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, "not both")
	})

	t.Run("the compact filter string parses into the stored structured array", func(t *testing.T) {
		// given
		fx := setup(t)
		captured := fx.expectCreate("newSet")
		fx.expectEtagRead("newSet")

		// when
		result, err := fx.CreateSet(context.Background(), testSpaceId, v2model.CreateSetRequest{
			Name:   "High chores",
			Type:   "chore",
			Filter: `severity IN ("High") AND name CONTAINS "fix"`,
		}, false)

		// then — the set document stores the structured array (SPEC §6.2.1:
		// the document field `filter` stays reserved; export writes filters)
		require.NoError(t, err)
		assert.Equal(t, "newSet", result.Id)
		snapshot := *captured
		require.NotNil(t, snapshot)
		dv := snapshot.Blocks[1].GetDataview()
		require.NotNil(t, dv)
		require.Len(t, dv.Views, 1)
		filters := dv.Views[0].Filters
		require.Len(t, filters, 2)
		assert.Equal(t, "severity", filters[0].RelationKey)
		assert.Equal(t, model.BlockContentDataviewFilter_In, filters[0].Condition)
		assert.Equal(t, []string{"opt-high"}, pbtypes.GetStringListValue(filters[0].Value),
			"the option NAME in the string resolves to the existing option id")
		assert.Equal(t, "name", filters[1].RelationKey)
		assert.Equal(t, model.BlockContentDataviewFilter_Like, filters[1].Condition)
	})

	t.Run("a filter-string parse error is offset-addressed with did-you-mean", func(t *testing.T) {
		// given
		fx := setup(t)

		// when — "sevirity" is a typo of the type's "severity"
		_, err := fx.CreateSet(context.Background(), testSpaceId, v2model.CreateSetRequest{
			Name: "X", Type: "chore", Filter: `sevirity IN ("High")`,
		}, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/filter", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, `parse error at offset 0 near "sevirity"`)
		assert.Contains(t, apiErr.Issues[0].Message, `unknown property key "sevirity"`)
		assert.Equal(t, "did you mean severity?", apiErr.Issues[0].Hint)
	})

	t.Run("system keys pass the sets reference set (rule 2)", func(t *testing.T) {
		// given
		fx := setup(t)
		captured := fx.expectCreate("newSet")
		fx.expectEtagRead("newSet")

		// when — lastModifiedDate is in no type's recommended lists
		result, err := fx.CreateSet(context.Background(), testSpaceId, v2model.CreateSetRequest{
			Name: "Fresh chores", Type: "chore",
			Filter: `lastModifiedDate > daysAgo(7)`,
		}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "newSet", result.Id)
		dv := (*captured).Blocks[1].GetDataview()
		require.Len(t, dv.Views[0].Filters, 1)
		assert.Equal(t, "lastModifiedDate", dv.Views[0].Filters[0].RelationKey)
		assert.Equal(t, model.BlockContentDataviewFilter_NumberOfDaysAgo, dv.Views[0].Filters[0].QuickOption)
	})

	t.Run("views with top-level filters are ambiguous_input", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		_, err := fx.CreateSet(context.Background(), testSpaceId, v2model.CreateSetRequest{
			Name: "X", Type: "chore",
			Views:   json.RawMessage(`[{"name":"V"}]`),
			Filters: json.RawMessage(`[{"property":"severity","condition":"notEmpty"}]`),
		}, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
	})

	t.Run("M6: the advertised sorts cap is enforced", func(t *testing.T) {
		fx := setup(t)
		sorts := make([]map[string]string, maxV2SetSorts+1)
		for i := range sorts {
			sorts[i] = map[string]string{"property": "severity"}
		}
		raw, err := json.Marshal(sorts)
		require.NoError(t, err)

		_, err = fx.CreateSet(context.Background(), testSpaceId,
			v2model.CreateSetRequest{Name: "Sorted", Type: "chore", Sorts: raw}, false)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/sorts", apiErr.Issues[0].Path)
	})

	t.Run("dry run validates without creating", func(t *testing.T) {
		// given: no creator expectations
		fx := setup(t)

		// when
		result, err := fx.CreateSet(context.Background(), testSpaceId, v2model.CreateSetRequest{
			Name: "Open chores", Type: "chore",
			Filters: json.RawMessage(`[{"property":"severity","condition":"notEmpty"}]`),
		}, true)

		// then
		require.NoError(t, err)
		assert.True(t, result.DryRun)
		assert.Empty(t, result.Id)
	})
}

func TestV2CreateCollection(t *testing.T) {
	t.Run("collection carries its items through the import path", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("member1"),
			bundle.RelationKeyName:           domain.String("Member"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})
		captured := fx.expectCreate("newCollection")
		fx.expectEtagRead("newCollection")

		// when
		result, err := fx.CreateCollection(context.Background(), testSpaceId,
			v2model.CreateCollectionRequest{Name: "Reading list", Items: []string{"member1"}}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "newCollection", result.Id)
		snapshot := *captured
		require.NotNil(t, snapshot)
		assert.Equal(t, []string{"ot-collection"}, snapshot.ObjectTypes)
		require.NotNil(t, snapshot.Collections, "items land in the collection store")
		objects := snapshot.Collections.Fields["objects"]
		require.NotNil(t, objects)
		assert.Equal(t, []string{"member1"}, pbtypes.GetStringListValue(objects))
	})

	t.Run("unknown items are rejected with their index", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateCollection(context.Background(), testSpaceId,
			v2model.CreateCollectionRequest{Name: "X", Items: []string{"ghost"}}, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/items/0", apiErr.Issues[0].Path)
	})

	t.Run("M6: the items cap is enforced before any store lookup", func(t *testing.T) {
		fx := newV2Fixture(t)
		items := make([]string, maxV2CollectionItems+1)
		for i := range items {
			items[i] = fmt.Sprintf("obj%d", i)
		}

		_, err := fx.CreateCollection(context.Background(), testSpaceId,
			v2model.CreateCollectionRequest{Name: "Big", Items: items}, false)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/items", apiErr.Issues[0].Path)
		assert.NotContains(t, apiErr.Message, "not found",
			"the cap must fire before the per-item existence walk")
	})

	t.Run("name is required", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateCollection(context.Background(), testSpaceId, v2model.CreateCollectionRequest{}, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
	})
}
