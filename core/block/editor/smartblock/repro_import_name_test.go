package smartblock

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// newStateWithTextBlock builds a note-shaped doc: a root holding a single text block,
// with the title living in that block rather than in the name detail.
func newStateWithTextBlock(rootId, blockId, text string) *state.State {
	return state.NewDoc(rootId, map[string]simple.Block{
		rootId: simple.New(&model.Block{Id: rootId, ChildrenIds: []string{blockId}}),
		blockId: simple.New(&model.Block{
			Id:      blockId,
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: text}},
		}),
	}).NewState()
}

// An object imported before its custom type is indexed used to resolve to the note layout,
// because it carries its title in the name detail and so has no title block. The creation
// migration then moved that name into a plain text block and deleted the detail, leaving the
// object as "Untitled" in sets until it was opened.
func TestResolveLayoutOfObjectWithNotIndexedType(t *testing.T) {
	const id = "id"

	newImportedState := func() *state.State {
		st := state.NewDoc(id, nil).NewState()
		st.SetDetail(bundle.RelationKeyName, domain.String("Ship import fix to prod"))
		st.SetObjectTypeKey(domain.TypeKey("teamTask"))
		st.SetLocalDetail(bundle.RelationKeyType, domain.String("typeObjectId"))
		return st
	}

	t.Run("type is not indexed yet -> basic layout is guessed and the name is kept", func(t *testing.T) {
		// given
		fx := newFixture(id, t)
		st := newImportedState()

		// when
		fx.resolveLayout(st)

		// then
		assert.Equal(t, int64(model.ObjectType_basic), st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout))
		assert.Equal(t, "Ship import fix to prod", st.Details().GetString(bundle.RelationKeyName))
		assert.Equal(t, "", st.Snippet(), "name must not leak into the snippet as a text block")
	})

	t.Run("type is indexed -> layout comes from the type and the name is kept", func(t *testing.T) {
		// given
		fx := newFixture(id, t)
		st := newImportedState()
		fx.lastDepDetails = map[string]*domain.Details{
			"typeObjectId": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyRecommendedLayout: domain.Int64(model.ObjectType_basic),
			}),
		}

		// when
		fx.resolveLayout(st)

		// then
		require.Equal(t, int64(model.ObjectType_basic), st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout))
		assert.Equal(t, "Ship import fix to prod", st.Details().GetString(bundle.RelationKeyName))
	})

	t.Run("guessed layout does not convert blocks", func(t *testing.T) {
		// given: a note-shaped object - no name, title lives in the first text block -
		// whose type is unknown. Guessing basic must not pull that block into the name.
		fx := newFixture(id, t)
		st := newStateWithTextBlock(id, "textBlock", "First line doubles as the title")
		st.SetObjectTypeKey(domain.TypeKey("teamNote"))
		st.SetLocalDetail(bundle.RelationKeyType, domain.String("typeObjectId"))

		// when
		fx.resolveLayout(st)

		// then
		assert.Equal(t, int64(model.ObjectType_basic), st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout))
		assert.Equal(t, "", st.Details().GetString(bundle.RelationKeyName))
		assert.True(t, st.Exists("textBlock"), "text block was consumed on a guessed layout")
	})

	t.Run("known layout still converts blocks", func(t *testing.T) {
		// given: same object, but now the type is indexed and says basic
		fx := newFixture(id, t)
		st := newStateWithTextBlock(id, "textBlock", "First line doubles as the title")
		st.SetObjectTypeKey(domain.TypeKey("teamNote"))
		st.SetLocalDetail(bundle.RelationKeyType, domain.String("typeObjectId"))
		fx.lastDepDetails = map[string]*domain.Details{
			"typeObjectId": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyRecommendedLayout: domain.Int64(model.ObjectType_basic),
			}),
		}

		// when
		fx.resolveLayout(st)

		// then
		assert.Equal(t, "First line doubles as the title", st.Details().GetString(bundle.RelationKeyName))
	})
}
