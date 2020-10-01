package core

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/pb"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
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

	resp2 := mw.AccountCreate(&pb.RpcAccountCreateRequest{Name: "test", AlphaInviteCode: "elbrus"})
	require.Equal(t, 0, int(resp2.Error.Code))

	resp3 := mw.ObjectTypeCreate(&pb.RpcObjectTypeCreateRequest{
		ObjectType: &pbrelation.ObjectType{
			Name: "1",
			Relations: []*pbrelation.Relation{
				{Format: pbrelation.RelationFormat_date, Name: "date of birth"},
				{Format: pbrelation.RelationFormat_objectId, Name: "bio", ObjectType: "https://anytype.io/schemas/object/bundled/pages"},
			},
		},
	})
	require.Equal(t, 0, int(resp3.Error.Code), resp3.Error.Description)
	require.Len(t, resp3.ObjectType.Relations, 2)
	require.True(t, strings.HasPrefix(resp3.ObjectType.Url, "https://anytype.io/schemas/object/custom/"))

	resp4 := mw.ObjectTypeList(nil)
	require.Equal(t, 0, int(resp4.Error.Code), resp4.Error.Description)
	require.Len(t, resp4.ObjectTypes, 2)
	require.Equal(t, resp3.ObjectType.Url, resp4.ObjectTypes[1].Url)
	require.Len(t, resp4.ObjectTypes[1].Relations, 2)

	resp5 := mw.SetCreate(&pb.RpcSetCreateRequest{
		ObjectTypeURL: resp4.ObjectTypes[1].Url,
	})
	require.Equal(t, 0, int(resp5.Error.Code), resp5.Error.Description)
	require.NotEmpty(t, resp5.PageId)
}
