package core

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/config"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/globalsign/mgo/bson"
	types2 "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
)

func getBlockById(id string, blocks []*model.Block) *model.Block {
	for _, block := range blocks {
		if block.Id == id {
			return block
		}
	}
	return nil
}

func getRelationByKey(relations []*pbrelation.Relation, key string) *pbrelation.Relation {
	for _, relation := range relations {
		if relation.Key == key {
			return relation
		}
	}
	return nil
}

func start(t *testing.T) (rootPath string, mw *Middleware) {
	mw = New()
	rootPath, err := ioutil.TempDir(os.TempDir(), "anytype_*")
	require.NoError(t, err)
	defer os.RemoveAll(rootPath)
	// override default config
	config.DefaultConfig.InMemoryDS = true
	config.DefaultConfig.Offline = true
	config.DefaultConfig.CafeP2PAddr = "-"
	config.DefaultConfig.CafeGRPCAddr = "-"

	mw.EventSender = event.NewCallbackSender(func(event *pb.Event) {
		// nothing to do
	})

	respWalletCreate := mw.WalletCreate(&pb.RpcWalletCreateRequest{RootPath: rootPath})
	require.Equal(t, 0, int(respWalletCreate.Error.Code), respWalletCreate.Error.Description)

	respAccountCreate := mw.AccountCreate(&pb.RpcAccountCreateRequest{Name: "profile", AlphaInviteCode: "elbrus"})
	require.Equal(t, 0, int(respAccountCreate.Error.Code), respAccountCreate.Error.Description)

	return rootPath, mw
}

func addRelation(t *testing.T, contextId string, mw *Middleware) (key string, name string) {
	name = bson.NewObjectId().String()
	respDataviewRelationAdd := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
		ContextId: contextId,
		BlockId:   "dataview",
		Relation: &pbrelation.Relation{
			Key:      "",
			Format:   0,
			Name:     name,
			ReadOnly: false,
		},
	})

	require.Equal(t, 0, int(respDataviewRelationAdd.Error.Code), respDataviewRelationAdd.Error.Description)
	key = respDataviewRelationAdd.RelationKey
	return
}

func TestRelationAdd(t *testing.T) {
	rootPath, mw := start(t)
	respOpenNewPage := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
	require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
	require.Len(t, respOpenNewPage.Event.Messages, 2)
	block := getBlockById("dataview", getEventBlockShow(respOpenNewPage.Event.Messages).Blocks)

	require.Len(t, block.GetDataview().Relations, len(bundle.MustGetType(bundle.TypeKeyPage).Relations))

	t.Run("add_incorrect", func(t *testing.T) {
		respDataviewRelationAdd := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Relation: &pbrelation.Relation{
				Key:      "name",
				Format:   0,
				Name:     "new",
				ReadOnly: false,
			},
		})
		require.Equal(t, pb.RpcBlockDataviewRelationAddResponseError_BAD_INPUT, respDataviewRelationAdd.Error.Code, respDataviewRelationAdd.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventBlockShow(respOpenNewPage.Event.Messages).Blocks)

		require.Len(t, block.GetDataview().Relations, len(bundle.MustGetType(bundle.TypeKeyPage).Relations))
	})

	t.Run("add_correct", func(t *testing.T) {
		respDataviewRelationAdd := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Relation: &pbrelation.Relation{
				Key:      "",
				Format:   0,
				Name:     "relation1",
				ReadOnly: false,
			},
		})

		require.Equal(t, 0, int(respDataviewRelationAdd.Error.Code), respDataviewRelationAdd.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventBlockShow(respOpenNewPage.Event.Messages).Blocks)
		require.Len(t, block.GetDataview().Relations, len(bundle.MustGetType(bundle.TypeKeyPage).Relations)+1)

		respAccountCreate := mw.AccountSelect(&pb.RpcAccountSelectRequest{Id: mw.Anytype.Account(), RootPath: rootPath})
		require.Equal(t, 0, int(respAccountCreate.Error.Code))
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventBlockShow(respOpenNewPage.Event.Messages).Blocks)
		require.Len(t, block.GetDataview().Relations, len(bundle.MustGetType(bundle.TypeKeyPage).Relations)+1)
	})

	t.Run("relation_scope_becomes_object", func(t *testing.T) {
		respPageCreate := mw.PageCreate(&pb.RpcPageCreateRequest{Details: &types2.Struct{Fields: map[string]*types2.Value{
			bundle.RelationKeyType.String(): pbtypes.StringList([]string{bundle.TypeKeyTask.URL()}),
		}}})

		require.Equal(t, 0, int(respPageCreate.Error.Code), respPageCreate.Error.Description)

		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: respPageCreate.PageId})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)

		blockShow := getEventBlockShow(respOpenNewPage.Event.Messages)

		for _, rel := range bundle.MustGetType(bundle.TypeKeyTask).Relations {
			var found bool
			for _, relInObj := range blockShow.Relations {
				if relInObj.Key == rel.Key {
					found = true
					break
				}
			}
			require.True(t, found, fmt.Errorf("missing %s(%s) relation", rel.Key, rel.Name))
			if rel.Key == bundle.RelationKeyStatus.String() {
				require.Equal(t, pbrelation.Relation_type, rel.Scope, fmt.Errorf("relation '%s' has scope %s instead of %s", rel.Name, rel.Scope.String(), pbrelation.Relation_type.String()))
			}
		}

		mw.BlockSetDetails(&pb.RpcBlockSetDetailsRequest{ContextId: respPageCreate.PageId, Details: []*pb.RpcBlockSetDetailsDetail{
			{Key: bundle.RelationKeyStatus.String(), Value: pbtypes.String("Done")},
		}})

		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: respPageCreate.PageId})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)

		blockShow = getEventBlockShow(respOpenNewPage.Event.Messages)
		for _, rel := range bundle.MustGetType(bundle.TypeKeyTask).Relations {
			var found bool
			for _, relInObj := range blockShow.Relations {
				if relInObj.Key == rel.Key {
					found = true
					if rel.Key == bundle.RelationKeyStatus.String() {
						require.Equal(t, pbrelation.Relation_object, relInObj.Scope, fmt.Errorf("relation '%s' has scope %s instead of %s", rel.Name, rel.Scope.String(), pbrelation.Relation_object.String()))
					}
					break
				}
			}
			require.True(t, found, fmt.Errorf("missing %s(%s) relation", rel.Key, rel.Name))

		}
	})

	t.Run("update_not_existing", func(t *testing.T) {
		respUpdate := mw.BlockDataviewRelationUpdate(&pb.RpcBlockDataviewRelationUpdateRequest{
			ContextId:   mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: "not_existing_key",
			Relation:    &pbrelation.Relation{Key: "ffff"},
		})
		require.Equal(t, pb.RpcBlockDataviewRelationUpdateResponseError_BAD_INPUT, respUpdate.Error.Code, respUpdate.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventBlockShow(respOpenNewPage.Event.Messages).Blocks)
	})

	t.Run("update_cant_change_format", func(t *testing.T) {
		relKey, relName := addRelation(t, mw.Anytype.PredefinedBlocks().SetPages, mw)
		respUpdate := mw.BlockDataviewRelationUpdate(&pb.RpcBlockDataviewRelationUpdateRequest{
			ContextId:   mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: relKey,
			Relation: &pbrelation.Relation{
				Key:      relKey,
				Format:   1,
				Name:     "relation1_changed",
				ReadOnly: false,
			},
		})
		require.Equal(t, pb.RpcBlockDataviewRelationUpdateResponseError_BAD_INPUT, respUpdate.Error.Code, respUpdate.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventBlockShow(respOpenNewPage.Event.Messages).Blocks)

		require.Equal(t, relName, block.GetDataview().Relations[len(block.GetDataview().Relations)-1].Name)
	})

	t.Run("update_correct", func(t *testing.T) {
		relKey, _ := addRelation(t, mw.Anytype.PredefinedBlocks().SetPages, mw)

		respUpdate := mw.BlockDataviewRelationUpdate(&pb.RpcBlockDataviewRelationUpdateRequest{
			ContextId:   mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: relKey,
			Relation: &pbrelation.Relation{
				Key:      relKey,
				Format:   0,
				Name:     "new_changed",
				ReadOnly: false,
			},
		})
		require.Equal(t, pb.RpcBlockDataviewRelationUpdateResponseError_NULL, respUpdate.Error.Code, respUpdate.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventBlockShow(respOpenNewPage.Event.Messages).Blocks)

		require.Equal(t, "new_changed", block.GetDataview().Relations[len(block.GetDataview().Relations)-1].Name)
	})

	t.Run("delete_incorrect", func(t *testing.T) {
		respDataviewRelationAdd := mw.BlockDataviewRelationDelete(&pb.RpcBlockDataviewRelationDeleteRequest{
			ContextId:   mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: "not_existing_key",
		})
		require.Equal(t, pb.RpcBlockDataviewRelationDeleteResponseError_BAD_INPUT, respDataviewRelationAdd.Error.Code, respDataviewRelationAdd.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventBlockShow(respOpenNewPage.Event.Messages).Blocks)
	})

	t.Run("delete_correct", func(t *testing.T) {
		relKey, _ := addRelation(t, mw.Anytype.PredefinedBlocks().SetPages, mw)

		respDataviewRelationDelete := mw.BlockDataviewRelationDelete(&pb.RpcBlockDataviewRelationDeleteRequest{
			ContextId:   mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: relKey,
		})
		//mw.blocksService.Close()
		respAccountCreate := mw.AccountSelect(&pb.RpcAccountSelectRequest{Id: mw.Anytype.Account(), RootPath: rootPath})
		require.Equal(t, 0, int(respAccountCreate.Error.Code))

		require.Equal(t, 0, int(respDataviewRelationDelete.Error.Code), respDataviewRelationDelete.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventBlockShow(respOpenNewPage.Event.Messages).Blocks)

		require.Nil(t, getRelationByKey(block.GetDataview().Relations, relKey))
	})

	t.Run("relation_add_select_option", func(t *testing.T) {
		respRelCreate := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Relation: &pbrelation.Relation{
				Format: pbrelation.RelationFormat_status,
				SelectDict: []*pbrelation.RelationOption{{
					Text:  "opt1",
					Color: "red",
				}},
				Name:     "relation2",
				ReadOnly: false,
			},
		})
		require.Equal(t, 0, int(respRelCreate.Error.Code), respRelCreate.Error.Description)
		rel := respRelCreate.Event.GetMessages()[0].GetBlockDataviewRelationSet()
		require.Equal(t, respRelCreate.RelationKey, rel.RelationKey)
		// option is trimmed
		require.Len(t, rel.Relation.SelectDict, 0)

		respRecordCreate := mw.BlockDataviewRecordCreate(
			&pb.RpcBlockDataviewRecordCreateRequest{
				ContextId: mw.Anytype.PredefinedBlocks().SetPages,
				BlockId:   "dataview",
			})

		require.Equal(t, 0, int(respRecordCreate.Error.Code), respRecordCreate.Error.Description)
		newPageId := respRecordCreate.Record.Fields["id"].GetStringValue()
		respRelOptCreate := mw.BlockDataviewRecordRelationOptionAdd(&pb.RpcBlockDataviewRecordRelationOptionAddRequest{
			ContextId: mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Option: &pbrelation.RelationOption{
				Text:  "opt1",
				Color: "red",
			},
			RecordId:    newPageId,
			RelationKey: respRelCreate.RelationKey,
		})
		require.Equal(t, 0, int(respRelOptCreate.Error.Code), respRelOptCreate.Error.Description)
		time.Sleep(time.Second * 1)

		respRecordUpdate := mw.BlockDataviewRecordUpdate(
			&pb.RpcBlockDataviewRecordUpdateRequest{
				ContextId: mw.Anytype.PredefinedBlocks().SetPages,
				BlockId:   "dataview",
				RecordId:  newPageId,
				Record: &types2.Struct{
					Fields: map[string]*types2.Value{
						rel.Relation.Key: pbtypes.StringList([]string{respRelOptCreate.Option.Id}),
					},
				},
			})

		require.Equal(t, 0, int(respRecordUpdate.Error.Code), respRecordUpdate.Error.Description)

		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: newPageId})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 1)

		relOnPage := getRelationByKey(getEventBlockShow(respOpenNewPage.Event.Messages).Relations, rel.RelationKey)
		require.Equal(t, rel.Relation.Key, relOnPage.Key)

		respOptAdd := mw.BlockDataviewRecordRelationOptionAdd(&pb.RpcBlockDataviewRecordRelationOptionAddRequest{
			ContextId:   mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: rel.RelationKey,
			RecordId:    newPageId,
			Option: &pbrelation.RelationOption{
				Text:  "opt2",
				Color: "green",
			},
		})

		require.Equal(t, 0, int(respOptAdd.Error.Code), respOptAdd.Error.Description)
		time.Sleep(time.Second)

		respRecordUpdate2 := mw.BlockDataviewRecordUpdate(
			&pb.RpcBlockDataviewRecordUpdateRequest{
				ContextId: mw.Anytype.PredefinedBlocks().SetPages,
				BlockId:   "dataview",
				RecordId:  newPageId,
				Record: &types2.Struct{
					Fields: map[string]*types2.Value{
						rel.Relation.Key: pbtypes.String(respOptAdd.Option.Id),
					},
				},
			})
		require.Equal(t, 0, int(respRecordUpdate2.Error.Code), respRecordUpdate2.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: newPageId})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 1)

		rel.Relation.SelectDict = append(rel.Relation.SelectDict, respOptAdd.Option)
		relOnPage = getRelationByKey(getEventBlockShow(respOpenNewPage.Event.Messages).Relations, rel.RelationKey)
		require.Equal(t, rel.Relation.Key, relOnPage.Key)
		require.Len(t, relOnPage.SelectDict, 2)
	})

	t.Run("aggregated_options", func(t *testing.T) {
		type test struct {
			relKey             string
			mustHaveOptions    map[string]pbrelation.RelationOptionScope
			mustNotHaveOptions []string
		}

		rel1 := &pbrelation.Relation{
			Format:   pbrelation.RelationFormat_status,
			Name:     "ao_relation_status1",
			ReadOnly: false,
		}
		rel2 := &pbrelation.Relation{
			Format:   pbrelation.RelationFormat_status,
			Name:     "ao_relation_status2",
			ReadOnly: false,
		}
		rel3 := &pbrelation.Relation{
			Format:   pbrelation.RelationFormat_tag,
			Name:     "ao_relation_tag3",
			ReadOnly: false,
		}

		respPage1Create := mw.PageCreate(&pb.RpcPageCreateRequest{
			Details: &types2.Struct{Fields: map[string]*types2.Value{
				bundle.RelationKeyType.String(): pbtypes.StringList([]string{bundle.TypeKeyPage.URL()}),
			}},
		})
		require.Equal(t, 0, int(respPage1Create.Error.Code), respPage1Create.Error.Description)
		respPage2Create := mw.PageCreate(&pb.RpcPageCreateRequest{
			Details: &types2.Struct{Fields: map[string]*types2.Value{
				bundle.RelationKeyType.String(): pbtypes.StringList([]string{bundle.TypeKeyPage.URL()}),
			}},
		})
		require.Equal(t, 0, int(respPage1Create.Error.Code), respPage1Create.Error.Description)
		respTask1Create := mw.PageCreate(&pb.RpcPageCreateRequest{
			Details: &types2.Struct{Fields: map[string]*types2.Value{
				bundle.RelationKeyType.String(): pbtypes.StringList([]string{bundle.TypeKeyTask.URL()}),
			}},
		})
		require.Equal(t, 0, int(respTask1Create.Error.Code), respTask1Create.Error.Description)

		respRelAdd1 := mw.ObjectRelationAdd(&pb.RpcObjectRelationAddRequest{
			ContextId: respPage1Create.PageId,
			Relation:  rel1,
		})
		require.Equal(t, 0, int(respRelAdd1.Error.Code), respRelAdd1.Error.Description)
		rel1.Key = respRelAdd1.RelationKey

		respRelAdd1_2 := mw.ObjectRelationAdd(&pb.RpcObjectRelationAddRequest{
			ContextId: respPage2Create.PageId,
			Relation:  rel1,
		})
		require.Equal(t, 0, int(respRelAdd1_2.Error.Code), respRelAdd1_2.Error.Description)

		respRelAdd2 := mw.ObjectRelationAdd(&pb.RpcObjectRelationAddRequest{
			ContextId: respTask1Create.PageId,
			Relation:  rel2,
		})
		require.Equal(t, 0, int(respRelAdd2.Error.Code), respRelAdd2.Error.Description)
		rel2.Key = respRelAdd2.RelationKey

		respRelAdd2_2 := mw.ObjectRelationAdd(&pb.RpcObjectRelationAddRequest{
			ContextId: respPage2Create.PageId,
			Relation:  rel2,
		})
		require.Equal(t, 0, int(respRelAdd2_2.Error.Code), respRelAdd2_2.Error.Description)

		respRelAdd3 := mw.ObjectRelationAdd(&pb.RpcObjectRelationAddRequest{
			ContextId: respPage2Create.PageId,
			Relation:  rel3,
		})
		require.Equal(t, 0, int(respRelAdd3.Error.Code), respRelAdd3.Error.Description)
		rel3.Key = respRelAdd3.RelationKey
		time.Sleep(time.Second)

		respOptionAdd1 := mw.ObjectRelationOptionAdd(&pb.RpcObjectRelationOptionAddRequest{
			ContextId:   respPage1Create.PageId,
			RelationKey: rel1.Key,
			Option: &pbrelation.RelationOption{
				Id:    "ao_opt1_id",
				Text:  "ao_opt1",
				Color: "red",
			},
		})
		require.Equal(t, 0, int(respOptionAdd1.Error.Code), respOptionAdd1.Error.Description)

		// same rel format different object type
		respOptionAdd2 := mw.ObjectRelationOptionAdd(&pb.RpcObjectRelationOptionAddRequest{
			ContextId:   respTask1Create.PageId,
			RelationKey: rel2.Key,
			Option: &pbrelation.RelationOption{
				Id:    "ao_opt2_id",
				Text:  "ao_opt2",
				Color: "green",
			},
		})
		require.Equal(t, 0, int(respOptionAdd2.Error.Code), respOptionAdd2.Error.Description)

		respOptionAdd3 := mw.ObjectRelationOptionAdd(&pb.RpcObjectRelationOptionAddRequest{
			ContextId:   respPage2Create.PageId,
			RelationKey: rel1.Key,
			Option: &pbrelation.RelationOption{
				Id:    "ao_opt3_id",
				Text:  "ao_opt3",
				Color: "green",
			},
		})
		require.Equal(t, 0, int(respOptionAdd3.Error.Code), respOptionAdd3.Error.Description)

		respOptionAdd4 := mw.ObjectRelationOptionAdd(&pb.RpcObjectRelationOptionAddRequest{
			ContextId:   respPage2Create.PageId,
			RelationKey: rel2.Key,
			Option: &pbrelation.RelationOption{
				Id:    "ao_opt4_id",
				Text:  "ao_opt4",
				Color: "green",
			},
		})
		require.Equal(t, 0, int(respOptionAdd4.Error.Code), respOptionAdd4.Error.Description)

		respOptionAdd5 := mw.ObjectRelationOptionAdd(&pb.RpcObjectRelationOptionAddRequest{
			ContextId:   respPage2Create.PageId,
			RelationKey: rel3.Key,
			Option: &pbrelation.RelationOption{
				Id:    "ao_opt5_id",
				Text:  "ao_opt5",
				Color: "green",
			},
		})
		require.Equal(t, 0, int(respOptionAdd5.Error.Code), respOptionAdd5.Error.Description)
		time.Sleep(time.Second * 2)
		tests := []test{
			{
				rel1.Key,
				map[string]pbrelation.RelationOptionScope{
					"ao_opt1_id": pbrelation.RelationOption_local,
					"ao_opt3_id": pbrelation.RelationOption_local,
					"ao_opt4_id": pbrelation.RelationOption_format,
					"ao_opt2_id": pbrelation.RelationOption_format,
				},
				[]string{"ao_opt5_id"},
			},
			{
				rel2.Key,
				map[string]pbrelation.RelationOptionScope{
					"ao_opt4_id": pbrelation.RelationOption_local,
					"ao_opt1_id": pbrelation.RelationOption_format,
					"ao_opt2_id": pbrelation.RelationOption_relation,
					"ao_opt3_id": pbrelation.RelationOption_format,
				},
				[]string{"ao_opt5_id"},
			},
			{
				rel3.Key,
				map[string]pbrelation.RelationOptionScope{
					"ao_opt5_id": pbrelation.RelationOption_local,
				},
				[]string{"ao_opt1_id", "ao_opt2_id", "ao_opt3_id", "ao_opt4_id"},
			},
		}
		respDvRelAdd := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Relation:  rel1,
		})
		require.Equal(t, 0, int(respDvRelAdd.Error.Code), respDvRelAdd.Error.Description)

		respDvRelAdd = mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Relation:  rel2,
		})
		require.Equal(t, 0, int(respDvRelAdd.Error.Code), respDvRelAdd.Error.Description)

		respDvRelAdd = mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Relation:  rel3,
		})
		require.Equal(t, 0, int(respDvRelAdd.Error.Code), respDvRelAdd.Error.Description)

		respOpenNewPage := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		block := getBlockById("dataview", getEventBlockShow(respOpenNewPage.Event.Messages).Blocks)
		for _, test := range tests {
			var relFound bool
			for _, rel := range block.GetDataview().Relations {
				if rel.Key != test.relKey {
					continue
				}
				relFound = true
				for optId, optScope := range test.mustHaveOptions {
					var found bool
					for _, opt := range rel.SelectDict {
						if opt.Id != optId {
							continue
						}
						found = true
						require.Equal(t, optScope, opt.Scope, "required opt %s on rel %s(%s) should has %s scope, got %s instead", optId, rel.Key, rel.Name, optScope.String(), opt.Scope)
					}
					require.True(t, found, "required opt %s not found for rel %s(%s)", optId, rel.Key, rel.Name)
				}
				for _, optId := range test.mustNotHaveOptions {
					var found bool
					for _, opt := range rel.SelectDict {
						if opt.Id == optId {
							found = true
						}
					}
					require.False(t, found, "opt %s should not be found for rel %s(%s)", optId, rel.Key, rel.Name)
				}
			}
			require.True(t, relFound, "aggregated options for relation %s(%s) not found", test.relKey)
		}
	})
}

func TestCustomType(t *testing.T) {
	_, mw := start(t)

	respObjectTypeList := mw.ObjectTypeList(nil)
	require.Equal(t, 0, int(respObjectTypeList.Error.Code), respObjectTypeList.Error.Description)

	respObjectTypeCreate := mw.ObjectTypeCreate(&pb.RpcObjectTypeCreateRequest{
		ObjectType: &pbrelation.ObjectType{
			Name: "1",
			Relations: []*pbrelation.Relation{
				{Format: pbrelation.RelationFormat_date, Name: "date of birth"},
				{Format: pbrelation.RelationFormat_object, Name: "assignee", ObjectTypes: []string{"https://anytype.io/schemas/object/bundled/page"}},
				{Format: pbrelation.RelationFormat_description, Name: "bio"},
			},
		},
	})

	require.Equal(t, 0, int(respObjectTypeCreate.Error.Code), respObjectTypeCreate.Error.Description)
	require.Len(t, respObjectTypeCreate.ObjectType.Relations, len(bundle.RequiredInternalRelations)+3) // including relation.RequiredInternalRelations
	require.True(t, strings.HasPrefix(respObjectTypeCreate.ObjectType.Url, "https://anytype.io/schemas/object/custom/"))
	var newRelation *pbrelation.Relation
	for _, rel := range respObjectTypeCreate.ObjectType.Relations {
		if rel.Name == "bio" {
			newRelation = rel
			break
		}
	}

	respObjectTypeList = mw.ObjectTypeList(nil)
	require.Equal(t, 0, int(respObjectTypeList.Error.Code), respObjectTypeList.Error.Description)
	lastObjType := respObjectTypeList.ObjectTypes[len(respObjectTypeList.ObjectTypes)-1]
	require.Equal(t, respObjectTypeCreate.ObjectType.Url, lastObjType.Url)
	require.Len(t, lastObjType.Relations, len(bundle.RequiredInternalRelations)+3)

	respCreateCustomTypeSet := mw.SetCreate(&pb.RpcSetCreateRequest{
		ObjectTypeUrl: respObjectTypeCreate.ObjectType.Url,
	})
	require.Equal(t, 0, int(respCreateCustomTypeSet.Error.Code), respCreateCustomTypeSet.Error.Description)
	require.NotEmpty(t, respCreateCustomTypeSet.Id)

	respOpenCustomTypeSet := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: respCreateCustomTypeSet.Id})
	require.Equal(t, 0, int(respOpenCustomTypeSet.Error.Code), respOpenCustomTypeSet.Error.Description)

	respCreateRecordInCustomTypeSet := mw.BlockDataviewRecordCreate(&pb.RpcBlockDataviewRecordCreateRequest{ContextId: respCreateCustomTypeSet.Id, BlockId: "dataview", Record: &types2.Struct{Fields: map[string]*types2.Value{"name": pbtypes.String("custom1"), newRelation.Key: pbtypes.String("newRelationVal")}}})
	require.Equal(t, 0, int(respCreateRecordInCustomTypeSet.Error.Code), respCreateRecordInCustomTypeSet.Error.Description)

	customObjectId := respCreateRecordInCustomTypeSet.Record.Fields["id"].GetStringValue()
	respOpenCustomTypeObject := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: customObjectId})
	require.Equal(t, 0, int(respOpenCustomTypeObject.Error.Code), respOpenCustomTypeObject.Error.Description)
	require.Len(t, respOpenCustomTypeObject.Event.Messages, 1)
	show := getEventBlockShow(respOpenCustomTypeObject.Event.Messages)
	require.NotNil(t, show)

	profile := getDetailsForContext(show.Details, mw.Anytype.PredefinedBlocks().Profile)
	require.NotNil(t, profile)
	// should have custom obj type + profile, because it has the relation `creator`
	require.Len(t, show.ObjectTypes, 2)
	require.Len(t, show.ObjectTypePerObject, 2)
	var found bool
	for _, ot := range show.ObjectTypes {
		if ot.Url == respObjectTypeCreate.ObjectType.Url {
			// omit relations
			respObjectTypeCreate.ObjectType.Relations = nil
			require.Equal(t, respObjectTypeCreate.ObjectType, ot)
			found = true
		}
	}
	require.True(t, found, "required custom obj type not found")

	var customObjectDetails = getDetailsForContext(show.Details, customObjectId)
	require.NotNil(t, customObjectDetails)
	require.Equal(t, mw.Anytype.PredefinedBlocks().Profile, pbtypes.GetString(customObjectDetails, bundle.RelationKeyCreator.String()))
	rel := getRelationByKey(show.Relations, newRelation.Key)
	require.NotNil(t, rel)
	require.Equal(t, newRelation, rel)

	require.NotNil(t, customObjectDetails.Fields[newRelation.Key])
	require.Equal(t, "newRelationVal", customObjectDetails.Fields[newRelation.Key].GetStringValue())
	for i := 0; i <= 20; i++ {
		respOpenCustomTypeSet = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: respCreateCustomTypeSet.Id})
		require.Equal(t, 0, int(respOpenCustomTypeSet.Error.Code), respOpenCustomTypeSet.Error.Description)

		recordsSet := getEventRecordsSet(respOpenCustomTypeSet.Event.Messages)
		require.NotNil(t, recordsSet)
		if len(recordsSet.Records) == 0 {
			if i < 20 {
				time.Sleep(time.Millisecond * 200)
				continue
			}
		}

		require.Equal(t, 1, len(recordsSet.Records))
		require.Equal(t, getEventRecordsSet(respOpenCustomTypeSet.Event.Messages).Records[0].Fields["id"].GetStringValue(), respCreateRecordInCustomTypeSet.Record.Fields["id"].GetStringValue())
	}
	show = getEventBlockShow(respOpenCustomTypeSet.Event.Messages)
	require.NotNil(t, show)

	respSearch := mw.ObjectSearch(&pb.RpcObjectSearchRequest{Filters: []*model.BlockContentDataviewFilter{{
		RelationKey: "type",
		Condition:   model.BlockContentDataviewFilter_Equal,
		Value:       pbtypes.String(respObjectTypeCreate.ObjectType.Url),
	}}})
	require.Equal(t, 0, int(respSearch.Error.Code), respSearch.Error.Description)
	require.Len(t, respSearch.Records, 1)
}

func TestBundledType(t *testing.T) {

	_, mw := start(t)

	respCreatePage := mw.PageCreate(&pb.RpcPageCreateRequest{Details: &types2.Struct{Fields: map[string]*types2.Value{"name": pbtypes.String("test1")}}})
	require.Equal(t, 0, int(respCreatePage.Error.Code), respCreatePage.Error.Description)
	time.Sleep(time.Second)
	respOpenPagesSet := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
	require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)

	show := getEventBlockShow(respOpenPagesSet.Event.Messages)
	require.NotNil(t, show)

	recordsSet := getEventRecordsSet(respOpenPagesSet.Event.Messages)
	require.NotNil(t, recordsSet)

	require.Len(t, recordsSet.Records, 1)
	require.Equal(t, respCreatePage.PageId, getEventRecordsSet(respOpenPagesSet.Event.Messages).Records[0].Fields["id"].GetStringValue())

	respCreatePage = mw.PageCreate(&pb.RpcPageCreateRequest{Details: &types2.Struct{Fields: map[string]*types2.Value{"name": pbtypes.String("test2")}}})
	require.Equal(t, 0, int(respCreatePage.Error.Code), respCreatePage.Error.Description)

	time.Sleep(time.Second)
	respOpenPagesSet = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
	require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)

	show = getEventBlockShow(respOpenPagesSet.Event.Messages)
	require.NotNil(t, show)
	require.Len(t, getEventRecordsSet(respOpenPagesSet.Event.Messages).Records, 2)

	require.True(t, hasRecordWithKeyAndVal(getEventRecordsSet(respOpenPagesSet.Event.Messages).Records, "id", respCreatePage.PageId))
}

func hasRecordWithKeyAndVal(recs []*types2.Struct, key string, val string) bool {
	for _, rec := range recs {
		if pbtypes.GetString(rec, key) == val {
			return true
		}
	}
	return false
}

func getEventRecordsSet(msgs []*pb.EventMessage) *pb.EventBlockDataviewRecordsSet {
	for _, msg := range msgs {
		if v, ok := msg.Value.(*pb.EventMessageValueOfBlockDataviewRecordsSet); ok {
			return v.BlockDataviewRecordsSet
		}
	}
	return nil
}

func getEventBlockShow(msgs []*pb.EventMessage) *pb.EventBlockShow {
	for _, msg := range msgs {
		if v, ok := msg.Value.(*pb.EventMessageValueOfBlockShow); ok {
			return v.BlockShow
		}
	}
	return nil
}

func getDetailsForContext(msgs []*pb.EventBlockSetDetails, contextId string) *types2.Struct {
	for _, msg := range msgs {
		if msg.Id == contextId {
			return msg.Details
		}
	}
	return nil
}
