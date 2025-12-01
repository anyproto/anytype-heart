package integration

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/application"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/builtinobjects"
)

type testApplication struct {
	appService *application.Service
	account    *model.Account
	eventQueue *mb.MB[*pb.EventMessage]
}

func (a *testApplication) personalSpaceId() string {
	return a.account.Info.AccountSpaceId
}

func (a *testApplication) waitEventMessage(t *testing.T, pred func(msg *pb.EventMessage) bool) {
	queueCond := a.eventQueue.NewCond().WithFilter(func(msg *pb.EventMessage) bool {
		return pred(msg)
	})

	queueCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := queueCond.WaitOne(queueCtx)
	require.NoError(t, err)
}

func createAccountAndStartApp(t *testing.T, defaultUsecase pb.RpcObjectImportUseCaseRequestUseCase) *testApplication {
	repoDir := t.TempDir()

	ctx := context.Background()
	app := application.New()
	platform := "test"
	version := "1.0.0"
	app.SetClientVersion(platform, version)
	metrics.Service.SetPlatform(platform)
	metrics.Service.SetStartVersion(version)
	metrics.Service.InitWithKeys(metrics.DefaultInHouseKey)

	mnemonic, _, err := app.WalletCreate(&pb.RpcWalletCreateRequest{
		RootPath: repoDir,
	})
	t.Log(mnemonic)

	eventQueue := mb.New[*pb.EventMessage](0)
	sender := event.NewCallbackSender(func(event *pb.Event) {
		for _, msg := range event.Messages {
			err := eventQueue.Add(ctx, msg)
			if err != nil {
				log.Println("event queue error:", err)
			}
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
	_, _, err = objCreator.CreateObjectsForUseCase(session.NewContext(), acc.Info.AccountSpaceId, defaultUsecase)
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

type testSubscription struct {
	subscriptionId string
}

func newTestSubscription(t *testing.T, app *testApplication, keys []domain.RelationKey, filters []database.FilterRequest) *testSubscription {
	keysConverted := make([]string, 0, len(keys))
	for _, key := range keys {
		keysConverted = append(keysConverted, key.String())
	}
	subscriptionId := bson.NewObjectId().Hex()
	subscriptionService := getService[subscription.Service](app)
	_, err := subscriptionService.Search(subscription.SubscribeRequest{
		SpaceId: app.account.Info.AccountSpaceId,
		SubId:   subscriptionId,
		Keys:    keysConverted,
		Filters: filters,
	})
	require.NoError(t, err)
	return &testSubscription{
		subscriptionId: subscriptionId,
	}
}

func (s *testSubscription) waitOneObjectDetailsSet(t *testing.T, app *testApplication, assertion func(t *testing.T, msg *pb.EventObjectDetailsSet)) {
	app.waitEventMessage(t, func(msg *pb.EventMessage) bool {
		if v := msg.GetObjectDetailsSet(); v != nil {
			if slices.Contains(v.SubIds, s.subscriptionId) {
				assertion(t, v)
				return true
			}
		}
		return false
	})
}

func (s *testSubscription) waitObjectDetailsSetWithPredicate(t *testing.T, app *testApplication, assertion func(t *testing.T, msg *pb.EventObjectDetailsSet) bool) {
	app.waitEventMessage(t, func(msg *pb.EventMessage) bool {
		if v := msg.GetObjectDetailsSet(); v != nil {
			if slices.Contains(v.SubIds, s.subscriptionId) {
				return assertion(t, v)
			}
		}
		return false
	})
}
