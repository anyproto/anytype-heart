//go:build integration

package test

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
					bundle.RelationKeyType.String(): pbtypes.String(bundle.TypeKeyNote.URL()),
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
