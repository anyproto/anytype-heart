package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestPage_StateMigrations(t *testing.T) {
	t.Run("page doesn't have relation links", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		p := &Page{SmartBlock: sb}

		initCtx := &smartblock.InitContext{State: sb.NewState()}

		// when
		migration.RunMigrations(p, initCtx)
		err := p.Apply(initCtx.State)
		assert.NoError(t, err)

		// then
		fields := p.NewState().Details().GetFields()
		assert.Len(t, fields, 1)
		assert.NotEmpty(t, fields[bundle.RelationKeyFeaturedRelations.String()])
	})
	t.Run("page has relation links, but not checkbox", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		err := sb.AddRelationLinks(nil, "key")
		assert.NoError(t, err)

		p := &Page{SmartBlock: sb}

		initCtx := &smartblock.InitContext{State: sb.NewState()}

		// when
		migration.RunMigrations(p, initCtx)
		err = p.Apply(initCtx.State)
		assert.NoError(t, err)

		// then
		fields := p.NewState().Details().GetFields()
		assert.Len(t, fields, 1)
		assert.NotEmpty(t, fields[bundle.RelationKeyFeaturedRelations.String()])
	})
	t.Run("page has done relation", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		newState := sb.NewState()
		newState.AddRelationLinks(&model.RelationLink{Key: bundle.RelationKeyDone.String(), Format: model.RelationFormat_checkbox})
		err := sb.Apply(newState)
		assert.NoError(t, err)

		p := &Page{SmartBlock: sb}
		initCtx := &smartblock.InitContext{State: sb.NewState()}

		// when
		migration.RunMigrations(p, initCtx)
		err = p.Apply(initCtx.State)
		assert.NoError(t, err)

		// then
		fields := p.NewState().Details().GetFields()
		assert.Len(t, fields, 2)
		assert.Equal(t, pbtypes.Bool(false), fields[bundle.RelationKeyDone.String()])
	})
	t.Run("page has custom checkbox relation", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		newState := sb.NewState()
		newState.AddRelationLinks(&model.RelationLink{Key: "key", Format: model.RelationFormat_checkbox})
		err := sb.Apply(newState)
		assert.NoError(t, err)

		p := &Page{SmartBlock: sb}
		initCtx := &smartblock.InitContext{State: sb.NewState()}

		// when
		migration.RunMigrations(p, initCtx)
		err = p.Apply(initCtx.State)
		assert.NoError(t, err)

		// then
		fields := p.NewState().Details().GetFields()
		assert.Len(t, fields, 2)
		assert.Equal(t, pbtypes.Bool(false), fields["key"])
	})
}
