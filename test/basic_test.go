package test

import (
	"context"
	"os"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pb/service"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const rootPath = "/var/anytype"

func createSessionCtx(t *testing.T, c service.ClientCommandsClient, ctx context.Context, mnemonic string) (context.Context, *eventReceiver) {
	tok := call(t, ctx, c.WalletCreateSession, &pb.RpcWalletCreateSessionRequest{
		Mnemonic: mnemonic,
	}).Token

	ctx = metadata.AppendToOutgoingContext(ctx, "token", tok)

	events, err := startEventReceiver(ctx, c, tok)
	require.NoError(t, err)

	return ctx, events
}

func accountCreate(t *testing.T, c service.ClientCommandsClient) string {
	ctx := context.Background()

	mnemonic := call(t, ctx, c.WalletCreate, &pb.RpcWalletCreateRequest{
		RootPath: rootPath,
	}).Mnemonic

	ctx, events := createSessionCtx(t, c, ctx, mnemonic)

	acc := call(t, ctx, c.AccountCreate, &pb.RpcAccountCreateRequest{
		Name:            "John Doe",
		AlphaInviteCode: "elbrus",
		StorePath:       rootPath,
	})

	require.NotNil(t, acc.Account)
	require.NotNil(t, acc.Account.Info)
	assert.NotEmpty(t, acc.Account.Id)

	call(t, ctx, c.AccountStop, &pb.RpcAccountStopRequest{
		RemoveData: true,
	})
	call(t, ctx, c.WalletCloseSession, &pb.RpcWalletCloseSessionRequest{
		Token: events.token,
	})

	return mnemonic
}

const mnemonicFile = "mnemonic.txt"

type testSession struct {
	service.ClientCommandsClient

	ctx    context.Context
	acc    *model.Account
	events *eventReceiver
}

func createTestSession(t *testing.T) (context.Context, *testSession) {
	c, err := newClient()
	require.NoError(t, err)

	raw, err := os.ReadFile(mnemonicFile)
	mnemonic := string(raw)
	if os.IsNotExist(err) || mnemonic == "" {
		t.Log("creating new test account")
		mnemonic = accountCreate(t, c)
		err := os.WriteFile(mnemonicFile, []byte(mnemonic), 0600)
		require.NoError(t, err)
		t.Log("your mnemonic:", mnemonic)
	} else {
		t.Log("use existing mnemonic:", mnemonic)
	}

	ctx := context.Background()

	var events *eventReceiver
	var acc *model.Account
	t.Run("login", func(t *testing.T) {
		_ = call(t, ctx, c.WalletRecover, &pb.RpcWalletRecoverRequest{
			Mnemonic: mnemonic,
			RootPath: rootPath,
		})

		ctx, events = createSessionCtx(t, c, ctx, mnemonic)

		call(t, ctx, c.AccountRecover, &pb.RpcAccountRecoverRequest{})
		var id string
		waitEvent(events, func(a *pb.EventMessageValueOfAccountShow) {
			id = a.AccountShow.Account.Id
		})
		acc = call(t, ctx, c.AccountSelect, &pb.RpcAccountSelectRequest{
			Id: id,
		}).Account
	})

	return ctx, &testSession{
		ctx:                  ctx,
		ClientCommandsClient: c,
		events:               events,
		acc:                  acc,
	}
}

func (s *testSession) close(t *testing.T) {
	t.Run("log out", func(t *testing.T) {
		call(t, s.ctx, s.AccountStop, &pb.RpcAccountStopRequest{
			RemoveData: true,
		})

		call(t, s.ctx, s.WalletCloseSession, &pb.RpcWalletCloseSessionRequest{
			Token: s.events.token,
		})
	})
}

func TestBasic(t *testing.T) {
	ctx, s := createTestSession(t)
	defer s.close(t)

	t.Run("open dashboard", func(t *testing.T) {
		resp := call(t, ctx, s.ObjectOpen, &pb.RpcObjectOpenRequest{
			ObjectId: s.acc.Info.HomeObjectId,
		})

		require.NotNil(t, resp.ObjectView)
		assert.NotEmpty(t, resp.ObjectView.Blocks)
		assert.NotEmpty(t, resp.ObjectView.Details)
		assert.NotEmpty(t, resp.ObjectView.Restrictions)
		assert.NotEmpty(t, resp.ObjectView.RelationLinks)
		assert.NotZero(t, resp.ObjectView.Type)
	})

	{
		resp := call(t, ctx, s.ObjectSearch, &pb.RpcObjectSearchRequest{
			Keys: []string{"id", "type", "name"},
		})
		require.NotEmpty(t, resp.Records)
	}

	call(t, ctx, s.ObjectSearchSubscribe, &pb.RpcObjectSearchSubscribeRequest{
		SubId: "recent",
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLastOpenedDate.String(),
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
		},
		Keys: []string{"id", "lastOpenedDate"},
	})

	objId := call(t, ctx, s.BlockLinkCreateWithObject, &pb.RpcBlockLinkCreateWithObjectRequest{
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
	}).TargetId

	t.Run("open an object", func(t *testing.T) {
		resp := call(t, ctx, s.ObjectOpen, &pb.RpcObjectOpenRequest{
			ObjectId: objId,
		})
		require.NotNil(t, resp.ObjectView)

		waitEvent(s.events, func(sa *pb.EventMessageValueOfSubscriptionAdd) {
			require.Equal(t, sa.SubscriptionAdd.Id, objId)
		})
		waitEvent(s.events, func(sa *pb.EventMessageValueOfObjectDetailsSet) {
			require.Equal(t, sa.ObjectDetailsSet.Id, objId)
			require.Contains(t, sa.ObjectDetailsSet.Details.Fields, bundle.RelationKeyLastOpenedDate.String())
		})
	})

}
