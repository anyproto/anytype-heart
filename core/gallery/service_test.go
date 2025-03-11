package gallery

import (
	"context"
	"os"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/import/mock_importer"

	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/notifications/mock_notifications"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core/mock_core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const spaceId = "spaceId"

type dumbProgress struct {
	app.ComponentRunnable
}

func (dp *dumbProgress) Add(process.Process) error {
	return nil
}

func (dp *dumbProgress) Cancel(string) error {
	return nil
}

func (dp *dumbProgress) NewQueue(pb.ModelProcess, int, bool, process.NotificationService) process.Queue {
	return nil
}

func (dp *dumbProgress) Subscribe(token string) {}

func (dp *dumbProgress) Unsubscribe(token string) {}

type spaceNameGetter struct{}

func (sng *spaceNameGetter) GetSpaceName(string) string {
	return spaceId
}

type fixture struct {
	Service
	importer       *mock_importer.MockImporter
	tempDirService *mock_core.MockTempDirProvider
	progress       dumbProgress
	notifService   *mock_notifications.MockNotifications
	indexCache     *cache
}

func newFixture(t *testing.T) *fixture {
	importer := mock_importer.NewMockImporter(t)
	tempDirService := mock_core.NewMockTempDirProvider(t)
	notifService := mock_notifications.NewMockNotifications(t)
	indexCache := &cache{}

	notifService.EXPECT().CreateAndSend(mock.Anything).Maybe().Return(nil)

	s := &service{
		importer:          importer,
		spaceNameGetter:   &spaceNameGetter{},
		tempDirService:    tempDirService,
		progress:          &dumbProgress{},
		notifications:     notifService,
		indexCache:        indexCache,
		withUrlValidation: false,
	}

	return &fixture{
		Service:        s,
		importer:       importer,
		tempDirService: tempDirService,
		notifService:   notifService,
		indexCache:     indexCache,
	}
}

func TestService_ImportExperience(t *testing.T) {
	t.Run("import experience by local path", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.importer.EXPECT().Import(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, req *importer.ImportRequest) *importer.ImportResponse {
			assert.Equal(t, model.ObjectOrigin_usecase, req.Origin.Origin)
			assert.Equal(t, model.Import_Pb, req.Origin.ImportType)
			assert.Equal(t, spaceId, req.SpaceId)
			assert.False(t, req.IsMigration)
			assert.False(t, req.NoProgress)
			return &importer.ImportResponse{}
		})

		// when
		err := fx.ImportExperience(nil, spaceId, UseCaseInfo{
			Title:        "Empty",
			DownloadLink: "./testdata/get_started.zip",
		}, true)

		// then
		assert.NoError(t, err)
	})

	t.Run("import experience from remote", func(t *testing.T) {
		// given
		server := buildServer(t, "hash1")
		defer server.Close()

		var (
			path string
			url  = "http://127.0.0.1:" + port + "/get_started.zip"
		)

		fx := newFixture(t)
		fx.importer.EXPECT().Import(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, req *importer.ImportRequest) *importer.ImportResponse {
			path = req.Params.(*pb.RpcObjectImportRequestParamsOfPbParams).PbParams.Path[0]
			assert.NotEmpty(t, path)
			assert.Equal(t, model.ObjectOrigin_usecase, req.Origin.Origin)
			assert.Equal(t, model.Import_Pb, req.Origin.ImportType)
			assert.Equal(t, spaceId, req.SpaceId)
			assert.False(t, req.IsMigration)
			assert.False(t, req.NoProgress)
			return &importer.ImportResponse{}
		})

		fx.indexCache.storage = &testCacheStorage{}

		fx.tempDirService.EXPECT().TempDir().Return("./testdata")

		// when
		err := fx.ImportExperience(nil, spaceId, UseCaseInfo{
			Title:        "Get Started",
			DownloadLink: url,
		}, true)

		// then
		assert.NoError(t, err)
		_, err = os.Stat(path)
		assert.Error(t, err)
	})

	t.Run("failed to import experience from all places", func(t *testing.T) {
		// given
		var (
			url = "http://127.0.0.1:" + port + "/get_started.zip"
		)

		fx := newFixture(t)
		fx.indexCache.storage = &testCacheStorage{}

		// when
		err := fx.ImportExperience(nil, spaceId, UseCaseInfo{
			Title:        "Get Started",
			DownloadLink: url,
		}, true)

		// then
		assert.Error(t, err)
	})
}
