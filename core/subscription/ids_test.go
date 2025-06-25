package subscription

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

func TestIdsSub(t *testing.T) {
	fx := newFixtureWithRealObjectStore(t)

	const testSubId = "subId"

	initialObjects := []spaceindex.TestObject{
		{
			bundle.RelationKeyId:      domain.String("obj1"),
			bundle.RelationKeySpaceId: domain.String(testSpaceId),
			bundle.RelationKeyName:    domain.String("Obj 1"),
		},
		{
			bundle.RelationKeyId:      domain.String("obj2"),
			bundle.RelationKeySpaceId: domain.String(testSpaceId),
		},
	}
	fx.store.AddObjects(t, testSpaceId, initialObjects)

	resp, err := fx.SubscribeIdsReq(pb.RpcObjectSubscribeIdsRequest{
		SpaceId: testSpaceId,
		SubId:   testSubId,
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeySpaceId.String(),
			bundle.RelationKeyName.String(),
		},
		Ids: []string{"obj1", "obj2", "obj3"},
	})
	require.NoError(t, err)

	wantRecords := make([]*types.Struct, 0, len(resp.Records))
	for _, record := range initialObjects {
		wantRecords = append(wantRecords, record.Details().ToProto())
	}

	assert.ElementsMatch(t, wantRecords, resp.Records)

	thirdObject := spaceindex.TestObject{
		bundle.RelationKeyId:      domain.String("obj3"),
		bundle.RelationKeySpaceId: domain.String(testSpaceId),
	}
	unsuitableObject := spaceindex.TestObject{
		bundle.RelationKeyId:      domain.String("obj4"),
		bundle.RelationKeySpaceId: domain.String(testSpaceId),
	}
	fx.store.AddObjects(t, testSpaceId, []spaceindex.TestObject{
		thirdObject,
		unsuitableObject,
	})

	fx.waitEvents(t,
		&pb.EventMessageValueOfObjectDetailsSet{
			ObjectDetailsSet: &pb.EventObjectDetailsSet{
				Id: "obj3",
				SubIds: []string{
					testSubId,
				},
				Details: thirdObject.Details().ToProto(),
			},
		},
		&pb.EventMessageValueOfSubscriptionAdd{
			SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
				SubId: testSubId,
				Id:    "obj3",
			},
		},
	)

	secondObjectEdited := spaceindex.TestObject{
		bundle.RelationKeyId:      domain.String("obj2"),
		bundle.RelationKeySpaceId: domain.String(testSpaceId),
		bundle.RelationKeyName:    domain.String("New name"),
	}
	fx.store.AddObjects(t, testSpaceId, []spaceindex.TestObject{
		secondObjectEdited,
	})

	fx.waitEvents(t,
		&pb.EventMessageValueOfObjectDetailsAmend{
			ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
				Id: "obj2",
				SubIds: []string{
					testSubId,
				},
				Details: []*pb.EventObjectDetailsAmendKeyValue{
					{
						Key:   bundle.RelationKeyName.String(),
						Value: domain.String("New name").ToProto(),
					},
				},
			},
		})
}
