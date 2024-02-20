package integration

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/application"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/util/builtinobjects"
)

func TestSimple(t *testing.T) {
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

	sender := event.NewCallbackSender(func(event *pb.Event) {
		t.Log(event)
	})
	app.SetEventSender(sender)

	acc, err := app.AccountCreate(ctx, &pb.RpcAccountCreateRequest{
		Name:                    "test name",
		StorePath:               repoDir,
		DisableLocalNetworkSync: true,
		NetworkMode:             pb.RpcAccount_LocalOnly,
	})
	require.NoError(t, err)

	objCreator := getService[builtinobjects.BuiltinObjects](app)
	_, err = objCreator.CreateObjectsForUseCase(session.NewContext(), acc.Info.AccountSpaceId, pb.RpcObjectImportUseCaseRequest_GET_STARTED)
	require.NoError(t, err)

	objectStore := getService[objectstore.ObjectStore](app)
	recs, _, err := objectStore.Query(database.Query{})
	assert.NotEmpty(t, recs)
	t.Log(acc)
}

func getService[T any](appService *application.Service) T {
	a := appService.GetApp()
	return app.MustComponent[T](a)
}
