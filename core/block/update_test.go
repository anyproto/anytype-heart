package block

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
	"github.com/mohae/deepcopy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommonSmart_Update(t *testing.T) {

	newSmartBlock := func(fx *fixture) (*commonSmart, *blockWrapper) {
		sb := &commonSmart{
			s:              fx.Service.(*service),
			versionsChange: func(vers []core.BlockVersion) {},
		}

		block, _ := fx.newMockBlockWithContent(
			"1",
			&model.BlockCoreContentOfPage{Page: &model.BlockContentPage{}},
			[]string{"2", "3"},
			map[string]core.BlockVersion{
				"2": fx.newMockVersion(&model.Block{Id: "2"}),
				"3": fx.newMockVersion(&model.Block{Id: "3"}),
			},
		)
		require.NoError(t, sb.Open(block))
		return sb, block
	}

	newChanges := func() []*pb.ChangesBlock {
		return []*pb.ChangesBlock{
			{
				Id: "2",
				Fields: &types.Struct{
					Fields: map[string]*types.Value{
						"test": testStringValue("test value 2"),
					},
				},
				Content: &model.BlockCore{Content: &model.BlockCoreContentOfText{Text: &model.BlockContentText{
					Text: "text 2",
				}}},
			},
			{
				Id: "3",
				Fields: &types.Struct{
					Fields: map[string]*types.Value{
						"test": testStringValue("test value 3"),
					},
				},
				Content: &model.BlockCore{Content: &model.BlockCoreContentOfText{Text: &model.BlockContentText{
					Text: "text 3",
				}}},
			},
		}
	}

	t.Run("should update blocks", func(t *testing.T) {
		fx := newFixture(t, "1")
		defer fx.ctrl.Finish()
		defer fx.tearDown()
		sb, ab := newSmartBlock(fx)

		changes := newChanges()

		ab.EXPECT().AddVersions(&matcher{name: "add versions", f: func(x interface{}) bool {
			require.IsType(t, ([]*model.Block)(nil), x)
			versions := x.([]*model.Block)
			require.Len(t, versions, 2)
			var b2, b3 *model.Block
			for _, v := range versions {
				switch v.Id {
				case "2":
					b2 = v
				case "3":
					b3 = v
				}

			}

			require.NotNil(t, b2)
			assert.Equal(t, b2.Content.String(), changes[0].Content.String())
			assert.Equal(t, b2.Fields, changes[0].Fields)

			require.NotNil(t, b3)
			assert.Equal(t, b3.Content.String(), changes[1].Content.String())
			assert.Equal(t, b3.Fields, changes[1].Fields)
			return true
		}})

		err := sb.Update(pb.RpcBlockUpdateRequest{
			Changes: &pb.Changes{
				Changes: changes,
			},
		})
		require.NoError(t, err)
	})

	t.Run("should rollback changes for error", func(t *testing.T) {
		fx := newFixture(t, "1")
		defer fx.ctrl.Finish()
		defer fx.tearDown()
		sb, _ := newSmartBlock(fx)

		origVersions := deepcopy.Copy(sb.versions).(map[string]simple)

		changes := newChanges()

		changes[1].ChildrenIds = &pb.ChangesBlockChildrenIds{ChildrenIds: []string{"nonexistent"}}

		err := sb.Update(pb.RpcBlockUpdateRequest{
			Changes: &pb.Changes{
				Changes: changes,
			},
		})
		require.Error(t, err)

		assert.Equal(t, origVersions, sb.versions)
	})
}
