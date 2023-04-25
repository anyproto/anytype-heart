//go:build integration

package tests

import (
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func (s *testSuite) TestBasic() {
	s.Run("open dashboard", func() {
		resp := call(s, s.ObjectOpen, &pb.RpcObjectOpenRequest{
			ObjectId: s.acc.Info.HomeObjectId,
		})

		s.Require().NotNil(resp.ObjectView)
		s.NotEmpty(resp.ObjectView.Blocks)
		s.NotEmpty(resp.ObjectView.Details)
		s.NotEmpty(resp.ObjectView.Restrictions)
		s.NotEmpty(resp.ObjectView.RelationLinks)
		s.NotZero(resp.ObjectView.Type)
	})

	s.Require().NotEmpty(
		call(s, s.ObjectSearch, &pb.RpcObjectSearchRequest{
			Keys: []string{"id", "type", "name"},
		}).Records,
	)

	call(s, s.ObjectSearchSubscribe, &pb.RpcObjectSearchSubscribeRequest{
		SubId: "recent",
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLastOpenedDate.String(),
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
		},
		Keys: []string{"id", "lastOpenedDate"},
	})

	s.Run("create and open an object", func() {
		objId := call(s, s.BlockLinkCreateWithObject, &pb.RpcBlockLinkCreateWithObjectRequest{
			InternalFlags: []*model.InternalFlag{
				{
					Value: model.InternalFlag_editorDeleteEmpty,
				},
				{
					Value: model.InternalFlag_editorSelectType,
				},
			},
			Details: &types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeyType.String(): pbtypes.String(bundle.TypeKeyNote.BundledURL()),
				},
			},
		}).TargetId

		resp := call(s, s.ObjectOpen, &pb.RpcObjectOpenRequest{
			ObjectId: objId,
		})
		s.Require().NotNil(resp.ObjectView)

		waitEvent(s, func(sa *pb.EventMessageValueOfSubscriptionAdd) {
			s.Equal(sa.SubscriptionAdd.Id, objId)
		})
		waitEvent(s, func(sa *pb.EventMessageValueOfObjectDetailsSet) {
			s.Equal(sa.ObjectDetailsSet.Id, objId)
			s.Contains(sa.ObjectDetailsSet.Details.Fields, bundle.RelationKeyLastOpenedDate.String())
		})
	})
}

func pageTemplate(children ...*Block) *Block {
	cs := []*Block{
		Header(Children(
			Text("",
				TextStyle(model.BlockContentText_Title),
				Fields(&types.Struct{
					Fields: map[string]*types.Value{
						"_detailsKey": pbtypes.StringList([]string{"name", "done"}),
					},
				}),
				Restrictions(model.BlockRestrictions{
					Remove: true,
					Drag:   true,
					DropOn: true,
				})),
			FeaturedRelations())),
	}
	cs = append(cs, children...)
	return Root(Children(cs...))
}

func (s *testSuite) TestSimpleEditor() {
	resp := call(s, s.ObjectCreate, &pb.RpcObjectCreateRequest{
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyType.String(): pbtypes.String(bundle.TypeKeyPage.BundledURL()),
			},
		},
	})

	s.NotEmpty(resp.ObjectId)

	bresp := call(s, s.BlockCreate, &pb.RpcBlockCreateRequest{
		ContextId: resp.ObjectId,
		Block:     Text("Level 1", Color("red")).block,
		TargetId:  "",
		Position:  model.Block_Inner,
	})
	s.NotEmpty(bresp.BlockId)

	bresp2 := call(s, s.BlockCreate, &pb.RpcBlockCreateRequest{
		ContextId: resp.ObjectId,
		Block:     Text("Level 2").block,
		TargetId:  bresp.BlockId,
		Position:  model.Block_Inner,
	})
	s.NotEmpty(bresp2.BlockId)

	sresp := call(s, s.ObjectShow, &pb.RpcObjectShowRequest{
		ObjectId: resp.ObjectId,
	})

	want := pageTemplate(
		Text("Level 1", Color("red"), Children(
			Text("Level 2")),
		))

	AssertTreesEqual(s.T(), BuildAST(want.Build()), BuildAST(sresp.ObjectView.Blocks))
}
