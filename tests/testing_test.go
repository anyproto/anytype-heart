//go:build integration

package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const rootPath = "/var/anytype"

type testSuite struct {
	suite.Suite

	*testSession
}

func TestBasic(t *testing.T) {
	suite.Run(t, &testSuite{})
}

func (s *testSession) recoverAccount(t *testing.T) (accountID string) {
	cctx := s.newCallCtx(t)
	t.Log("recovering the account")
	call(cctx, s.AccountRecover, &pb.RpcAccountRecoverRequest{})
	waitEvent(t, s, func(a *pb.EventMessageValueOfAccountShow) {
		accountID = a.AccountShow.Account.Id
	})
	return accountID
}

func (s *testSuite) SetupSuite() {
	port := os.Getenv("ANYTYPE_TEST_GRPC_PORT")
	if port == "" {
		s.FailNow("you must specify ANYTYPE_TEST_GRPC_PORT env variable")
	}

	s.testSession = newTestSession(s.T(), port, "mnemonic", "account_id")
}

type testSession struct {
	service.ClientCommandsClient

	acc           *model.Account
	eventReceiver *eventReceiver
	token         string
}

func (s *testSession) EventReceiver() *eventReceiver {
	return s.eventReceiver
}

// TODO Pass cache path
func newTestSession(t *testing.T, port string, mnemonicKey string, accountIDKey string) *testSession {
	var s testSession

	c, err := newClient(port)
	require.NoError(t, err)
	s.ClientCommandsClient = c

	mnemonic, _, err := cachedString(mnemonicKey, false, func() (string, error) {
		t.Log("creating new test account")
		return s.accountCreate(t), nil
	})
	require.NoError(t, err)
	t.Log("your mnemonic:", mnemonic)

	cctx := s.newCallCtx(t)
	_ = call(cctx, s.WalletRecover, &pb.RpcWalletRecoverRequest{
		Mnemonic: mnemonic,
		RootPath: rootPath,
	})

	cctx, s.eventReceiver = s.openClientSession(t, mnemonic)

	accountID, _, err := cachedString(accountIDKey, false, func() (string, error) {
		return s.recoverAccount(t), nil
	})
	require.NoError(t, err)
	t.Log("your account ID:", accountID)

	resp, err := callReturnError(cctx, s.AccountSelect, &pb.RpcAccountSelectRequest{
		Id:       accountID,
		RootPath: rootPath,
	})
	if err != nil {
		t.Log("can't select account, recovering...")
		accountID, _, err = cachedString(accountIDKey, true, func() (string, error) {
			return s.recoverAccount(t), nil
		})
		require.NoError(t, err)
		t.Log("freshly recovered account ID:", accountID)
		resp, err = callReturnError(cctx, s.AccountSelect, &pb.RpcAccountSelectRequest{
			Id:       accountID,
			RootPath: rootPath,
		})
		require.NoError(t, err)
	}

	s.acc = resp.Account

	return &s
}

func (s *testSuite) TearDownSuite() {
	// Do nothing if client hasn't been started
	if s.ClientCommandsClient == nil {
		return
	}

	s.stopAccount(s.T())
}

func (s *testSession) stopAccount(t *testing.T) {
	cctx := s.newCallCtx(t)
	call(cctx, s.AccountStop, &pb.RpcAccountStopRequest{
		RemoveData: false,
	})

	call(cctx, s.WalletCloseSession, &pb.RpcWalletCloseSessionRequest{
		Token: s.eventReceiver.token,
	})
}

func (s *testSession) openClientSession(t *testing.T, mnemonic string) (callCtx, *eventReceiver) {
	cctx := s.newCallCtx(t)
	tok := call(cctx, s.WalletCreateSession, &pb.RpcWalletCreateSessionRequest{
		Mnemonic: mnemonic,
	}).Token

	s.token = tok
	cctx = s.newCallCtx(t)

	events, err := startEventReceiver(cctx.newContext(), s, tok)
	require.NoError(t, err)

	return cctx, events
}

func (s *testSession) accountCreate(t *testing.T) string {
	cctx := s.newCallCtx(t)

	mnemonic := call(cctx, s.WalletCreate, &pb.RpcWalletCreateRequest{
		RootPath: rootPath,
	}).Mnemonic

	cctx, events := s.openClientSession(t, mnemonic)

	acc := call(cctx, s.AccountCreate, &pb.RpcAccountCreateRequest{
		Name:      "John Doe",
		StorePath: rootPath,
	})

	require.NotNil(t, acc.Account)
	require.NotNil(t, acc.Account.Info)
	assert.NotEmpty(t, acc.Account.Id)

	time.Sleep(1 * time.Minute)

	call(cctx, s.AccountStop, &pb.RpcAccountStopRequest{
		RemoveData: false,
	})
	call(cctx, s.WalletCloseSession, &pb.RpcWalletCloseSessionRequest{
		Token: events.token,
	})

	return mnemonic
}

func newClient(port string) (service.ClientCommandsClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, ":"+port, grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return service.NewClientCommandsClient(conn), nil
}
