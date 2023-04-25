//go:build integration

package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func cachedString(key string, proc func() (string, error)) (string, bool, error) {
	filename := filepath.Join(cacheDir, key)
	raw, err := os.ReadFile(filename)
	result := string(raw)

	if os.IsNotExist(err) || result == "" {
		res, err := proc()
		if err != nil {
			return "", false, fmt.Errorf("running proc for caching %s: %w", key, err)
		}
		err = os.WriteFile(filename, []byte(res), 0600)
		if err != nil {
			return "", false, fmt.Errorf("writing cache for %s: %w", key, err)
		}
		return res, false, nil
	}

	return result, true, nil
}

func (s *testSuite) SetupSuite() {
	s.ctx = context.Background()

	c, err := newClient()
	s.Require().NoError(err)
	s.ClientCommandsClient = c

	mnemonic, _, err := cachedString("mnemonic", func() (string, error) {
		s.T().Log("creating new test account")
		return s.accountCreate(), nil
	})
	s.Require().NoError(err)
	s.T().Log("your mnemonic:", mnemonic)

	_ = call(s, c.WalletRecover, &pb.RpcWalletRecoverRequest{
		Mnemonic: mnemonic,
		RootPath: rootPath,
	})

	s.events = s.setSessionCtx(mnemonic)

	accountID, _, err := cachedString("account_id", func() (string, error) {
		s.T().Log("recovering the account")
		call(s, c.AccountRecover, &pb.RpcAccountRecoverRequest{})
		var res string
		waitEvent(s, func(a *pb.EventMessageValueOfAccountShow) {
			res = a.AccountShow.Account.Id
		})
		return res, nil
	})
	s.Require().NoError(err)
	s.T().Log("your account ID:", accountID)

	acc := call(s, c.AccountSelect, &pb.RpcAccountSelectRequest{
		Id: accountID,
	}).Account

	s.acc = acc
}

func (s *testSuite) TearDownSuite() {
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

const cacheDir = ".cache"

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
