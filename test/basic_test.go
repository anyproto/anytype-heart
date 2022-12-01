package test

import (
	"context"
	"os"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/metadata"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pb/service"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const rootPath = "/var/anytype"

type testSuite struct {
	suite.Suite

	service.ClientCommandsClient

	ctx    context.Context
	acc    *model.Account
	events *eventReceiver
}

func (s *testSuite) Context() context.Context {
	return s.ctx
}

func (s *testSuite) SetupTest() {
	s.ctx = context.Background()

	c, err := newClient()
	s.Require().NoError(err)
	s.ClientCommandsClient = c

	raw, err := os.ReadFile(mnemonicFile)
	mnemonic := string(raw)
	if os.IsNotExist(err) || mnemonic == "" {
		s.T().Log("creating new test account")
		mnemonic = s.accountCreate()
		err := os.WriteFile(mnemonicFile, []byte(mnemonic), 0600)
		s.Require().NoError(err)
		s.T().Log("your mnemonic:", mnemonic)
	} else {
		s.T().Log("use existing mnemonic:", mnemonic)
	}

	_ = call(s, c.WalletRecover, &pb.RpcWalletRecoverRequest{
		Mnemonic: mnemonic,
		RootPath: rootPath,
	})

	s.events = s.createSessionCtx(mnemonic)

	call(s, c.AccountRecover, &pb.RpcAccountRecoverRequest{})
	var id string
	waitEvent(s.events, func(a *pb.EventMessageValueOfAccountShow) {
		id = a.AccountShow.Account.Id
	})
	acc := call(s, c.AccountSelect, &pb.RpcAccountSelectRequest{
		Id: id,
	}).Account

	s.acc = acc
}

func (s *testSuite) TearDownTest() {
	call(s, s.AccountStop, &pb.RpcAccountStopRequest{
		RemoveData: true,
	})

	call(s, s.WalletCloseSession, &pb.RpcWalletCloseSessionRequest{
		Token: s.events.token,
	})
}

// TODO rename to setSessionCtx
func (s *testSuite) createSessionCtx(mnemonic string) *eventReceiver {
	tok := call(s, s.WalletCreateSession, &pb.RpcWalletCreateSessionRequest{
		Mnemonic: mnemonic,
	}).Token

	s.ctx = metadata.AppendToOutgoingContext(s.ctx, "token", tok)

	events, err := startEventReceiver(s.ctx, s, tok)
	s.Require().NoError(err)

	return events
}

func (s *testSuite) accountCreate() string {
	s.ctx = context.Background()

	mnemonic := call(s, s.WalletCreate, &pb.RpcWalletCreateRequest{
		RootPath: rootPath,
	}).Mnemonic

	events := s.createSessionCtx(mnemonic)

	acc := call(s, s.AccountCreate, &pb.RpcAccountCreateRequest{
		Name:            "John Doe",
		AlphaInviteCode: "elbrus",
		StorePath:       rootPath,
	})

	t := s.T()
	require.NotNil(t, acc.Account)
	require.NotNil(t, acc.Account.Info)
	assert.NotEmpty(t, acc.Account.Id)

	call(s, s.AccountStop, &pb.RpcAccountStopRequest{
		RemoveData: true,
	})
	call(s, s.WalletCloseSession, &pb.RpcWalletCloseSessionRequest{
		Token: events.token,
	})

	return mnemonic
}

const mnemonicFile = "mnemonic.txt"

func (s *testSuite) TestBasic() {
	s.Run("open dashboard", func() {
		t := s.T()
		resp := call(s, s.ObjectOpen, &pb.RpcObjectOpenRequest{
			ObjectId: s.acc.Info.HomeObjectId,
		})

		require.NotNil(t, resp.ObjectView)
		assert.NotEmpty(t, resp.ObjectView.Blocks)
		assert.NotEmpty(t, resp.ObjectView.Details)
		assert.NotEmpty(t, resp.ObjectView.Restrictions)
		assert.NotEmpty(t, resp.ObjectView.RelationLinks)
		assert.NotZero(t, resp.ObjectView.Type)
	})

	t := s.T()
	{
		resp := call(s, s.ObjectSearch, &pb.RpcObjectSearchRequest{
			Keys: []string{"id", "type", "name"},
		})
		require.NotEmpty(t, resp.Records)
	}

	call(s, s.ObjectSearchSubscribe, &pb.RpcObjectSearchSubscribeRequest{
		SubId: "recent",
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLastOpenedDate.String(),
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
		},
		Keys: []string{"id", "lastOpenedDate"},
	})

	objId := call(s, s.BlockLinkCreateWithObject, &pb.RpcBlockLinkCreateWithObjectRequest{
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
		t = s.T()

		resp := call(s, s.ObjectOpen, &pb.RpcObjectOpenRequest{
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

func TestBasic(t *testing.T) {
	suite.Run(t, &testSuite{})
}
