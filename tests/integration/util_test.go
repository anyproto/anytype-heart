package integration

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/application"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/builtinobjects"
)

type testApplication struct {
	appService *application.Service
	account    *model.Account
	eventQueue *mb.MB[*pb.Event]
}

func (a *testApplication) personalSpaceId() string {
	return a.account.Info.AccountSpaceId
}

func (a *testApplication) waitEventMessage(t *testing.T, pred func(msg *pb.EventMessage) bool) {
	queueCond := a.eventQueue.NewCond().WithFilter(func(event *pb.Event) bool {
		for _, msg := range event.Messages {
			if pred(msg) {
				return true
			}
		}
		return false
	})

	queueCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := queueCond.WaitOne(queueCtx)
	require.NoError(t, err)
}

func createAccountAndStartApp(t *testing.T) *testApplication {
	repoDir := t.TempDir()

	ctx := context.Background()
	app := application.New()
	platform := "test"
	version := "1.0.0"
	app.SetClientVersion(platform, version)
	metrics.Service.SetPlatform(platform)
	metrics.Service.SetStartVersion(version)
	metrics.Service.InitWithKeys(metrics.DefaultAmplitudeKey, metrics.DefaultInHouseKey)

	mnemonic, err := app.WalletCreate(&pb.RpcWalletCreateRequest{
		RootPath: repoDir,
	})
	t.Log(mnemonic)

	eventQueue := mb.New[*pb.Event](0)
	sender := event.NewCallbackSender(func(event *pb.Event) {
		err := eventQueue.Add(ctx, event)
		if err != nil {
			log.Println("event queue error:", err)
		}
	})
	app.SetEventSender(sender)

	acc, err := app.AccountCreate(ctx, &pb.RpcAccountCreateRequest{
		Name:                    "test name",
		StorePath:               repoDir,
		DisableLocalNetworkSync: true,
		NetworkMode:             pb.RpcAccount_LocalOnly,
	})
	require.NoError(t, err)

	testApp := &testApplication{
		appService: app,
		account:    acc,
		eventQueue: eventQueue,
	}
	objCreator := getService[builtinobjects.BuiltinObjects](testApp)
	_, err = objCreator.CreateObjectsForUseCase(session.NewContext(), acc.Info.AccountSpaceId, pb.RpcObjectImportUseCaseRequest_GET_STARTED)
	require.NoError(t, err)

	t.Cleanup(func() {
		err := app.AccountStop(&pb.RpcAccountStopRequest{
			RemoveData: true,
		})
		require.NoError(t, err)
	})

	return testApp
}

func getService[T any](testApp *testApplication) T {
	a := testApp.appService.GetApp()
	return app.MustComponent[T](a)
}
