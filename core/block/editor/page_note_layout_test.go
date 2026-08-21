package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The note layout is the one layout whose creation migration deletes the name detail and
// turns it into a plain text block. That is intended for real notes, and is why resolveLayout
// must never hand a merely guessed note layout to this migration.
func TestCreationStateMigrationNoteLayoutMovesNameIntoBlock(t *testing.T) {
	newState := func(layout model.ObjectTypeLayout) *state.State {
		st := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root"}),
		}).NewState()
		st.SetDetail(bundle.RelationKeyName, domain.String("Ship import fix to prod"))
		st.SetLocalDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(layout)))
		return st
	}

	t.Run("resolvedLayout=note -> name detail is stripped into a text block", func(t *testing.T) {
		// given
		p := &Page{}
		st := newState(model.ObjectType_note)
		ctx := &smartblock.InitContext{State: st, IsNewObject: true}

		// when
		p.CreationStateMigration(ctx).Proc(st)

		// then
		assert.Equal(t, "", st.Details().GetString(bundle.RelationKeyName), "name survived")
		assert.Equal(t, "Ship import fix to prod", st.Snippet(), "name leaked into snippet")
	})

	t.Run("resolvedLayout=basic -> name detail is kept", func(t *testing.T) {
		// given
		p := &Page{}
		st := newState(model.ObjectType_basic)
		ctx := &smartblock.InitContext{State: st, IsNewObject: true}

		// when
		p.CreationStateMigration(ctx).Proc(st)

		// then
		assert.Equal(t, "Ship import fix to prod", st.Details().GetString(bundle.RelationKeyName))
		assert.Equal(t, "", st.Snippet())
	})
}
