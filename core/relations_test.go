package core

import (
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

	respOpenPagesSet := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
	require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)
	require.Len(t, respOpenPagesSet.Event.Messages, 2)
	block := getBlockById("dataview", respOpenPagesSet.Event.Messages[0].GetBlockShow().Blocks)

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
		respOpenPagesSet = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)
		require.Len(t, respOpenPagesSet.Event.Messages, 2)
		block = getBlockById("dataview", respOpenPagesSet.Event.Messages[0].GetBlockShow().Blocks)

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
				Name:     "new",
				ReadOnly: false,
			},
		})

		require.Equal(t, 0, int(respDataviewRelationAdd.Error.Code), respDataviewRelationAdd.Error.Description)
		respOpenPagesSet = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)
		require.Len(t, respOpenPagesSet.Event.Messages, 2)
		block = getBlockById("dataview", respOpenPagesSet.Event.Messages[0].GetBlockShow().Blocks)
		relKey = respDataviewRelationAdd.RelationKey
		require.Len(t, block.GetDataview().Relations, len(relation.BundledObjectTypes["page"].Relations)+1)

		respAccountCreate := mw.AccountSelect(&pb.RpcAccountSelectRequest{Id: mw.Anytype.Account(), RootPath: rootPath})
		require.Equal(t, 0, int(respAccountCreate.Error.Code))
		respOpenPagesSet = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)
		require.Len(t, respOpenPagesSet.Event.Messages, 2)
		block = getBlockById("dataview", respOpenPagesSet.Event.Messages[0].GetBlockShow().Blocks)
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
		respOpenPagesSet = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)
		require.Len(t, respOpenPagesSet.Event.Messages, 2)
		block = getBlockById("dataview", respOpenPagesSet.Event.Messages[0].GetBlockShow().Blocks)

		require.Len(t, block.GetDataview().Relations, len(relation.BundledObjectTypes["page"].Relations)+1)
		require.Equal(t, "new", block.GetDataview().Relations[len(block.GetDataview().Relations)-1].Name)

	})

	t.Run("update_cant_change_format", func(t *testing.T) {
		respUpdate := mw.BlockDataviewRelationUpdate(&pb.RpcBlockDataviewRelationUpdateRequest{
			ContextId:   mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: relKey,
			Relation: &pbrelation.Relation{
				Key:      relKey,
				Format:   1,
				Name:     "new_changed",
				ReadOnly: false,
			},
		})
		require.Equal(t, pb.RpcBlockDataviewRelationUpdateResponseError_BAD_INPUT, respUpdate.Error.Code, respUpdate.Error.Description)
		respOpenPagesSet = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)
		require.Len(t, respOpenPagesSet.Event.Messages, 2)
		block = getBlockById("dataview", respOpenPagesSet.Event.Messages[0].GetBlockShow().Blocks)

		require.Len(t, block.GetDataview().Relations, len(relation.BundledObjectTypes["page"].Relations)+1)
		require.Equal(t, "new", block.GetDataview().Relations[len(block.GetDataview().Relations)-1].Name)
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
		respOpenPagesSet = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)
		require.Len(t, respOpenPagesSet.Event.Messages, 2)
		block = getBlockById("dataview", respOpenPagesSet.Event.Messages[0].GetBlockShow().Blocks)

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
		respOpenPagesSet = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)
		require.Len(t, respOpenPagesSet.Event.Messages, 2)
		block = getBlockById("dataview", respOpenPagesSet.Event.Messages[0].GetBlockShow().Blocks)

		require.Len(t, block.GetDataview().Relations, len(relation.BundledObjectTypes["page"].Relations)+1)

	})

	t.Run("delete_correct", func(t *testing.T) {
		respDataviewRelationDelete := mw.BlockDataviewRelationDelete(&pb.RpcBlockDataviewRelationDeleteRequest{
			ContextId:   mw.Anytype.PredefinedBlocks().SetPages,
			BlockId:     "dataview",
			RelationKey: relKey,
		})
		require.Equal(t, 0, int(respDataviewRelationDelete.Error.Code), respDataviewRelationDelete.Error.Description)
		respOpenPagesSet = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
		require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)
		require.Len(t, respOpenPagesSet.Event.Messages, 2)
		block = getBlockById("dataview", respOpenPagesSet.Event.Messages[0].GetBlockShow().Blocks)

		require.Len(t, block.GetDataview().Relations, len(relation.BundledObjectTypes["page"].Relations))
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
				{Format: pbrelation.RelationFormat_object, Name: "bio", ObjectType: "https://anytype.io/schemas/object/bundled/pages"},
			},
		},
	})

	require.Equal(t, 0, int(respObjectTypeCreate.Error.Code), respObjectTypeCreate.Error.Description)
	require.Len(t, respObjectTypeCreate.ObjectType.Relations, 8) // including relation.RequiredInternalRelations
	require.True(t, strings.HasPrefix(respObjectTypeCreate.ObjectType.Url, "https://anytype.io/schemas/object/custom/"))
	newRelation := respObjectTypeCreate.ObjectType.Relations[7]

	respObjectTypeList = mw.ObjectTypeList(nil)
	require.Equal(t, 0, int(respObjectTypeList.Error.Code), respObjectTypeList.Error.Description)
	require.Len(t, respObjectTypeList.ObjectTypes, len(relation.BundledObjectTypes)+1)
	lastObjType := respObjectTypeList.ObjectTypes[len(respObjectTypeList.ObjectTypes)-1]
	require.Equal(t, respObjectTypeCreate.ObjectType.Url, lastObjType.Url)
	require.Len(t, lastObjType.Relations, 8)

	respCreateCustomTypeSet := mw.SetCreate(&pb.RpcSetCreateRequest{
		ObjectTypeUrl: respObjectTypeCreate.ObjectType.Url,
	})
	require.Equal(t, 0, int(respCreateCustomTypeSet.Error.Code), respCreateCustomTypeSet.Error.Description)
	require.NotEmpty(t, respCreateCustomTypeSet.Id)

	respOpenCustomTypeSet := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: respCreateCustomTypeSet.Id})
	require.Equal(t, 0, int(respOpenCustomTypeSet.Error.Code), respOpenCustomTypeSet.Error.Description)

	respCreateRecordInCustomTypeSet := mw.BlockCreateDataviewRecord(&pb.RpcBlockCreateDataviewRecordRequest{ContextId: respCreateCustomTypeSet.Id, BlockId: "dataview", Record: &types2.Struct{Fields: map[string]*types2.Value{"name": pbtypes.String("custom1"), newRelation.Key: pbtypes.String("newRelationVal")}}})
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
