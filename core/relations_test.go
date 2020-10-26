package core

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/pb"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	types2 "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
)

func TestRelations(t *testing.T) {
	mw := New()
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

	respObjectTypeList := mw.ObjectTypeList(nil)
	require.Equal(t, 0, int(respObjectTypeList.Error.Code), respObjectTypeList.Error.Description)
	require.Len(t, respObjectTypeList.ObjectTypes, 2)

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

	respObjectTypeList = mw.ObjectTypeList(nil)
	require.Equal(t, 0, int(respObjectTypeList.Error.Code), respObjectTypeList.Error.Description)
	require.Len(t, respObjectTypeList.ObjectTypes, 3)
	require.Equal(t, respObjectTypeCreate.ObjectType.Url, respObjectTypeList.ObjectTypes[2].Url)
	require.Len(t, respObjectTypeList.ObjectTypes[2].Relations, 8)

	respAccountSelect := mw.AccountSelect(&pb.RpcAccountSelectRequest{Id: respAccountCreate.Account.Id, RootPath: rootPath})
	require.Equal(t, 0, int(respAccountSelect.Error.Code))

	respObjectTypeList = mw.ObjectTypeList(nil)
	require.Equal(t, 0, int(respObjectTypeList.Error.Code), respObjectTypeList.Error.Description)
	require.Len(t, respObjectTypeList.ObjectTypes, 3)
	require.Equal(t, respObjectTypeCreate.ObjectType.Url, respObjectTypeList.ObjectTypes[2].Url)
	require.Len(t, respObjectTypeList.ObjectTypes[2].Relations, 8)

	respCreateCustomTypeSet := mw.SetCreate(&pb.RpcSetCreateRequest{
		ObjectTypeUrl: respObjectTypeList.ObjectTypes[2].Url,
	})
	require.Equal(t, 0, int(respCreateCustomTypeSet.Error.Code), respCreateCustomTypeSet.Error.Description)
	require.NotEmpty(t, respCreateCustomTypeSet.Id)

	respOpenCustomTypeSet := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: respCreateCustomTypeSet.Id})
	require.Equal(t, 0, int(respOpenCustomTypeSet.Error.Code), respOpenCustomTypeSet.Error.Description)

	respCreateRecordInCustomTypeSet := mw.BlockCreateDataviewRecord(&pb.RpcBlockCreateDataviewRecordRequest{ContextId: respCreateCustomTypeSet.Id, BlockId: "dataview", Record: &types2.Struct{Fields: map[string]*types2.Value{"name": pbtypes.String("custom1")}}})
	require.Equal(t, 0, int(respCreateRecordInCustomTypeSet.Error.Code), respCreateRecordInCustomTypeSet.Error.Description)

	respOpenCustomTypeObject := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: respCreateRecordInCustomTypeSet.Record.Fields["id"].GetStringValue()})
	require.Equal(t, 0, int(respOpenCustomTypeObject.Error.Code), respOpenCustomTypeObject.Error.Description)
	require.Len(t, respOpenCustomTypeObject.Event.Messages, 1)
	show := respOpenCustomTypeObject.Event.Messages[0].GetBlockShow()
	require.NotNil(t, show)
	require.Len(t, show.ObjectTypes, 1)
	require.Len(t, show.ObjectTypesPerObject, 1)
	require.Len(t, show.RelationsPerObject, 1)
	require.Equal(t, show.ObjectTypes[0], respObjectTypeCreate.ObjectType)

	respOpenCustomTypeSet = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: respCreateCustomTypeSet.Id})
	require.Equal(t, 0, int(respOpenCustomTypeSet.Error.Code), respOpenCustomTypeSet.Error.Description)

	require.Len(t, respOpenCustomTypeSet.Event.Messages, 2)
	require.Len(t, respOpenCustomTypeSet.Event.Messages[1].GetBlockSetDataviewRecords().Inserted, 1)
	require.Equal(t, respCreateRecordInCustomTypeSet.Record.Fields["id"].GetStringValue(), respOpenCustomTypeSet.Event.Messages[1].GetBlockSetDataviewRecords().Inserted[0].Fields["id"].GetStringValue())

	show = respOpenCustomTypeSet.Event.Messages[0].GetBlockShow()
	require.NotNil(t, show)

	respCreatePage := mw.PageCreate(&pb.RpcPageCreateRequest{Details: &types2.Struct{Fields: map[string]*types2.Value{"name": pbtypes.String("test1")}}})
	require.Equal(t, 0, int(respCreatePage.Error.Code), respCreatePage.Error.Description)

	respOpenPagesSet := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
	require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)
	require.Len(t, respOpenPagesSet.Event.Messages, 2)
	show = respOpenPagesSet.Event.Messages[0].GetBlockShow()
	require.NotNil(t, show)
	require.Len(t, respOpenPagesSet.Event.Messages[1].GetBlockSetDataviewRecords().Inserted, 1)
	require.Equal(t, respCreatePage.PageId, respOpenPagesSet.Event.Messages[1].GetBlockSetDataviewRecords().Inserted[0].Fields["id"].GetStringValue())

	respCreatePage = mw.PageCreate(&pb.RpcPageCreateRequest{Details: &types2.Struct{Fields: map[string]*types2.Value{"name": pbtypes.String("test2")}}})
	require.Equal(t, 0, int(respCreatePage.Error.Code), respCreatePage.Error.Description)

	respOpenPagesSet = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
	require.Equal(t, 0, int(respOpenPagesSet.Error.Code), respOpenPagesSet.Error.Description)
	require.Len(t, respOpenPagesSet.Event.Messages, 2)
	show = respOpenCustomTypeSet.Event.Messages[0].GetBlockShow()
	require.NotNil(t, show)
	require.Len(t, respOpenPagesSet.Event.Messages[1].GetBlockSetDataviewRecords().Inserted, 2)
	require.Equal(t, respCreatePage.PageId, respOpenPagesSet.Event.Messages[1].GetBlockSetDataviewRecords().Inserted[1].Fields["id"].GetStringValue())
}
