//go:build integration

package tests

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pb/service"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

const rootPath = "/var/anytype"

type testSuite struct {
	suite.Suite

	service.ClientCommandsClient

	ctx    context.Context
	acc    *model.Account
	events *eventReceiver
}

func TestBasic(t *testing.T) {
	suite.Run(t, &testSuite{})
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

	s.events = s.setSessionCtx(mnemonic)

	call(s, c.AccountRecover, &pb.RpcAccountRecoverRequest{})
	var id string
	waitEvent(s, func(a *pb.EventMessageValueOfAccountShow) {
		id = a.AccountShow.Account.Id
	})
	acc := call(s, c.AccountSelect, &pb.RpcAccountSelectRequest{
		Id: id,
	}).Account

	s.acc = acc
}

func (s *testSuite) TearDownTest() {
	// Do nothing if client hasn't been started
	if s.ClientCommandsClient == nil {
		return
	}
	call(s, s.AccountStop, &pb.RpcAccountStopRequest{
		RemoveData: true,
	})

	call(s, s.WalletCloseSession, &pb.RpcWalletCloseSessionRequest{
		Token: s.events.token,
	})
}

func (s *testSuite) setSessionCtx(mnemonic string) *eventReceiver {
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

	events := s.setSessionCtx(mnemonic)

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

func getError(i interface{}) (int, string) {
	v := reflect.ValueOf(i).Elem()

	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() != reflect.Pointer {
			continue
		}
		el := f.Elem()
		if !el.IsValid() {
			continue
		}
		if strings.Contains(el.Type().Name(), "ResponseError") {
			code := el.FieldByName("Code").Int()
			desc := el.FieldByName("Description").String()
			return int(code), desc
		}
	}
	return 0, ""
}

type callCtx interface {
	T() *testing.T
	Context() context.Context
}

func call[reqT, respT any](
	cctx callCtx,
	method func(context.Context, reqT, ...grpc.CallOption) (respT, error),
	req reqT,
) respT {
	name := runtime.FuncForPC(reflect.ValueOf(method).Pointer()).Name()
	name = name[strings.LastIndex(name, ".")+1:]
	name = name[:strings.LastIndex(name, "-")]
	t := cctx.T()
	t.Logf("calling %s", name)

	resp, err := method(cctx.Context(), req)
	require.NoError(t, err)
	code, desc := getError(resp)
	require.Zero(t, code, desc)
	require.NotNil(t, resp)
	return resp
}

func newClient() (service.ClientCommandsClient, error) {
	port := os.Getenv("ANYTYPE_TEST_GRPC_PORT")
	if port == "" {
		return nil, fmt.Errorf("you must specify ANYTYPE_TEST_GRPC_PORT env variable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, ":"+port, grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return service.NewClientCommandsClient(conn), nil
}
