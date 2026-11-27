package api

// The API adapters that write through the editor directly (no ClientCommands
// call in between) hand their request's attribution to the state they apply
// (docs/ApiChangeAttribution.md, carry 2). smarttest records what the real
// smartblock.Apply would stamp on the pushed change — the same question,
// asked of the same state, at the same point; the smartblock package's own
// tests take it from there to the pushed pb.Change.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestAdaptersStampTheRequest(t *testing.T) {
	appCtx := context.Background()
	apiCtx := domain.CtxWithIntegrationName(appCtx, "Raycast")

	t.Run("v2 PATCH object: the edit is stamped with the calling app", func(t *testing.T) {
		// given
		sb := smarttest.New("obj1")
		sb.AddBlock(simple.New(&model.Block{Id: "obj1", ChildrenIds: []string{"p1"}}))
		sb.AddBlock(simple.New(&model.Block{Id: "p1",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "body"}}}))
		adapter := newObjectMutateAdapter(fakeGetter{sb: sb})
		edit := func(text string) func(apicore.ObjectEdit) error {
			return func(edit apicore.ObjectEdit) error {
				edit.State.Get("p1").Model().GetText().Text = text
				return nil
			}
		}

		// when: the same adapter serves an API request, then a caller with
		// no API session
		_, err := adapter.MutateObject(apiCtx, "space1", "obj1", allAxes, edit("by the api"))
		require.NoError(t, err)
		_, err = adapter.MutateObject(appCtx, "space1", "obj1", allAxes, edit("by something else"))
		require.NoError(t, err)

		// then: red if MutateObject builds its state without the request
		// (first value), or if the name outlives the request (second)
		want := []string{"Raycast", ""}
		assert.Equal(t, want, sb.Results.IntegrationNames)
	})

	t.Run("v2 widgets, space root: create, update and delete are stamped", func(t *testing.T) {
		// given
		fx := newWidgetAdapterFixture(t)

		// when
		entry, err := fx.adapter.CreateWidget(apiCtx, widgetTestSpaceId, apicore.WidgetScopeSpace, apicore.WidgetCreate{
			Target: "obj1", Layout: model.BlockContentWidget_Tree, Limit: 6,
		})
		require.NoError(t, err)
		limit := int32(10)
		_, err = fx.adapter.UpdateWidget(apiCtx, widgetTestSpaceId, apicore.WidgetScopeSpace, entry.Id, planOf(apicore.WidgetUpdate{Limit: &limit}))
		require.NoError(t, err)
		_, err = fx.adapter.DeleteWidget(apiCtx, widgetTestSpaceId, apicore.WidgetScopeSpace, entry.Id, entry.Target)
		require.NoError(t, err)

		// then: red if any of the three commits drops StampFromCtx
		want := []string{"Raycast", "Raycast", "Raycast"}
		assert.Equal(t, want, fx.spaceRoot.Results.IntegrationNames)
	})

	t.Run("v2 widgets, personal root: the store writes carry the stamp", func(t *testing.T) {
		// given: the personal root pushes no tree change — its Apply
		// projects the edit onto the favorites store
		fx := newWidgetAdapterFixture(t)

		// when
		entry, err := fx.adapter.CreateWidget(apiCtx, widgetTestSpaceId, apicore.WidgetScopePersonal, apicore.WidgetCreate{
			Target: "obj1", Layout: model.BlockContentWidget_Tree, Limit: 6,
		})
		require.NoError(t, err)
		_, err = fx.adapter.DeleteWidget(appCtx, widgetTestSpaceId, apicore.WidgetScopePersonal, entry.Id, entry.Target)
		require.NoError(t, err)

		// then: red if VirtualWidgetObject.Apply stops handing the apply's
		// attribution to the store ops, or hands it to a later apply too
		want := []string{"Raycast", ""}
		assert.Equal(t, want, fx.favorites.integrationNames)
	})
}
