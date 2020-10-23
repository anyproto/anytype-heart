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

	resp := mw.WalletCreate(&pb.RpcWalletCreateRequest{RootPath: rootPath})
	require.Equal(t, 0, int(resp.Error.Code))

	resp2 := mw.AccountCreate(&pb.RpcAccountCreateRequest{Name: "profile", AlphaInviteCode: "elbrus"})
	require.Equal(t, 0, int(resp2.Error.Code))

	resp2_1 := mw.ObjectTypeList(nil)
	require.Equal(t, 0, int(resp2_1.Error.Code), resp2_1.Error.Description)
	require.Len(t, resp2_1.ObjectTypes, 2)

	resp3 := mw.ObjectTypeCreate(&pb.RpcObjectTypeCreateRequest{
		ObjectType: &pbrelation.ObjectType{
			Name: "1",
			Relations: []*pbrelation.Relation{
				{Format: pbrelation.RelationFormat_date, Name: "date of birth"},
				{Format: pbrelation.RelationFormat_object, Name: "bio", ObjectType: "https://anytype.io/schemas/object/bundled/pages"},
			},
		},
	})

	require.Equal(t, 0, int(resp3.Error.Code), resp3.Error.Description)
	require.Len(t, resp3.ObjectType.Relations, 7) // including relation.RequiredInternalRelations
	require.True(t, strings.HasPrefix(resp3.ObjectType.Url, "https://anytype.io/schemas/object/custom/"))

	resp4 := mw.ObjectTypeList(nil)
	require.Equal(t, 0, int(resp4.Error.Code), resp4.Error.Description)
	require.Len(t, resp4.ObjectTypes, 3)
	require.Equal(t, resp3.ObjectType.Url, resp4.ObjectTypes[2].Url)
	require.Len(t, resp4.ObjectTypes[2].Relations, 7)

	resp2_3 := mw.AccountSelect(&pb.RpcAccountSelectRequest{Id: resp2.Account.Id, RootPath: rootPath})
	require.Equal(t, 0, int(resp2_3.Error.Code))

	resp4 = mw.ObjectTypeList(nil)
	require.Equal(t, 0, int(resp4.Error.Code), resp4.Error.Description)
	require.Len(t, resp4.ObjectTypes, 3)
	require.Equal(t, resp3.ObjectType.Url, resp4.ObjectTypes[2].Url)
	require.Len(t, resp4.ObjectTypes[2].Relations, 7)

	resp5 := mw.SetCreate(&pb.RpcSetCreateRequest{
		ObjectTypeUrl: resp4.ObjectTypes[2].Url,
	})
	require.Equal(t, 0, int(resp5.Error.Code), resp5.Error.Description)
	require.NotEmpty(t, resp5.Id)

	resp6 := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: resp5.Id})
	require.Equal(t, 0, int(resp6.Error.Code), resp6.Error.Description)

	respCreate1 := mw.BlockCreateDataviewRecord(&pb.RpcBlockCreateDataviewRecordRequest{ContextId: resp5.Id, BlockId: "dataview", Record: &types2.Struct{Fields: map[string]*types2.Value{"name": pbtypes.String("custom1")}}})
	require.Equal(t, 0, int(respCreate1.Error.Code), respCreate1.Error.Description)

	resp6 = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: resp5.Id})
	require.Equal(t, 0, int(resp6.Error.Code), resp6.Error.Description)

	require.Len(t, resp6.Event.Messages, 2)
	require.Len(t, resp6.Event.Messages[1].GetBlockSetDataviewRecords().Inserted, 1)
	require.Equal(t, respCreate1.Record.Fields["id"].GetStringValue(), resp6.Event.Messages[1].GetBlockSetDataviewRecords().Inserted[0].Fields["id"].GetStringValue())

	show := resp6.Event.Messages[0].GetBlockShow()
	require.NotNil(t, show)

	respCreatePage := mw.PageCreate(&pb.RpcPageCreateRequest{Details: &types2.Struct{Fields: map[string]*types2.Value{"name": pbtypes.String("test1")}}})
	require.Equal(t, 0, int(respCreatePage.Error.Code), respCreatePage.Error.Description)

	resp7 := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
	require.Equal(t, 0, int(resp7.Error.Code), resp7.Error.Description)
	require.Len(t, resp7.Event.Messages, 2)
	show = resp7.Event.Messages[0].GetBlockShow()
	require.NotNil(t, show)
	require.Len(t, resp7.Event.Messages[1].GetBlockSetDataviewRecords().Inserted, 1)
	require.Equal(t, respCreatePage.PageId, resp7.Event.Messages[1].GetBlockSetDataviewRecords().Inserted[0].Fields["id"].GetStringValue())

	respCreatePage = mw.PageCreate(&pb.RpcPageCreateRequest{Details: &types2.Struct{Fields: map[string]*types2.Value{"name": pbtypes.String("test2")}}})
	require.Equal(t, 0, int(respCreatePage.Error.Code), respCreatePage.Error.Description)

	resp7 = mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: mw.Anytype.PredefinedBlocks().SetPages})
	require.Equal(t, 0, int(resp7.Error.Code), resp7.Error.Description)
	require.Len(t, resp7.Event.Messages, 2)
	show = resp6.Event.Messages[0].GetBlockShow()
	require.NotNil(t, show)
	require.Len(t, resp7.Event.Messages[1].GetBlockSetDataviewRecords().Inserted, 2)
	require.Equal(t, respCreatePage.PageId, resp7.Event.Messages[1].GetBlockSetDataviewRecords().Inserted[1].Fields["id"].GetStringValue())
}
