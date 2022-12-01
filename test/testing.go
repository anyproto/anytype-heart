package test

import (
	"context"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/anytypeio/go-anytype-middleware/pb/service"
)

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

type testingT interface {
	require.TestingT
	Logf(string, ...any)
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
		port = "31077"
	}
	conn, err := grpc.Dial(":"+port, grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return service.NewClientCommandsClient(conn), nil
}
