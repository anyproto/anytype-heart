package editor

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testApp() *app.App {
	ap := new(app.App)
	ap.Register(restriction.New())
	return ap
}

func TestBreadcrumbs_Init(t *testing.T) {
	b := NewBreadcrumbs(nil)
	err := b.Init(&smartblock.InitContext{
		App:    testApp(),
		Source: source.NewVirtual(nil, model.SmartBlockType_Breadcrumbs),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, b.Id())
	assert.NotEmpty(t, b.RootId())
	assert.Len(t, b.Blocks(), 1)
}

func TestBreadcrumbs_SetCrumbs(t *testing.T) {
	t.Run("set ids", func(t *testing.T) {
		b := NewBreadcrumbs(nil)
		err := b.Init(&smartblock.InitContext{
			App:    testApp(),
			Source: source.NewVirtual(nil, model.SmartBlockType_Breadcrumbs),
		})
		require.NoError(t, err)
		require.NoError(t, b.SetCrumbs([]string{"one", "two"}))
		require.Len(t, b.NewState().Pick(b.RootId()).Model().ChildrenIds, 2)
		require.NoError(t, b.SetCrumbs([]string{"one", "two", "three"}))
		require.Len(t, b.NewState().Pick(b.RootId()).Model().ChildrenIds, 3)
		require.NoError(t, b.SetCrumbs([]string{"next"}))
		require.Len(t, b.NewState().Pick(b.RootId()).Model().ChildrenIds, 1)
	})
}
