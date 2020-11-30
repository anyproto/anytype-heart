package core

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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
	os.Setenv("cafe_p2p_addr", "-")
	os.Setenv("cafe_grpc_addr", "-")

	mw.EventSender = event.NewCallbackSender(func(event *pb.Event) {
		// nothing to do
	})

	respWalletCreate := mw.WalletCreate(&pb.RpcWalletCreateRequest{RootPath: rootPath})
	require.Equal(t, 0, int(respWalletCreate.Error.Code))

	respAccountCreate := mw.AccountCreate(&pb.RpcAccountCreateRequest{Name: "profile", AlphaInviteCode: "elbrus"})
	require.Equal(t, 0, int(respAccountCreate.Error.Code))

	return rootPath, mw
}

func TestRelationAdd(t *testing.T) {
	rootPath, mw := start(t)

	respOpenNewPage := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
	require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
	require.Len(t, respOpenNewPage.Event.Messages, 2)
	block := getBlockById("dataview", respOpenNewPage.Event.Messages[0].GetBlockShow().Blocks)

	require.Len(t, block.GetDataview().Relations, len(relation.BundledObjectTypes["page"].Relations))

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
		block = getBlockById("dataview", respOpenNewPage.Event.Messages[0].GetBlockShow().Blocks)

		require.Len(t, block.GetDataview().Relations, len(relation.BundledObjectTypes["page"].Relations))

	})

	var relKey string
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
		block = getBlockById("dataview", respOpenNewPage.Event.Messages[0].GetBlockShow().Blocks)
		relKey = respDataviewRelationAdd.RelationKey
		require.Len(t, block.GetDataview().Relations, len(relation.BundledObjectTypes["page"].Relations)+1)

		respAccountCreate := mw.AccountSelect(&pb.RpcAccountSelectRequest{Id: mw.Anytype.Account(), RootPath: rootPath})
		require.Equal(t, 0, int(respAccountCreate.Error.Code))
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", respOpenNewPage.Event.Messages[0].GetBlockShow().Blocks)
		relKey = respDataviewRelationAdd.RelationKey
		require.Len(t, block.GetDataview().Relations, len(relation.BundledObjectTypes["page"].Relations)+1)
	})

	t.Run("update_not_existing", func(t *testing.T) {
		respUpdate := mw.BlockDataviewRelationUpdate(&pb.RpcBlockDataviewRelationUpdateRequest{
			ContextId:   mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: "ffff",
			Relation:    &pbrelation.Relation{Key: "ffff"},
		})
		require.Equal(t, pb.RpcBlockDataviewRelationUpdateResponseError_BAD_INPUT, respUpdate.Error.Code, respUpdate.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", respOpenNewPage.Event.Messages[0].GetBlockShow().Blocks)

		require.Len(t, block.GetDataview().Relations, len(relation.BundledObjectTypes["page"].Relations)+1)
		require.Equal(t, "relation1", block.GetDataview().Relations[len(block.GetDataview().Relations)-1].Name)

	})

	t.Run("update_cant_change_format", func(t *testing.T) {
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
		block = getBlockById("dataview", respOpenNewPage.Event.Messages[0].GetBlockShow().Blocks)

		require.Len(t, block.GetDataview().Relations, len(relation.BundledObjectTypes["page"].Relations)+1)
		require.Equal(t, "relation1", block.GetDataview().Relations[len(block.GetDataview().Relations)-1].Name)
	})

	t.Run("update_correct", func(t *testing.T) {
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
		block = getBlockById("dataview", respOpenNewPage.Event.Messages[0].GetBlockShow().Blocks)

		require.Len(t, block.GetDataview().Relations, len(relation.BundledObjectTypes["page"].Relations)+1)
		require.Equal(t, "new_changed", block.GetDataview().Relations[len(block.GetDataview().Relations)-1].Name)
	})

	t.Run("delete_incorrect", func(t *testing.T) {
		respDataviewRelationAdd := mw.BlockDataviewRelationDelete(&pb.RpcBlockDataviewRelationDeleteRequest{
			ContextId:   mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: "ffff",
		})
		require.Equal(t, pb.RpcBlockDataviewRelationDeleteResponseError_BAD_INPUT, respDataviewRelationAdd.Error.Code, respDataviewRelationAdd.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", respOpenNewPage.Event.Messages[0].GetBlockShow().Blocks)

		require.Len(t, block.GetDataview().Relations, len(relation.BundledObjectTypes["page"].Relations)+1)

	})

	t.Run("delete_correct", func(t *testing.T) {
		respDataviewRelationDelete := mw.BlockDataviewRelationDelete(&pb.RpcBlockDataviewRelationDeleteRequest{
			ContextId:   mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: relKey,
		})
		mw.blocksService.Close()
		respAccountCreate := mw.AccountSelect(&pb.RpcAccountSelectRequest{Id: mw.Anytype.Account(), RootPath: rootPath})
		require.Equal(t, 0, int(respAccountCreate.Error.Code))

		require.Equal(t, 0, int(respDataviewRelationDelete.Error.Code), respDataviewRelationDelete.Error.Description)
		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 2)
		block = getBlockById("dataview", respOpenNewPage.Event.Messages[0].GetBlockShow().Blocks)

		require.Len(t, block.GetDataview().Relations, len(relation.BundledObjectTypes["page"].Relations))
	})

	t.Run("relation_add_select_option", func(t *testing.T) {
		respRelCreate := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
			ContextId: mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:   "dataview",
			Relation: &pbrelation.Relation{
				Format: pbrelation.RelationFormat_select,
				SelectDict: []*pbrelation.RelationSelectOption{{
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
		require.Len(t, rel.Relation.SelectDict, 1)
		require.True(t, len(rel.Relation.SelectDict[0].Id) > 0)

		respRecordCreate := mw.BlockDataviewRecordCreate(
			&pb.RpcBlockDataviewRecordCreateRequest{
				ContextId: mw.Anytype.PredefinedBlocks().SetPages,
				BlockId:   "dataview",
				Record: &types2.Struct{
					Fields: map[string]*types2.Value{
						rel.Relation.Key: pbtypes.StringList([]string{rel.Relation.SelectDict[0].Id}),
					},
				},
			})

		require.Equal(t, 0, int(respRecordCreate.Error.Code), respRecordCreate.Error.Description)
		newPageId := respRecordCreate.Record.Fields["id"].GetStringValue()

		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: newPageId})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 1)

		require.Len(t, respOpenNewPage.Event.Messages[0].GetBlockShow().Relations, len(relation.BundledObjectTypes["page"].Relations)+1)
		relOnPage := getRelationByKey(respOpenNewPage.Event.Messages[0].GetBlockShow().Relations, rel.RelationKey)
		require.Equal(t, rel.Relation, relOnPage)

		respOptAdd := mw.BlockDataviewRelationSelectOptionAdd(&pb.RpcBlockDataviewRelationSelectOptionAddRequest{
			ContextId:   mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: rel.RelationKey,
			Option: &pbrelation.RelationSelectOption{
				Text:  "opt2",
				Color: "green",
			},
		})

		require.Equal(t, 0, int(respOptAdd.Error.Code), respOptAdd.Error.Description)

		respRecordUpdate := mw.BlockDataviewRecordUpdate(
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
		require.Equal(t, 0, int(respRecordUpdate.Error.Code), respRecordUpdate.Error.Description)

		respOpenNewPage = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: newPageId})
		require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)
		require.Len(t, respOpenNewPage.Event.Messages, 1)

		rel.Relation.SelectDict = append(rel.Relation.SelectDict, respOptAdd.Option)
		relOnPage = getRelationByKey(respOpenNewPage.Event.Messages[0].GetBlockShow().Relations, rel.RelationKey)
		require.Len(t, relOnPage.SelectDict, 2)
		require.Equal(t, rel.Relation, relOnPage)
	})

	t.Run("search_object", func(t *testing.T) {
		respSearch := mw.ObjectSearch(&pb.RpcObjectSearchRequest{Filters: []*model.BlockContentDataviewFilter{{
			RelationKey: "type",
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(bundledObjectTypeURLPrefix + "page"),
		}}})
		require.Equal(t, 0, int(respSearch.Error.Code), respSearch.Error.Description)
		require.Len(t, respSearch.Records, 1)
	})
}

func TestCustomType(t *testing.T) {
	_, mw := start(t)

	respObjectTypeList := mw.ObjectTypeList(nil)
	require.Equal(t, 0, int(respObjectTypeList.Error.Code), respObjectTypeList.Error.Description)
	require.Len(t, respObjectTypeList.ObjectTypes, len(relation.BundledObjectTypes))

	respObjectTypeCreate := mw.ObjectTypeCreate(&pb.RpcObjectTypeCreateRequest{
		ObjectType: &pbrelation.ObjectType{
			Name: "1",
			Relations: []*pbrelation.Relation{
				{Format: pbrelation.RelationFormat_date, Name: "date of birth"},
				{Format: pbrelation.RelationFormat_object, Name: "assignee", ObjectTypes: []string{"https://anytype.io/schemas/object/bundled/pages"}},
				{Format: pbrelation.RelationFormat_description, Name: "bio"},
			},
		},
	})

	require.Equal(t, 0, int(respObjectTypeCreate.Error.Code), respObjectTypeCreate.Error.Description)
	require.Len(t, respObjectTypeCreate.ObjectType.Relations, 9) // including relation.RequiredInternalRelations
	require.True(t, strings.HasPrefix(respObjectTypeCreate.ObjectType.Url, "https://anytype.io/schemas/object/custom/"))
	var newRelation *pbrelation.Relation
	for _, rel := range respObjectTypeCreate.ObjectType.Relations {
		if rel.Name == "bio" {
			newRelation = rel
			break
		}
	}

	fmt.Printf("newRelation: %+v\n", newRelation)
	respObjectTypeList = mw.ObjectTypeList(nil)
	require.Equal(t, 0, int(respObjectTypeList.Error.Code), respObjectTypeList.Error.Description)
	require.Len(t, respObjectTypeList.ObjectTypes, len(relation.BundledObjectTypes)+1)
	lastObjType := respObjectTypeList.ObjectTypes[len(respObjectTypeList.ObjectTypes)-1]
	require.Equal(t, respObjectTypeCreate.ObjectType.Url, lastObjType.Url)
	require.Len(t, lastObjType.Relations, 9)

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
	show := respOpenCustomTypeObject.Event.Messages[0].GetBlockShow()
	require.NotNil(t, show)
	require.Len(t, show.ObjectTypes, 1)
	require.Len(t, show.ObjectTypesPerObject, 1)
	// omit relations
	respObjectTypeCreate.ObjectType.Relations = nil
	require.Equal(t, respObjectTypeCreate.ObjectType, show.ObjectTypes[0])
	var details *types2.Struct
	for _, detail := range show.Details {
		if detail.Id == customObjectId {
			details = detail.Details
			break
		}
	}

	var found bool
	for _, rel := range show.Relations {
		if rel.Key == newRelation.Key {
			require.Equal(t, newRelation, rel)
			found = true
			break
		}
	}
	require.True(t, found)

	require.NotNil(t, details.Fields[newRelation.Key])
	require.Equal(t, "newRelationVal", details.Fields[newRelation.Key].GetStringValue())

	respOpenCustomTypeSet = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: respCreateCustomTypeSet.Id})
	require.Equal(t, 0, int(respOpenCustomTypeSet.Error.Code), respOpenCustomTypeSet.Error.Description)

	require.Len(t, respOpenCustomTypeSet.Event.Messages, 2)
	require.Len(t, respOpenCustomTypeSet.Event.Messages[1].GetBlockDataviewRecordsSet().Records, 1)
	require.Equal(t, respOpenCustomTypeSet.Event.Messages[1].GetBlockDataviewRecordsSet().Records[0].Fields["id"].GetStringValue(), respCreateRecordInCustomTypeSet.Record.Fields["id"].GetStringValue())

	show = respOpenCustomTypeSet.Event.Messages[0].GetBlockShow()
	require.NotNil(t, show)

	t.Run("search_object", func(t *testing.T) {
		respSearch := mw.ObjectSearch(&pb.RpcObjectSearchRequest{Filters: []*model.BlockContentDataviewFilter{{
			RelationKey: "type",
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(respObjectTypeCreate.ObjectType.Url),
		}}})
		require.Equal(t, 0, int(respSearch.Error.Code), respSearch.Error.Description)
		require.Len(t, respSearch.Records, 1)
	})
}

func TestBundledType(t *testing.T) {
	_, mw := start(t)

	respCreatePage := mw.PageCreate(&pb.RpcPageCreateRequest{Details: &types2.Struct{Fields: map[string]*types2.Value{"name": pbtypes.String("test1")}}})
	require.Equal(t, 0, int(respCreatePage.Error.Code), respCreatePage.Error.Description)

	respOpenPagesSet := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
	require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)
	require.Len(t, respOpenPagesSet.Event.Messages, 2)
	show := respOpenPagesSet.Event.Messages[0].GetBlockShow()
	require.NotNil(t, show)

	require.Len(t, respOpenPagesSet.Event.Messages[1].GetBlockDataviewRecordsSet().Records, 1)
	require.Equal(t, respCreatePage.PageId, respOpenPagesSet.Event.Messages[1].GetBlockDataviewRecordsSet().Records[0].Fields["id"].GetStringValue())

	respCreatePage = mw.PageCreate(&pb.RpcPageCreateRequest{Details: &types2.Struct{Fields: map[string]*types2.Value{"name": pbtypes.String("test2")}}})
	require.Equal(t, 0, int(respCreatePage.Error.Code), respCreatePage.Error.Description)

	respOpenPagesSet = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
	require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)
	require.Len(t, respOpenPagesSet.Event.Messages, 2)
	show = respOpenPagesSet.Event.Messages[0].GetBlockShow()
	require.NotNil(t, show)
	require.Len(t, respOpenPagesSet.Event.Messages[1].GetBlockDataviewRecordsSet().Records, 2)
	require.Equal(t, respCreatePage.PageId, respOpenPagesSet.Event.Messages[1].GetBlockDataviewRecordsSet().Records[1].Fields["id"].GetStringValue())
}
