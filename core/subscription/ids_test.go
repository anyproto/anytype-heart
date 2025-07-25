package subscription

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

func TestIdsSub(t *testing.T) {

	t.Run("basic test", func(t *testing.T) {
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
	})

	t.Run("dependencies", func(t *testing.T) {
		fx := newFixtureWithRealObjectStore(t)

		const testSubId = "subId"

		initialObjects := []spaceindex.TestObject{
			{
				bundle.RelationKeyId:      domain.String("obj1"),
				bundle.RelationKeySpaceId: domain.String(testSpaceId),
				bundle.RelationKeyName:    domain.String("Obj 1"),
				bundle.RelationKeyCreator: domain.String("creator"),
			},
			{
				bundle.RelationKeyId:      domain.String("creator"),
				bundle.RelationKeySpaceId: domain.String(testSpaceId),
				bundle.RelationKeyName:    domain.String("John Doe"),
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
				bundle.RelationKeyCreator.String(),
			},
			Ids: []string{"obj1", "obj2"},
		})
		require.NoError(t, err)

		wantRecords := []*types.Struct{
			initialObjects[0].Details().ToProto(),
		}
		assert.ElementsMatch(t, wantRecords, resp.Records)

		wantDeps := []*types.Struct{
			initialObjects[1].Details().ToProto(),
		}
		assert.ElementsMatch(t, wantDeps, resp.Dependencies)

		t.Run("update dependency", func(t *testing.T) {
			updatedDep := spaceindex.TestObject{
				bundle.RelationKeyId:      domain.String("creator"),
				bundle.RelationKeySpaceId: domain.String(testSpaceId),
				bundle.RelationKeyName:    domain.String("Jane Doe"),
			}
			fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{updatedDep})

			fx.waitEvents(t,
				&pb.EventMessageValueOfObjectDetailsAmend{
					ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
						Id: "creator",
						SubIds: []string{
							testSubId + "/dep",
						},
						Details: []*pb.EventObjectDetailsAmendKeyValue{
							{
								Key:   bundle.RelationKeyName.String(),
								Value: domain.String("Jane Doe").ToProto(),
							},
						},
					},
				})
		})

		t.Run("add the second object with dep", func(t *testing.T) {
			secondObject := spaceindex.TestObject{
				bundle.RelationKeyId:      domain.String("obj2"),
				bundle.RelationKeySpaceId: domain.String(testSpaceId),
				bundle.RelationKeyName:    domain.String("Obj 2"),
				bundle.RelationKeyCreator: domain.String("creator2"),
			}
			dependency := spaceindex.TestObject{
				bundle.RelationKeyId:      domain.String("creator2"),
				bundle.RelationKeySpaceId: domain.String(testSpaceId),
				bundle.RelationKeyName:    domain.String("Foobar"),
			}
			newObjects := []spaceindex.TestObject{
				secondObject,
				dependency,
			}
			fx.store.AddObjects(t, testSpaceId, newObjects)

			fx.waitEvents(t,
				&pb.EventMessageValueOfObjectDetailsSet{
					ObjectDetailsSet: &pb.EventObjectDetailsSet{
						Id: "obj2",
						SubIds: []string{
							testSubId,
						},
						Details: secondObject.Details().ToProto(),
					},
				},
				&pb.EventMessageValueOfObjectDetailsSet{
					ObjectDetailsSet: &pb.EventObjectDetailsSet{
						Id: "creator2",
						SubIds: []string{
							testSubId + "/dep",
						},
						Details: dependency.Details().ToProto(),
					},
				},
				&pb.EventMessageValueOfSubscriptionAdd{
					SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
						SubId: testSubId,
						Id:    "obj2",
					},
				},
				&pb.EventMessageValueOfSubscriptionAdd{
					SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
						SubId: testSubId + "/dep",
						Id:    "creator2",
					},
				},
			)
		})

		// thirdObject := spaceindex.TestObject{
		// 	bundle.RelationKeyId:      domain.String("obj3"),
		// 	bundle.RelationKeySpaceId: domain.String(testSpaceId),
		// }
		// unsuitableObject := spaceindex.TestObject{
		// 	bundle.RelationKeyId:      domain.String("obj4"),
		// 	bundle.RelationKeySpaceId: domain.String(testSpaceId),
		// }
		// fx.store.AddObjects(t, testSpaceId, []spaceindex.TestObject{
		// 	thirdObject,
		// 	unsuitableObject,
		// })
		//
		// fx.waitEvents(t,
		// 	&pb.EventMessageValueOfObjectDetailsSet{
		// 		ObjectDetailsSet: &pb.EventObjectDetailsSet{
		// 			Id: "obj3",
		// 			SubIds: []string{
		// 				testSubId,
		// 			},
		// 			Details: thirdObject.Details().ToProto(),
		// 		},
		// 	},
		// 	&pb.EventMessageValueOfSubscriptionAdd{
		// 		SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
		// 			SubId: testSubId,
		// 			Id:    "obj3",
		// 		},
		// 	},
		// )
		//
		// secondObjectEdited := spaceindex.TestObject{
		// 	bundle.RelationKeyId:      domain.String("obj2"),
		// 	bundle.RelationKeySpaceId: domain.String(testSpaceId),
		// 	bundle.RelationKeyName:    domain.String("New name"),
		// }
		// fx.store.AddObjects(t, testSpaceId, []spaceindex.TestObject{
		// 	secondObjectEdited,
		// })
		//
		// fx.waitEvents(t,
		// 	&pb.EventMessageValueOfObjectDetailsAmend{
		// 		ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
		// 			Id: "obj2",
		// 			SubIds: []string{
		// 				testSubId,
		// 			},
		// 			Details: []*pb.EventObjectDetailsAmendKeyValue{
		// 				{
		// 					Key:   bundle.RelationKeyName.String(),
		// 					Value: domain.String("New name").ToProto(),
		// 				},
		// 			},
		// 		},
		// 	})
	})
}
