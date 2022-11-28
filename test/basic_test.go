package test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pb/service"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func TestBasic(t *testing.T) {
	conn, err := grpc.Dial("127.0.0.1:31007", grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	c := service.NewClientCommandsClient(conn)

	const mnemonic = "lamp crane identify video setup cactus hat icon guard develop alert solar"
	const rootPath = "/var/anytype"
	ctx := context.Background()

	t.Run("WalletRecover", func(t *testing.T) {
		resp, err := c.WalletRecover(ctx, &pb.RpcWalletRecoverRequest{
			Mnemonic: mnemonic,
			RootPath: rootPath,
		})
		assert.NoError(t, json.NewEncoder(os.Stdout).Encode(resp))
		require.NoError(t, err)
	})

	var tok string
	t.Run("WalletCreateSession", func(t *testing.T) {
		resp, err := c.WalletCreateSession(ctx, &pb.RpcWalletCreateSessionRequest{
			Mnemonic: mnemonic,
		})
		require.NoError(t, err)
		tok = resp.Token
	})

	ctx = metadata.AppendToOutgoingContext(ctx, "token", tok)

	stream, err := c.ListenSessionEvents(ctx, &pb.StreamRequest{Token: tok})
	require.NoError(t, err)

	er := startEventReceiver(ctx, stream)

	t.Run("AccountRecover", func(t *testing.T) {
		resp, err := c.AccountRecover(ctx, &pb.RpcAccountRecoverRequest{})
		require.NoError(t, err)

		assert.NoError(t, json.NewEncoder(os.Stdout).Encode(resp))
	})

	t.Run("AccountSelect", func(t *testing.T) {
		var id string
		// TODO: log waiting for event?
		waitEvent(er, func(a *pb.EventMessageValueOfAccountShow) {
			id = a.AccountShow.Account.Id
		})

		resp, err := c.AccountSelect(ctx, &pb.RpcAccountSelectRequest{
			Id: id,
		})
		require.NoError(t, err)

		assert.NoError(t, json.NewEncoder(os.Stdout).Encode(resp))
	})

	t.Run("ObjectSearch", func(t *testing.T) {
		resp, err := c.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
			Keys: []string{"id", "type", "name"},
		})
		require.NoError(t, err)
		require.NotEmpty(t, resp.Records)
	})

	t.Run("ObjectSearchSubscribe", func(t *testing.T) {
		resp, err := c.ObjectSearchSubscribe(ctx, &pb.RpcObjectSearchSubscribeRequest{
			SubId: "recent",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyLastOpenedDate.String(),
					Condition:   model.BlockContentDataviewFilter_Greater,
				},
			},
			Keys: []string{"id", "lastOpenedDate"},
		})
		require.NoError(t, err)
		require.NotEmpty(t, resp.Records)
	})

	var objId string
	t.Run("BlockLinkCreateWithObject", func(t *testing.T) {
		resp, err := c.BlockLinkCreateWithObject(ctx, &pb.RpcBlockLinkCreateWithObjectRequest{
			InternalFlags: []*model.InternalFlag{
				{
					Value: model.InternalFlag_editorDeleteEmpty,
				},
				{
					Value: model.InternalFlag_editorSelectType,
				},
			},
			Details: &types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeyType.String(): pbtypes.String(bundle.TypeKeyNote.URL()),
				},
			},
		})

		require.NoError(t, err)
		require.NotEmpty(t, resp.TargetId)
		objId = resp.TargetId
	})

	t.Run("ObjectOpen", func(t *testing.T) {
		resp, err := c.ObjectOpen(ctx, &pb.RpcObjectOpenRequest{
			ObjectId: objId,
		})

		require.NoError(t, err)
		require.NotNil(t, resp.ObjectView)

		waitEvent(er, func(sa *pb.EventMessageValueOfSubscriptionAdd) {
			require.Equal(t, sa.SubscriptionAdd.Id, objId)
		})
		waitEvent(er, func(sa *pb.EventMessageValueOfObjectDetailsSet) {
			require.Equal(t, sa.ObjectDetailsSet.Id, objId)
			require.Contains(t, sa.ObjectDetailsSet.Details.Fields, bundle.RelationKeyLastOpenedDate.String())
		})
	})
}
