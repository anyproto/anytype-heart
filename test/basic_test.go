package test

import (
	"context"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pb/service"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
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

func call[reqT, respT any](t *testing.T, ctx context.Context,
	method func(context.Context, reqT, ...grpc.CallOption) (respT, error),
	req reqT,
) respT {
	name := runtime.FuncForPC(reflect.ValueOf(method).Pointer()).Name()
	name = name[strings.LastIndex(name, ".")+1:]
	name = name[:strings.LastIndex(name, "-")]
	t.Logf("calling %s", name)

	resp, err := method(ctx, req)
	require.NoError(t, err)
	code, desc := getError(resp)
	require.Zero(t, code, desc)
	require.NotNil(t, resp)
	return resp
}

func TestBasic(t *testing.T) {
	conn, err := grpc.Dial("127.0.0.1:31007", grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	c := service.NewClientCommandsClient(conn)

	const mnemonic = "lamp crane identify video setup cactus hat icon guard develop alert solar"
	const rootPath = "/var/anytype"
	ctx := context.Background()

	var er *eventReceiver
	t.Run("login", func(t *testing.T) {
		_ = call(t, ctx, c.WalletRecover, &pb.RpcWalletRecoverRequest{
			Mnemonic: mnemonic,
			RootPath: rootPath,
		})

		tok := call(t, ctx, c.WalletCreateSession, &pb.RpcWalletCreateSessionRequest{
			Mnemonic: mnemonic,
		}).Token

		ctx = metadata.AppendToOutgoingContext(ctx, "token", tok)

		stream, err := c.ListenSessionEvents(ctx, &pb.StreamRequest{Token: tok})
		require.NoError(t, err)

		er = startEventReceiver(ctx, stream)

		call(t, ctx, c.AccountRecover, &pb.RpcAccountRecoverRequest{})
		var id string
		waitEvent(er, func(a *pb.EventMessageValueOfAccountShow) {
			id = a.AccountShow.Account.Id
		})
		call(t, ctx, c.AccountSelect, &pb.RpcAccountSelectRequest{
			Id: id,
		})
	})

	{
		resp := call(t, ctx, c.ObjectSearch, &pb.RpcObjectSearchRequest{
			Keys: []string{"id", "type", "name"},
		})
		require.NotEmpty(t, resp.Records)
	}

	call(t, ctx, c.ObjectSearchSubscribe, &pb.RpcObjectSearchSubscribeRequest{
		SubId: "recent",
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLastOpenedDate.String(),
				Condition:   model.BlockContentDataviewFilter_Greater,
			},
		},
		Keys: []string{"id", "lastOpenedDate"},
	})

	objId := call(t, ctx, c.BlockLinkCreateWithObject, &pb.RpcBlockLinkCreateWithObjectRequest{
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
		resp := call(t, ctx, c.ObjectOpen, &pb.RpcObjectOpenRequest{
			ObjectId: objId,
		})
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
