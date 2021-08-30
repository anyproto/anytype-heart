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
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
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

func getRelationByKey(relations []*model.Relation, key string) *model.Relation {
	for _, relation := range relations {
		if relation.Key == key {
			return relation
		}
	}
	return nil
}

func start(t *testing.T, eventSender event.Sender) (rootPath string, mw *Middleware) {
	mw = New()
	rootPath, err := ioutil.TempDir(os.TempDir(), "anytype_*")
	require.NoError(t, err)
	defer os.RemoveAll(rootPath)

	if eventSender == nil {
		eventSender = event.NewCallbackSender(func(event *pb.Event) {
			log.Debugf("got event: %s", pbtypes.Sprint(event))
		})
	}

	mw.EventSender = eventSender

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
		Relation: &model.Relation{
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
	eventHandler := func(event *pb.Event) {
		return
	}

	rootPath, mw := start(t, event.NewCallbackSender(func(event *pb.Event) {
		eventHandler(event)
	}))

	respOpenNewPage := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.GetAnytype().PredefinedBlocks().SetPages})
	require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
	require.Len(t, respOpenNewPage.Event.Messages, 2)
	block := getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)

	require.Len(t, block.GetDataview().Relations, len(bundle.MustGetType(bundle.TypeKeyPage).Relations))

	t.Run("add_incorrect", func(t *testing.T) {
		respDataviewRelationAdd := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Relation: &model.Relation{
				Key:      "name",
				Format:   0,
				Name:     "new",
				ReadOnly: false,
			},
		})
		require.Equal(t, pb.RpcBlockDataviewRelationAddResponseError_BAD_INPUT, respDataviewRelationAdd.Error.Code, respDataviewRelationAdd.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.GetAnytype().PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)

		require.Len(t, block.GetDataview().Relations, len(bundle.MustGetType(bundle.TypeKeyPage).Relations))
	})

	t.Run("add_correct", func(t *testing.T) {
		respDataviewRelationAdd := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Relation: &model.Relation{
				Key:      "",
				Format:   0,
				Name:     "relation1",
				ReadOnly: false,
			},
		})

		require.Equal(t, 0, int(respDataviewRelationAdd.Error.Code), respDataviewRelationAdd.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.GetAnytype().PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)
		require.Len(t, block.GetDataview().Relations, len(bundle.MustGetType(bundle.TypeKeyPage).Relations)+1)

		respAccountCreate := mw.AccountSelect(&pb.RpcAccountSelectRequest{Id: mw.GetAnytype().Account(), RootPath: rootPath})
		require.Equal(t, 0, int(respAccountCreate.Error.Code))
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.GetAnytype().PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)
		require.Len(t, block.GetDataview().Relations, len(bundle.MustGetType(bundle.TypeKeyPage).Relations)+1)
	})

	t.Run("relation_aggregate_scope", func(t *testing.T) {
		respSet1 := mw.SetCreate(&pb.RpcSetCreateRequest{
			ObjectTypeUrl: bundle.TypeKeyIdea.URL(),
			Details:       nil,
		})
		require.Equal(t, 0, int(respSet1.Error.Code), respSet1.Error.Description)

		respSet2 := mw.SetCreate(&pb.RpcSetCreateRequest{
			ObjectTypeUrl: bundle.TypeKeyIdea.URL(),
			Details:       nil,
		})
		require.Equal(t, 0, int(respSet2.Error.Code), respSet2.Error.Description)

		respSetRelCreate1 := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: respSet1.Id,
			BlockId:   "dataview",
			Relation:  &model.Relation{Format: model.RelationFormat_shorttext, Name: "from set1"},
		})
		require.Equal(t, 0, int(respSetRelCreate1.Error.Code), respSetRelCreate1.Error.Description)

		respSetRelCreate2 := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: respSet2.Id,
			BlockId:   "dataview",
			Relation:  &model.Relation{Format: model.RelationFormat_shorttext, Name: "from set2"},
		})
		require.Equal(t, 0, int(respSetRelCreate2.Error.Code), respSetRelCreate2.Error.Description)

		respPageCreate := mw.PageCreate(&pb.RpcPageCreateRequest{Details: &types2.Struct{Fields: map[string]*types2.Value{
			bundle.RelationKeyType.String(): pbtypes.String(bundle.TypeKeyIdea.URL()),
		}}})
		require.Equal(t, 0, int(respPageCreate.Error.Code), respPageCreate.Error.Description)

		time.Sleep(time.Millisecond * 1000)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: respPageCreate.PageId})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)

		blockShow := getEventObjectShow(respOpenNewPage.Event.Messages)
		log.Debugf("block relations: %v", blockShow.Relations)

		relFromSet1 := pbtypes.GetRelation(blockShow.Relations, respSetRelCreate1.RelationKey)
		require.NotNil(t, relFromSet1)
		require.Equal(t, model.Relation_setOfTheSameType, relFromSet1.Scope)
		relFromSet2 := pbtypes.GetRelation(blockShow.Relations, respSetRelCreate2.RelationKey)
		require.NotNil(t, relFromSet2)
		require.Equal(t, model.Relation_setOfTheSameType, relFromSet2.Scope)
	})

	t.Run("relation_scope_becomes_object", func(t *testing.T) {
		respPageCreate := mw.PageCreate(&pb.RpcPageCreateRequest{Details: &types2.Struct{Fields: map[string]*types2.Value{
			bundle.RelationKeyType.String(): pbtypes.StringList([]string{bundle.TypeKeyTask.URL()}),
		}}})

		require.Equal(t, 0, int(respPageCreate.Error.Code), respPageCreate.Error.Description)

		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: respPageCreate.PageId})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)

		blockShow := getEventObjectShow(respOpenNewPage.Event.Messages)

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
				require.Equal(t, model.Relation_type, rel.Scope, fmt.Errorf("relation '%s' has scope %s instead of %s", rel.Name, rel.Scope.String(), model.Relation_type.String()))
			}
		}

		optionAddResp := mw.ObjectRelationOptionAdd(&pb.RpcObjectRelationOptionAddRequest{
			ContextId:   respPageCreate.PageId,
			RelationKey: bundle.RelationKeyStatus.String(),
			Option: &model.RelationOption{
				Text:  "Done",
				Color: "red",
			},
		})
		require.Equal(t, 0, int(optionAddResp.Error.Code), optionAddResp.Error.Description)

		setDetailsResp := mw.BlockSetDetails(&pb.RpcBlockSetDetailsRequest{ContextId: respPageCreate.PageId, Details: []*pb.RpcBlockSetDetailsDetail{
			{Key: bundle.RelationKeyStatus.String(), Value: pbtypes.StringList([]string{optionAddResp.Option.Id})},
		}})
		require.Equal(t, 0, int(setDetailsResp.Error.Code), setDetailsResp.Error.Description)

		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: respPageCreate.PageId})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)

		blockShow = getEventObjectShow(respOpenNewPage.Event.Messages)
		for _, rel := range bundle.MustGetType(bundle.TypeKeyTask).Relations {
			var found bool
			for _, relInObj := range blockShow.Relations {
				if relInObj.Key == rel.Key {
					found = true
					if rel.Key == bundle.RelationKeyStatus.String() {
						require.Equal(t, model.Relation_object, relInObj.Scope, fmt.Errorf("relation '%s' has scope %s instead of %s", rel.Name, rel.Scope.String(), model.Relation_object.String()))
					}
					break
				}
			}
			require.True(t, found, fmt.Errorf("missing %s(%s) relation", rel.Key, rel.Name))

		}
	})

	t.Run("update_not_existing", func(t *testing.T) {
		respUpdate := mw.BlockDataviewRelationUpdate(&pb.RpcBlockDataviewRelationUpdateRequest{
			ContextId:   mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: "not_existing_key",
			Relation:    &model.Relation{Key: "ffff"},
		})
		require.Equal(t, pb.RpcBlockDataviewRelationUpdateResponseError_BAD_INPUT, respUpdate.Error.Code, respUpdate.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.GetAnytype().PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)
	})

	t.Run("update_cant_change_format", func(t *testing.T) {
		relKey, relName := addRelation(t, mw.GetAnytype().PredefinedBlocks().SetPages, mw)
		respUpdate := mw.BlockDataviewRelationUpdate(&pb.RpcBlockDataviewRelationUpdateRequest{
			ContextId:   mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: relKey,
			Relation: &model.Relation{
				Key:      relKey,
				Format:   1,
				Name:     "relation1_changed",
				ReadOnly: false,
			},
		})
		require.Equal(t, pb.RpcBlockDataviewRelationUpdateResponseError_BAD_INPUT, respUpdate.Error.Code, respUpdate.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.GetAnytype().PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)

		require.Equal(t, relName, block.GetDataview().Relations[len(block.GetDataview().Relations)-1].Name)
	})

	t.Run("update_correct", func(t *testing.T) {
		relKey, _ := addRelation(t, mw.GetAnytype().PredefinedBlocks().SetPages, mw)

		respUpdate := mw.BlockDataviewRelationUpdate(&pb.RpcBlockDataviewRelationUpdateRequest{
			ContextId:   mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: relKey,
			Relation: &model.Relation{
				Key:      relKey,
				Format:   0,
				Name:     "new_changed",
				ReadOnly: false,
			},
		})
		require.Equal(t, pb.RpcBlockDataviewRelationUpdateResponseError_NULL, respUpdate.Error.Code, respUpdate.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.GetAnytype().PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)

		require.Equal(t, "new_changed", block.GetDataview().Relations[len(block.GetDataview().Relations)-1].Name)
	})

	t.Run("update_relation_name_in_set_expect_change_in_object", func(t *testing.T) {
		relKey, _ := addRelation(t, mw.GetAnytype().PredefinedBlocks().SetPages, mw)

		recCreate := mw.BlockDataviewRecordCreate(&pb.RpcBlockDataviewRecordCreateRequest{
			ContextId:   mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			Record:     nil,
			TemplateId: "",
		})
		respUpdate := mw.BlockDataviewRelationUpdate(&pb.RpcBlockDataviewRelationUpdateRequest{
			ContextId:   mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: relKey,
			Relation: &model.Relation{
				Key:      relKey,
				Format:   0,
				Name:     "new_changed",
				ReadOnly: false,
			},
		})
		require.Equal(t, pb.RpcBlockDataviewRelationUpdateResponseError_NULL, respUpdate.Error.Code, respUpdate.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.GetAnytype().PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)

		require.Equal(t, "new_changed", block.GetDataview().Relations[len(block.GetDataview().Relations)-1].Name)
		time.Sleep(time.Second*3)
		respOpenNewRecord := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: pbtypes.GetString(recCreate.Record, "id")})
		rel := pbtypes.GetRelation(getEventObjectShow(respOpenNewRecord.Event.Messages).Relations, relKey)
		require.NotNil(t, rel)
		require.Equal(t, "new_changed", rel.Name)
	})


	t.Run("delete_incorrect", func(t *testing.T) {
		respDataviewRelationAdd := mw.BlockDataviewRelationDelete(&pb.RpcBlockDataviewRelationDeleteRequest{
			ContextId:   mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: "not_existing_key",
		})
		require.Equal(t, pb.RpcBlockDataviewRelationDeleteResponseError_BAD_INPUT, respDataviewRelationAdd.Error.Code, respDataviewRelationAdd.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.GetAnytype().PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)
	})

	t.Run("delete_correct", func(t *testing.T) {
		relKey, _ := addRelation(t, mw.GetAnytype().PredefinedBlocks().SetPages, mw)

		respDataviewRelationDelete := mw.BlockDataviewRelationDelete(&pb.RpcBlockDataviewRelationDeleteRequest{
			ContextId:   mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: relKey,
		})
		//mw.blocksService.Close()
		respAccountCreate := mw.AccountSelect(&pb.RpcAccountSelectRequest{Id: mw.GetAnytype().Account(), RootPath: rootPath})
		require.Equal(t, 0, int(respAccountCreate.Error.Code))

		require.Equal(t, 0, int(respDataviewRelationDelete.Error.Code), respDataviewRelationDelete.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.GetAnytype().PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)

		require.Nil(t, getRelationByKey(block.GetDataview().Relations, relKey))
	})

	t.Run("relation_add_select_option", func(t *testing.T) {
		respRelCreate := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Relation: &model.Relation{
				Format: model.RelationFormat_status,
				SelectDict: []*model.RelationOption{{
					Text:  "opt1",
					Color: "red",
				}},
				Name:     "relation2",
				ReadOnly: false,
			},
		})
		require.Equal(t, 0, int(respRelCreate.Error.Code), respRelCreate.Error.Description)

		var foundRel *model.Relation
		for _, msg := range respRelCreate.Event.GetMessages() {
			if rel := msg.GetBlockDataviewRelationSet(); rel != nil && rel.Relation.Name == "relation2" {
				foundRel = rel.Relation
				break
			}
		}
		require.NotNil(t, foundRel)
		require.Equal(t, respRelCreate.RelationKey, foundRel.Key)
		// option is trimmed
		for _, opt := range foundRel.SelectDict {
			require.NotEqual(t, model.RelationOption_local, opt.Scope)
			require.NotEqual(t, "relation2", opt.Text)
		}

		respRecordCreate := mw.BlockDataviewRecordCreate(
			&pb.RpcBlockDataviewRecordCreateRequest{
				ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
				BlockId:   "dataview",
			})

		require.Equal(t, 0, int(respRecordCreate.Error.Code), respRecordCreate.Error.Description)
		newPageId := respRecordCreate.Record.Fields["id"].GetStringValue()

		respRelOptCreate := mw.BlockDataviewRecordRelationOptionAdd(&pb.RpcBlockDataviewRecordRelationOptionAddRequest{
			ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Option: &model.RelationOption{
				Text:  "opt1",
				Color: "red",
			},
			RecordId:    newPageId,
			RelationKey: respRelCreate.RelationKey,
		})
		require.Equal(t, 0, int(respRelOptCreate.Error.Code), respRelOptCreate.Error.Description)
		time.Sleep(time.Millisecond * 200)

		respRecordUpdate := mw.BlockDataviewRecordUpdate(
			&pb.RpcBlockDataviewRecordUpdateRequest{
				ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
				BlockId:   "dataview",
				RecordId:  newPageId,
				Record: &types2.Struct{
					Fields: map[string]*types2.Value{
						foundRel.Key: pbtypes.StringList([]string{respRelOptCreate.Option.Id}),
					},
				},
			})

		require.Equal(t, 0, int(respRecordUpdate.Error.Code), respRecordUpdate.Error.Description)

		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: newPageId})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 1)

		relOnPage := getRelationByKey(getEventObjectShow(respOpenNewPage.Event.Messages).Relations, foundRel.Key)
		require.Equal(t, foundRel.Key, relOnPage.Key)

		respOptAdd := mw.BlockDataviewRecordRelationOptionAdd(&pb.RpcBlockDataviewRecordRelationOptionAddRequest{
			ContextId:   mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: foundRel.Key,
			RecordId:    newPageId,
			Option: &model.RelationOption{
				Text:  "opt2",
				Color: "green",
			},
		})

		require.Equal(t, 0, int(respOptAdd.Error.Code), respOptAdd.Error.Description)
		time.Sleep(time.Millisecond * 200)

		respRecordUpdate2 := mw.BlockDataviewRecordUpdate(
			&pb.RpcBlockDataviewRecordUpdateRequest{
				ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
				BlockId:   "dataview",
				RecordId:  newPageId,
				Record: &types2.Struct{
					Fields: map[string]*types2.Value{
						foundRel.Key: pbtypes.StringList([]string{respOptAdd.Option.Id}),
					},
				},
			})
		require.Equal(t, 0, int(respRecordUpdate2.Error.Code), respRecordUpdate2.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: newPageId})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 1)

		relOnPage = getRelationByKey(getEventObjectShow(respOpenNewPage.Event.Messages).Relations, foundRel.Key)
		require.Equal(t, foundRel.Key, relOnPage.Key)
		require.Len(t, relOnPage.SelectDict, 3)
	})

	t.Run("relation_change_option_name", func(t *testing.T) {
		respRelCreate := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Relation: &model.Relation{
				Format: model.RelationFormat_status,
				SelectDict: []*model.RelationOption{{
					Text:  "opt1",
					Color: "red",
				}},
				Name:     "relation2",
				ReadOnly: false,
			},
		})
		require.Equal(t, 0, int(respRelCreate.Error.Code), respRelCreate.Error.Description)

		var foundRel *model.Relation
		for _, msg := range respRelCreate.Event.GetMessages() {
			if rel := msg.GetBlockDataviewRelationSet(); rel != nil && rel.Relation.Name == "relation2" {
				foundRel = rel.Relation
				break
			}
		}
		require.NotNil(t, foundRel)
		require.Equal(t, respRelCreate.RelationKey, foundRel.Key)
		// option is trimmed
		for _, opt := range foundRel.SelectDict {
			require.NotEqual(t, model.RelationOption_local, opt.Scope)
			require.NotEqual(t, "relation2", opt.Text)
		}

		respRecordCreate := mw.BlockDataviewRecordCreate(
			&pb.RpcBlockDataviewRecordCreateRequest{
				ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
				BlockId:   "dataview",
			})

		require.Equal(t, 0, int(respRecordCreate.Error.Code), respRecordCreate.Error.Description)
		newPageId := respRecordCreate.Record.Fields["id"].GetStringValue()

		respRelOptCreate := mw.BlockDataviewRecordRelationOptionAdd(&pb.RpcBlockDataviewRecordRelationOptionAddRequest{
			ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Option: &model.RelationOption{
				Text:  "opt3",
				Color: "red",
			},
			RecordId:    newPageId,
			RelationKey: respRelCreate.RelationKey,
		})
		require.Equal(t, 0, int(respRelOptCreate.Error.Code), respRelOptCreate.Error.Description)
		time.Sleep(time.Millisecond * 200)

		respRecordUpdate := mw.BlockDataviewRecordUpdate(
			&pb.RpcBlockDataviewRecordUpdateRequest{
				ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
				BlockId:   "dataview",
				RecordId:  newPageId,
				Record: &types2.Struct{
					Fields: map[string]*types2.Value{
						foundRel.Key: pbtypes.StringList([]string{respRelOptCreate.Option.Id}),
					},
				},
			})

		require.Equal(t, 0, int(respRecordUpdate.Error.Code), respRecordUpdate.Error.Description)

		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: newPageId})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 1)

		relOnPage := getRelationByKey(getEventObjectShow(respOpenNewPage.Event.Messages).Relations, foundRel.Key)
		require.Equal(t, foundRel.Key, relOnPage.Key)

		respOpt4Add := mw.BlockDataviewRecordRelationOptionAdd(&pb.RpcBlockDataviewRecordRelationOptionAddRequest{
			ContextId:   mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: foundRel.Key,
			RecordId:    newPageId,
			Option: &model.RelationOption{
				Text:  "opt4",
				Color: "green",
			},
		})

		require.Equal(t, 0, int(respOpt4Add.Error.Code), respOpt4Add.Error.Description)
		time.Sleep(time.Millisecond * 200)

		respRecordUpdate2 := mw.BlockDataviewRecordUpdate(
			&pb.RpcBlockDataviewRecordUpdateRequest{
				ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
				BlockId:   "dataview",
				RecordId:  newPageId,
				Record: &types2.Struct{
					Fields: map[string]*types2.Value{
						foundRel.Key: pbtypes.StringList([]string{respOpt4Add.Option.Id}),
					},
				},
			})
		require.Equal(t, 0, int(respRecordUpdate2.Error.Code), respRecordUpdate2.Error.Description)

		respOptUpdate := mw.BlockDataviewRecordRelationOptionUpdate(&pb.RpcBlockDataviewRecordRelationOptionUpdateRequest{
			ContextId:   mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: foundRel.Key,
			RecordId:    newPageId,
			Option: &model.RelationOption{
				Id:    respOpt4Add.Option.Id,
				Text:  "opt4_modified",
				Color: "green",
			},
		})

		require.Equal(t, 0, int(respOptUpdate.Error.Code), respOptUpdate.Error.Description)
		time.Sleep(time.Second * 1)

		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: newPageId})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 1)

		relOnPage = getRelationByKey(getEventObjectShow(respOpenNewPage.Event.Messages).Relations, foundRel.Key)
		require.Equal(t, foundRel.Key, relOnPage.Key)
		opt := pbtypes.GetOption(relOnPage.SelectDict, respOpt4Add.Option.Id)
		require.NotNil(t, opt)
		require.Equal(t, "opt4_modified", opt.Text)
	})

	t.Run("aggregated_options", func(t *testing.T) {
		type test struct {
			relKey             string
			mustHaveOptions    map[string]model.RelationOptionScope
			mustNotHaveOptions []string
		}

		rel1 := &model.Relation{
			Format:   model.RelationFormat_status,
			Name:     "ao_relation_status1",
			ReadOnly: false,
		}
		rel2 := &model.Relation{
			Format:   model.RelationFormat_status,
			Name:     "ao_relation_status2",
			ReadOnly: false,
		}
		rel3 := &model.Relation{
			Format:   model.RelationFormat_tag,
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
		time.Sleep(time.Millisecond * 200)

		respOptionAdd1 := mw.ObjectRelationOptionAdd(&pb.RpcObjectRelationOptionAddRequest{
			ContextId:   respPage1Create.PageId,
			RelationKey: rel1.Key,
			Option: &model.RelationOption{
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
			Option: &model.RelationOption{
				Id:    "ao_opt2_id",
				Text:  "ao_opt2",
				Color: "green",
			},
		})
		require.Equal(t, 0, int(respOptionAdd2.Error.Code), respOptionAdd2.Error.Description)

		respOptionAdd3 := mw.ObjectRelationOptionAdd(&pb.RpcObjectRelationOptionAddRequest{
			ContextId:   respPage2Create.PageId,
			RelationKey: rel1.Key,
			Option: &model.RelationOption{
				Id:    "ao_opt3_id",
				Text:  "ao_opt3",
				Color: "green",
			},
		})
		require.Equal(t, 0, int(respOptionAdd3.Error.Code), respOptionAdd3.Error.Description)

		respOptionAdd4 := mw.ObjectRelationOptionAdd(&pb.RpcObjectRelationOptionAddRequest{
			ContextId:   respPage2Create.PageId,
			RelationKey: rel2.Key,
			Option: &model.RelationOption{
				Id:    "ao_opt4_id",
				Text:  "ao_opt4",
				Color: "green",
			},
		})
		require.Equal(t, 0, int(respOptionAdd4.Error.Code), respOptionAdd4.Error.Description)

		respOptionAdd5 := mw.ObjectRelationOptionAdd(&pb.RpcObjectRelationOptionAddRequest{
			ContextId:   respPage2Create.PageId,
			RelationKey: rel3.Key,
			Option: &model.RelationOption{
				Id:    "ao_opt5_id",
				Text:  "ao_opt5",
				Color: "green",
			},
		})
		require.Equal(t, 0, int(respOptionAdd5.Error.Code), respOptionAdd5.Error.Description)
		time.Sleep(time.Millisecond * 200)
		tests := []test{
			{
				rel1.Key,
				map[string]model.RelationOptionScope{
					"ao_opt1_id": model.RelationOption_local,
					"ao_opt3_id": model.RelationOption_local,
					"ao_opt4_id": model.RelationOption_format,
					"ao_opt2_id": model.RelationOption_format,
				},
				[]string{"ao_opt5_id"},
			},
			{
				rel2.Key,
				map[string]model.RelationOptionScope{
					"ao_opt4_id": model.RelationOption_local,
					"ao_opt1_id": model.RelationOption_format,
					"ao_opt2_id": model.RelationOption_relation,
					"ao_opt3_id": model.RelationOption_format,
				},
				[]string{"ao_opt5_id"},
			},
			{
				rel3.Key,
				map[string]model.RelationOptionScope{
					"ao_opt5_id": model.RelationOption_local,
				},
				[]string{"ao_opt1_id", "ao_opt2_id", "ao_opt3_id", "ao_opt4_id"},
			},
		}
		respDvRelAdd := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Relation:  rel1,
		})
		require.Equal(t, 0, int(respDvRelAdd.Error.Code), respDvRelAdd.Error.Description)

		respDvRelAdd = mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Relation:  rel2,
		})
		require.Equal(t, 0, int(respDvRelAdd.Error.Code), respDvRelAdd.Error.Description)

		respDvRelAdd = mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Relation:  rel3,
		})
		require.Equal(t, 0, int(respDvRelAdd.Error.Code), respDvRelAdd.Error.Description)

		respOpenNewPage := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.GetAnytype().PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		block := getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)
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

	t.Run("aggregated_options_set_details", func(t *testing.T) {
		rel1 := &model.Relation{
			Format:   model.RelationFormat_status,
			Name:     "2ao_relation_status1",
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

		respOptionAdd1 := mw.ObjectRelationOptionAdd(&pb.RpcObjectRelationOptionAddRequest{
			ContextId:   respPage1Create.PageId,
			RelationKey: rel1.Key,
			Option: &model.RelationOption{
				Id:    "ao_opt1_id",
				Text:  "ao_opt1",
				Color: "red",
			},
		})
		require.Equal(t, 0, int(respOptionAdd1.Error.Code), respOptionAdd1.Error.Description)
		time.Sleep(time.Second)

		respSetDetails := mw.BlockSetDetails(&pb.RpcBlockSetDetailsRequest{
			ContextId: respPage2Create.PageId,
			Details:   []*pb.RpcBlockSetDetailsDetail{{Key: rel1.Key, Value: pbtypes.StringList([]string{respOptionAdd1.Option.Id})}},
		})
		require.Equal(t, 0, int(respSetDetails.Error.Code), respSetDetails.Error.Description)

		var found bool
		for _, msg := range respSetDetails.Event.Messages {
			for _, rel := range msg.GetObjectRelationsAmend().GetRelations() {
				for _, opt := range rel.SelectDict {
					if opt.Id == respOptionAdd1.Option.Id {
						if opt.Scope == model.RelationOption_local {
							found = true
						} else {
							t.Fatalf("wrong scope: %s", opt.Scope.String())
						}
					}
				}
			}
		}
		require.True(t, found, "event not found")

	})

	t.Run("new_record_creator", func(t *testing.T) {
		f := func(event *pb.Event) {
			return
		}

		mw.EventSender = event.NewCallbackSender(func(event *pb.Event) {
			f(event)
		})

		respRelCreate := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Relation: &model.Relation{
				Format: model.RelationFormat_status,
				SelectDict: []*model.RelationOption{{
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
		var waitCreator = make(chan struct{})
		f = func(event *pb.Event) {
			for _, msg := range event.Messages {
				for _, rec := range msg.GetBlockDataviewRecordsUpdate().Records {
					if pbtypes.GetString(rec, bundle.RelationKeyCreator.String()) != "" {
						close(waitCreator)
					}
				}
			}
		}

		respRecordCreate := mw.BlockDataviewRecordCreate(
			&pb.RpcBlockDataviewRecordCreateRequest{
				ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
				BlockId:   "dataview",
			})

		require.Equal(t, 0, int(respRecordCreate.Error.Code), respRecordCreate.Error.Description)
		if pbtypes.GetString(respRecordCreate.Record, bundle.RelationKeyCreator.String()) == "" {
			select {
			case <-waitCreator:
				break
			case <-time.After(time.Second * 10):
				require.Fail(t, "creator not found")
			}
		}

	})

}

func TestCustomType(t *testing.T) {
	_, mw := start(t, nil)

	respObjectTypeList := mw.ObjectTypeList(nil)
	require.Equal(t, 0, int(respObjectTypeList.Error.Code), respObjectTypeList.Error.Description)

	respObjectTypeCreate := mw.ObjectTypeCreate(&pb.RpcObjectTypeCreateRequest{
		ObjectType: &model.ObjectType{
			Name:   "1",
			Layout: model.ObjectType_todo,
			Relations: []*model.Relation{
				{Format: model.RelationFormat_date, Name: "date of birth", MaxCount: 1},
				{Format: model.RelationFormat_object, Name: "assignee", ObjectTypes: []string{"_otpage"}},
				{Format: model.RelationFormat_longtext, Name: "bio", MaxCount: 1},
			},
		},
	})

	require.Equal(t, 0, int(respObjectTypeCreate.Error.Code), respObjectTypeCreate.Error.Description)
	require.Len(t, respObjectTypeCreate.ObjectType.Relations, len(bundle.RequiredInternalRelations)+3+1) // including relation.RequiredInternalRelations and done from the to-do layout
	require.True(t, strings.HasPrefix(respObjectTypeCreate.ObjectType.Url, "b"))
	var newRelation *model.Relation
	for _, rel := range respObjectTypeCreate.ObjectType.Relations {
		if rel.Name == "bio" {
			newRelation = rel
			break
		}
	}

	time.Sleep(time.Millisecond * 200)
	respObjectTypeList = mw.ObjectTypeList(nil)
	require.Equal(t, 0, int(respObjectTypeList.Error.Code), respObjectTypeList.Error.Description)
	ot := pbtypes.GetObjectType(respObjectTypeList.ObjectTypes, respObjectTypeCreate.ObjectType.Url)
	require.NotNil(t, ot)
	require.Len(t, ot.Relations, len(bundle.RequiredInternalRelations)+3)

	respSearch := mw.ObjectSearch(&pb.RpcObjectSearchRequest{Filters: []*model.BlockContentDataviewFilter{{
		RelationKey: "type",
		Condition:   model.BlockContentDataviewFilter_Equal,
		Value:       pbtypes.String(bundle.TypeKeyObjectType.URL()),
	}}})

	var found2 bool
	for _, rec := range respSearch.Records {
		if pbtypes.GetString(rec, "id") == respObjectTypeCreate.ObjectType.Url {
			found2 = true
		}
	}
	require.True(t, found2, "new object type not found in search")

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
	show := getEventObjectShow(respOpenCustomTypeObject.Event.Messages)
	require.NotNil(t, show)

	profile := getDetailsForContext(show.Details, mw.GetAnytype().PredefinedBlocks().Profile)
	require.NotNil(t, profile)
	// should have custom obj type + profile, because it has the relation `creator`
	require.Len(t, show.ObjectTypes, 3)
	//require.Len(t, show.ObjectTypePerObject, 2)
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
	require.Equal(t, mw.GetAnytype().PredefinedBlocks().Profile, pbtypes.GetString(customObjectDetails, bundle.RelationKeyCreator.String()))
	rel := getRelationByKey(show.Relations, newRelation.Key)
	require.NotNil(t, rel)
	newRelation.Creator = mw.GetAnytype().ProfileID()
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
		break
	}
	show = getEventObjectShow(respOpenCustomTypeSet.Event.Messages)
	require.NotNil(t, show)

	respSearch2 := mw.ObjectSearch(&pb.RpcObjectSearchRequest{Filters: []*model.BlockContentDataviewFilter{{
		RelationKey: "type",
		Condition:   model.BlockContentDataviewFilter_Equal,
		Value:       pbtypes.String(respObjectTypeCreate.ObjectType.Url),
	}}})
	require.Equal(t, 0, int(respSearch2.Error.Code), respSearch2.Error.Description)
	require.Len(t, respSearch2.Records, 1)
}

func TestBundledType(t *testing.T) {
	_, mw := start(t, nil)

	respCreatePage := mw.PageCreate(&pb.RpcPageCreateRequest{Details: &types2.Struct{Fields: map[string]*types2.Value{"name": pbtypes.String("test1")}}})
	require.Equal(t, 0, int(respCreatePage.Error.Code), respCreatePage.Error.Description)
	log.Errorf("page %s created", respCreatePage.PageId)
	respOpenPage := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: respCreatePage.PageId})
	require.Equal(t, 0, int(respOpenPage.Error.Code), respOpenPage.Error.Description)

	show := getEventObjectShow(respOpenPage.Event.Messages)
	require.NotNil(t, show)

	pageDetails := getDetailsForContext(show.Details, respCreatePage.PageId)
	require.NotNil(t, pageDetails)
	require.Equal(t, mw.GetAnytype().PredefinedBlocks().Profile, pbtypes.GetString(pageDetails, bundle.RelationKeyCreator.String()))
	profile := getDetailsForContext(show.Details, mw.GetAnytype().PredefinedBlocks().Profile)
	require.NotNil(t, profile, fmt.Sprintf("%s got no details for profile", show.RootId))

	time.Sleep(time.Millisecond * 200)

	respOpenPagesSet := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.GetAnytype().PredefinedBlocks().SetPages})
	require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)

	show = getEventObjectShow(respOpenPagesSet.Event.Messages)
	require.NotNil(t, show)

	recordsSet := getEventRecordsSet(respOpenPagesSet.Event.Messages)
	require.NotNil(t, recordsSet)

	require.Len(t, recordsSet.Records, 1)
	require.Equal(t, respCreatePage.PageId, getEventRecordsSet(respOpenPagesSet.Event.Messages).Records[0].Fields["id"].GetStringValue())

	respCreatePage = mw.PageCreate(&pb.RpcPageCreateRequest{Details: &types2.Struct{Fields: map[string]*types2.Value{"name": pbtypes.String("test2")}}})
	require.Equal(t, 0, int(respCreatePage.Error.Code), respCreatePage.Error.Description)

	time.Sleep(time.Millisecond * 200)
	respOpenPagesSet = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.GetAnytype().PredefinedBlocks().SetPages})
	require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)

	show = getEventObjectShow(respOpenPagesSet.Event.Messages)
	require.NotNil(t, show)
	require.Len(t, getEventRecordsSet(respOpenPagesSet.Event.Messages).Records, 2)

	require.True(t, hasRecordWithKeyAndVal(getEventRecordsSet(respOpenPagesSet.Event.Messages).Records, "id", respCreatePage.PageId))
}

func TestArchiveIndex(t *testing.T) {
	_, mw := start(t, nil)

	resp := mw.BlockCreatePage(&pb.RpcBlockCreatePageRequest{})
	require.Equal(t, int(resp.Error.Code), 0, resp.Error.Description)

	respArchive := mw.BlockListSetPageIsArchived(&pb.RpcBlockListSetPageIsArchivedRequest{
		ContextId:  "",
		BlockIds:   []string{resp.TargetId},
		IsArchived: true,
	})
	require.Equal(t, int(respArchive.Error.Code), 0, respArchive.Error.Description)

	d, err := mw.GetAnytype().ObjectStore().GetDetails(resp.TargetId)
	require.NoError(t, err)
	pbtypes.Get(d.GetDetails(), bundle.RelationKeyIsArchived.String()).Equal(pbtypes.Bool(true))

	respArchive = mw.BlockListSetPageIsArchived(&pb.RpcBlockListSetPageIsArchivedRequest{
		ContextId:  "",
		BlockIds:   []string{resp.TargetId},
		IsArchived: false,
	})
	require.Equal(t, int(respArchive.Error.Code), 0, respArchive.Error.Description)

	d, err = mw.GetAnytype().ObjectStore().GetDetails(resp.TargetId)
	require.NoError(t, err)
	pbtypes.Get(d.GetDetails(), bundle.RelationKeyIsArchived.String()).Equal(pbtypes.Bool(false))
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

func getEventObjectShow(msgs []*pb.EventMessage) *pb.EventObjectShow {
	for _, msg := range msgs {
		if v, ok := msg.Value.(*pb.EventMessageValueOfObjectShow); ok {
			return v.ObjectShow
		}
	}
	return nil
}

func getDetailsForContext(msgs []*pb.EventObjectDetailsSet, contextId string) *types2.Struct {
	for _, msg := range msgs {
		if msg.Id == contextId {
			return msg.Details
		}
	}
	return nil
}

