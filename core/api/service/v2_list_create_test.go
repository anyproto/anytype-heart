package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
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
		result, err := fx.CreateSet(context.Background(), testSpaceId, apimodel.V2CreateSetRequest{
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
		_, err := fx.CreateSet(context.Background(), testSpaceId, apimodel.V2CreateSetRequest{
			Name:    "Broken",
			Type:    "chore",
			Filters: json.RawMessage(`[{"property":"sevirity","condition":"equal","value":true}]`),
		}, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeValidationFailed, apiErr.Code)
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
			apimodel.V2CreateSetRequest{Name: "X", Type: "chores"}, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/type", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "chore")
	})

	t.Run("filter and filters together are ambiguous_input (C6)", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		_, err := fx.CreateSet(context.Background(), testSpaceId, apimodel.V2CreateSetRequest{
			Name: "X", Type: "chore",
			Filter:  `done = false`,
			Filters: json.RawMessage(`[]`),
		}, false)

		// then — note: `[]` is non-empty as raw JSON
		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, "not both")
	})

	t.Run("the compact filter string is not implemented yet", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		_, err := fx.CreateSet(context.Background(), testSpaceId, apimodel.V2CreateSetRequest{
			Name: "X", Type: "chore", Filter: `done = false`,
		}, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotImplemented, apiErr.Status)
		assert.Contains(t, apiErr.Message, "filters")
	})

	t.Run("views with top-level filters are ambiguous_input", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		_, err := fx.CreateSet(context.Background(), testSpaceId, apimodel.V2CreateSetRequest{
			Name: "X", Type: "chore",
			Views:   json.RawMessage(`[{"name":"V"}]`),
			Filters: json.RawMessage(`[{"property":"severity","condition":"notEmpty"}]`),
		}, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeAmbiguousInput, apiErr.Code)
	})

	t.Run("dry run validates without creating", func(t *testing.T) {
		// given: no creator expectations
		fx := setup(t)

		// when
		result, err := fx.CreateSet(context.Background(), testSpaceId, apimodel.V2CreateSetRequest{
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
			apimodel.V2CreateCollectionRequest{Name: "Reading list", Items: []string{"member1"}}, false)

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
			apimodel.V2CreateCollectionRequest{Name: "X", Items: []string{"ghost"}}, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/items/0", apiErr.Issues[0].Path)
	})

	t.Run("name is required", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateCollection(context.Background(), testSpaceId, apimodel.V2CreateCollectionRequest{}, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeValidationFailed, apiErr.Code)
	})
}
