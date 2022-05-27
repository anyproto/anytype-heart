package bookmark

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

type testObjectManager struct {
	object smartblock.SmartBlock
}

func (m testObjectManager) CreateSmartBlock(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation) (id string, newDetails *types.Struct, err error) {
	return "", nil, nil
}

func (m *testObjectManager) SetDetails(ctx *state.Context, req pb.RpcObjectSetDetailsRequest) (err error) {
	return m.object.SetDetails(ctx, req.Details, false)
}

func (m testObjectManager) Do(id string, apply func(b smartblock.SmartBlock) error) error {
	return apply(m.object)
}

func TestUpdateBookmarkObject(t *testing.T) {
	newContent := &model.BlockContentBookmark{
		Url:            "https://example.com",
		Title:          "Example",
		Description:    "Very descriptive",
		FaviconHash:    "fhash",
		ImageHash:      "ihash",
		TargetObjectId: "doesn't matter here",
	}

	t.Run("empty object", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test"}))

		oldBlocks := sb.Blocks()

		m := &testObjectManager{sb}
		svc := service{
			objectManager: m,
		}

		err := svc.UpdateBookmarkObject("test", func() (*model.BlockContentBookmark, error) {
			return newContent, nil
		})

		assert.NoError(t, err)

		assertUpdatedObject(t, sb, oldBlocks, newContent)
	})

	t.Run("extra blocks", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"extra1"}}))
		sb.AddBlock(simple.New(&model.Block{Id: "extra1", ChildrenIds: []string{"extra2", "extra3"}}))
		sb.AddBlock(simple.New(&model.Block{Id: "extra2"}))
		sb.AddBlock(simple.New(&model.Block{Id: "extra3"}))
		oldBlocks := sb.Blocks()

		m := &testObjectManager{sb}
		svc := service{
			objectManager: m,
		}

		err := svc.UpdateBookmarkObject("test", func() (*model.BlockContentBookmark, error) {
			return newContent, nil
		})

		assert.NoError(t, err)

		assertUpdatedObject(t, sb, oldBlocks, newContent)
	})

	t.Run("required relation blocks placed in chaotic order", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"extra1", "quote", "tag", "url"}}))
		sb.AddBlock(simple.New(&model.Block{Id: "extra1"}))
		sb.AddBlock(simple.New(makeRelationBlock("quote")))
		sb.AddBlock(simple.New(makeRelationBlock("tag")))
		sb.AddBlock(simple.New(makeRelationBlock("url")))
		oldBlocks := sb.Blocks()

		m := &testObjectManager{sb}
		svc := service{
			objectManager: m,
		}

		err := svc.UpdateBookmarkObject("test", func() (*model.BlockContentBookmark, error) {
			return newContent, nil
		})

		assert.NoError(t, err)

		assertUpdatedObject(t, sb, oldBlocks, newContent)
	})
}

func assertUpdatedObject(t *testing.T, sb *smarttest.SmartTest, oldBlocks []*model.Block, newContent *model.BlockContentBookmark) {
	// Ensure that relation blocks are added
	for _, k := range relationBlockKeys {
		b := sb.Pick(k)
		if !assert.NotNil(t, b, "block: ", k) {
			continue
		}

		want := makeRelationBlock(k)
		assert.Equal(t, want.Content, b.Model().Content, "block: ", k)
	}
	// Ensure that old blocks are still present
	for _, ob := range oldBlocks {
		b := sb.Pick(ob.Id)
		assert.NotNil(t, b, "block: ", ob.Id)
	}

	// Assert correct order of blocks
	{
		root := sb.Pick(sb.RootId())

		// Required blocks come first
		gotIds := root.Model().ChildrenIds[:len(relationBlockKeys)]
		assert.Equal(t, relationBlockKeys, gotIds)
	}

	require.NotNil(t, sb.Details())
	wantDetails := detailsFromContent(newContent)
	for k, v := range wantDetails {
		assert.Equal(t, v, sb.Details().Fields[k], "details: ", k)
	}
}
