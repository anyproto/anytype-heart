package core

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"

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

func start(t *testing.T, eventSender event.Sender) (rootPath string, mw *Middleware, close func() error) {
	if debug, ok := os.LookupEnv("ANYPROF"); ok && debug != "" {
		go func() {
			http.ListenAndServe(debug, nil)
		}()
	}
	mw = New()
	rootPath, err := ioutil.TempDir(os.TempDir(), "anytype_*")
	require.NoError(t, err)
	close = func() error { return os.RemoveAll(rootPath) }

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

	return rootPath, mw, close
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

	rootPath, mw, appClose := start(t, event.NewCallbackSender(func(event *pb.Event) {
		eventHandler(event)
	}))
	defer appClose()

	respOpenNewPage := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.GetAnytype().PredefinedBlocks().SetPages})
	require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
	block := getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)

	respSetActiveView := mw.BlockDataviewViewSetActive(&pb.RpcBlockDataviewViewSetActiveRequest{
		ContextId: mw.GetAnytype().PredefinedBlocks().SetPages,
		BlockId:   block.Id,
		ViewId:    block.GetDataview().Views[0].Id,
	})
	require.Equal(t, 0, int(respSetActiveView.Error.Code), respSetActiveView.Error.Description)
	require.Len(t, block.GetDataview().Relations, len(bundle.MergeRelationsKeys(bundle.GetRelationsKeys(bundle.MustGetType(bundle.TypeKeyNote).Relations), dataview.DefaultDataviewRelations)))

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
		block = getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)

		require.Len(t, block.GetDataview().Relations, len(bundle.MergeRelationsKeys(bundle.GetRelationsKeys(bundle.MustGetType(bundle.TypeKeyNote).Relations), dataview.DefaultDataviewRelations)))
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
		block = getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)
		require.Len(t, block.GetDataview().Relations, len(bundle.MergeRelationsKeys(bundle.GetRelationsKeys(bundle.MustGetType(bundle.TypeKeyPage).Relations), dataview.DefaultDataviewRelations))+1)

		respAccountCreate := mw.AccountSelect(&pb.RpcAccountSelectRequest{Id: mw.GetAnytype().Account(), RootPath: rootPath})
		require.Equal(t, 0, int(respAccountCreate.Error.Code))
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.GetAnytype().PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		block = getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)
		require.Len(t, block.GetDataview().Relations, len(bundle.MergeRelationsKeys(bundle.GetRelationsKeys(bundle.MustGetType(bundle.TypeKeyPage).Relations), dataview.DefaultDataviewRelations))+1)
	})

	t.Run("relation_aggregate_scope", func(t *testing.T) {
		respSet1 := mw.SetCreate(&pb.RpcSetCreateRequest{
			Source:  []string{bundle.TypeKeyIdea.URL()},
			Details: nil,
		})

		require.Equal(t, 0, int(respSet1.Error.Code), respSet1.Error.Description)

		respSet2 := mw.SetCreate(&pb.RpcSetCreateRequest{
			Source:  []string{bundle.TypeKeyIdea.URL()},
			Details: nil,
		})
		require.Equal(t, 0, int(respSet2.Error.Code), respSet2.Error.Description)

		respSetRelCreate1 := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: respSet1.Id,
			BlockId:   "dataview",
			Relation:  &model.Relation{Format: model.RelationFormat_shorttext, Name: "from set1"},
		})
		if respSetRelCreate1.Error.Code != 0 {
			t.Fatalf(pbtypes.Sprint(respSet1))
		}
		require.Equal(t, 0, int(respSetRelCreate1.Error.Code), respSetRelCreate1.Error.Description)

		respSetRelCreate2 := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: respSet2.Id,
			BlockId:   "dataview",
			Relation:  &model.Relation{Format: model.RelationFormat_shorttext, Name: "from set2"},
		})
		if respSetRelCreate2.Error.Code != 0 {
			t.Fatalf(pbtypes.Sprint(respSet2))
		}
		require.Equal(t, 0, int(respSetRelCreate2.Error.Code), respSetRelCreate2.Error.Description)

		respPageCreate := mw.PageCreate(&pb.RpcPageCreateRequest{Details: &types2.Struct{Fields: map[string]*types2.Value{
			bundle.RelationKeyType.String(): pbtypes.String(bundle.TypeKeyIdea.URL()),
		}}})
		require.Equal(t, 0, int(respPageCreate.Error.Code), respPageCreate.Error.Description)

		//time.sleep(time.Millisecond * 200)
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
		block = getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)

		require.Equal(t, "new_changed", block.GetDataview().Relations[len(block.GetDataview().Relations)-1].Name)
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
		block = getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)

		require.Nil(t, getRelationByKey(block.GetDataview().Relations, relKey))
	})

	t.Run("relation_object_change_option_name", func(t *testing.T) {
		pageCreateResp := mw.PageCreate(&pb.RpcPageCreateRequest{})
		require.Equal(t, 0, int(pageCreateResp.Error.Code), pageCreateResp.Error.Description)

		pageOpenResp := mw.BlockOpen(&pb.RpcBlockOpenRequest{
			BlockId: pageCreateResp.PageId,
		})

		optCreateResp := mw.ObjectRelationOptionAdd(&pb.RpcObjectRelationOptionAddRequest{
			ContextId:   pageCreateResp.PageId,
			RelationKey: bundle.RelationKeyTag.String(),
			Option: &model.RelationOption{
				Id:    "",
				Text:  "opt7",
				Scope: 0,
			},
		})

		setDetailsResp := mw.BlockSetDetails(&pb.RpcBlockSetDetailsRequest{
			ContextId: pageCreateResp.PageId,
			Details: []*pb.RpcBlockSetDetailsDetail{{
				Key:   bundle.RelationKeyTag.String(),
				Value: pbtypes.StringList([]string{optCreateResp.Option.Id}),
			}},
		})
		require.Equal(t, 0, int(setDetailsResp.Error.Code), setDetailsResp.Error.Description)

		relOnPage := getRelationByKey(getEventObjectShow(pageOpenResp.Event.Messages).Relations, bundle.RelationKeyTag.String())
		require.NotNil(t, relOnPage)
		/*var found bool
		// option is trimmed
		for _, opt := range relOnPage.SelectDict {
			if opt.Id != optCreateResp.Option.Id {
				continue
			}

			require.Equal(t, model.RelationOption_local, opt.Scope)
			require.Equal(t, "rel7", opt.Text)
			found = true
		}
		require.True(t, found, "option not found")*/

		option := optCreateResp.Option
		option.Text = "opt7_modified"

		respOptUpdate := mw.ObjectRelationOptionUpdate(&pb.RpcObjectRelationOptionUpdateRequest{
			ContextId:   pageCreateResp.PageId,
			RelationKey: bundle.RelationKeyTag.String(),
			Option:      option,
		})
		require.Equal(t, 0, int(respOptUpdate.Error.Code), respOptUpdate.Error.Description)

		relAmend := getEventObjectRelationAmend(respOptUpdate.Event.GetMessages())
		require.Equal(t, relAmend.Id, pageCreateResp.PageId)

		rel := pbtypes.GetRelation(relAmend.Relations, bundle.RelationKeyTag.String())
		require.NotNil(t, rel)
		newOpt := pbtypes.GetOption(rel.SelectDict, option.Id)
		require.NotNil(t, newOpt)
		require.Equal(t, "opt7_modified", newOpt.Text)
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
				bundle.RelationKeyType.String(): pbtypes.StringList([]string{bundle.TypeKeyNote.URL()}),
			}},
		})
		require.Equal(t, 0, int(respPage1Create.Error.Code), respPage1Create.Error.Description)
		respPage2Create := mw.PageCreate(&pb.RpcPageCreateRequest{
			Details: &types2.Struct{Fields: map[string]*types2.Value{
				bundle.RelationKeyType.String(): pbtypes.StringList([]string{bundle.TypeKeyNote.URL()}),
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
		//time.sleep(time.Millisecond * 200)

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
		//time.sleep(time.Millisecond * 200)
		tests := []test{
			{
				rel1.Key,
				map[string]model.RelationOptionScope{
					"ao_opt1_id": model.RelationOption_local,
					"ao_opt3_id": model.RelationOption_local,
				},
				[]string{"ao_opt5_id"},
			},
			{
				rel2.Key,
				map[string]model.RelationOptionScope{
					"ao_opt4_id": model.RelationOption_local,
					"ao_opt2_id": model.RelationOption_relation,
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
				Id:    "ao_opt8_id",
				Text:  "ao_opt8",
				Color: "red",
			},
		})
		require.Equal(t, 0, int(respOptionAdd1.Error.Code), respOptionAdd1.Error.Description)

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
		require.True(t, found, "event not found", pbtypes.Sprint(respSetDetails.Event))

	})

	t.Run("update_relation_name_in_set_expect_change_in_object", func(t *testing.T) {
		relKey, _ := addRelation(t, mw.GetAnytype().PredefinedBlocks().SetPages, mw)

		recCreate := mw.BlockDataviewRecordCreate(&pb.RpcBlockDataviewRecordCreateRequest{
			ContextId:  mw.GetAnytype().PredefinedBlocks().SetPages,
			BlockId:    "dataview",
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
		block = getBlockById("dataview", getEventObjectShow(respOpenNewPage.Event.Messages).Blocks)

		require.Equal(t, "new_changed", block.GetDataview().Relations[len(block.GetDataview().Relations)-1].Name)
		//time.sleep(time.Millisecond*200)
		respOpenNewRecord := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: pbtypes.GetString(recCreate.Record, "id")})
		rel := pbtypes.GetRelation(getEventObjectShow(respOpenNewRecord.Event.Messages).Relations, relKey)
		require.NotNil(t, rel)
		require.Equal(t, "new_changed", rel.Name)
	})
}

func TestArchiveIndex(t *testing.T) {
	_, mw, close := start(t, nil)
	defer close()

	resp := mw.BlockCreatePage(&pb.RpcBlockCreatePageRequest{})
	require.Equal(t, int(resp.Error.Code), 0, resp.Error.Description)

	respArchive := mw.ObjectListSetIsArchived(&pb.RpcObjectListSetIsArchivedRequest{
		ObjectIds:  []string{resp.TargetId},
		IsArchived: true,
	})
	require.Equal(t, int(respArchive.Error.Code), 0, respArchive.Error.Description)
	time.Sleep(time.Millisecond * 500) // todo: remove after we have moved to the callbacks

	d, err := mw.GetAnytype().ObjectStore().GetDetails(resp.TargetId)
	require.NoError(t, err)
	require.True(t, pbtypes.Get(d.GetDetails(), bundle.RelationKeyIsArchived.String()).Equal(pbtypes.Bool(true)))

	respArchive = mw.ObjectListSetIsArchived(&pb.RpcObjectListSetIsArchivedRequest{
		ObjectIds:  []string{resp.TargetId},
		IsArchived: false,
	})
	require.Equal(t, int(respArchive.Error.Code), 0, respArchive.Error.Description)
	time.Sleep(time.Millisecond * 500) // todo: remove after we have moved to the callbacks
	d, err = mw.GetAnytype().ObjectStore().GetDetails(resp.TargetId)
	require.NoError(t, err)
	require.True(t, pbtypes.Get(d.GetDetails(), bundle.RelationKeyIsArchived.String()).Equal(pbtypes.Bool(false)))
}

func getEventObjectRelationAmend(msgs []*pb.EventMessage) *pb.EventObjectRelationsAmend {
	for _, msg := range msgs {
		if v, ok := msg.Value.(*pb.EventMessageValueOfObjectRelationsAmend); ok {
			return v.ObjectRelationsAmend
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
