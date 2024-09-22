package gallery

import (
	"context"
	"errors"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/block/import/mock_importer"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/notifications/mock_notifications"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core/mock_core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	spaceId   = "spaceId"
	cachePath = "./testdata/client_cache.zip"
)

type dumbProgress struct {
	app.ComponentRunnable
}

func (dp *dumbProgress) Add(process.Process) error {
	return nil
}

func (dp *dumbProgress) Cancel(string) error {
	return nil
}

func (dp *dumbProgress) NewQueue(pb.ModelProcess, int) process.Queue {
	return nil
}

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
	indexCache := &cache{save: func(path string, index *pb.RpcGalleryDownloadIndexResponse, version string) {}}

	notifService.EXPECT().CreateAndSend(mock.Anything).Maybe().Return(nil)

	s := &service{
		importer:        importer,
		spaceNameGetter: &spaceNameGetter{},
		tempDirService:  tempDirService,
		progress:        &dumbProgress{},
		notifications:   notifService,
		indexCache:      indexCache,
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
	t.Run("import local experience, no client cache", func(t *testing.T) {
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
		err := fx.ImportExperience(nil, spaceId, "./testdata/empty_experience.zip", "empty", "", true)

		// then
		assert.NoError(t, err)
	})

	// t.Run("import remote experience, with client cache", func(t *testing.T) {
	// 	// given
	// 	fx := newFixture(t)
	// 	fx.importer.EXPECT().Import(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, req *importer.ImportRequest) *importer.ImportResponse {
	// 		assert.Equal(t, model.ObjectOrigin_usecase, req.Origin.Origin)
	// 		assert.Equal(t, model.Import_Pb, req.Origin.ImportType)
	// 		assert.Equal(t, spaceId, req.SpaceId)
	// 		assert.False(t, req.IsMigration)
	// 		assert.False(t, req.NoProgress)
	// 		return &importer.ImportResponse{}
	// 	})
	//
	// 	fx.indexCache.getLocalIndex = func(string) (*pb.RpcGalleryDownloadIndexResponse, error) {
	// 		return &pb.RpcGalleryDownloadIndexResponse{}, nil
	// 	}
	//
	// 	// when
	// 	url := "https://github.com/anyproto/gallery/raw/main/experiences/get_started/get_started.zip"
	// 	err := fx.ImportExperience(nil, spaceId, url, "Get Started", cachePath, true)
	//
	// 	// then
	// 	assert.NoError(t, err)
	// })
}

func TestService_GetGalleryIndex(t *testing.T) {
	server := startHttpServer()
	defer server.Shutdown(nil)

	t.Run("get gallery index from middleware cache", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.indexCache.getLocalIndex = func(string) (*pb.RpcGalleryDownloadIndexResponse, error) {
			return buildIndex(), nil
		}
		fx.indexCache.getLocalVersion = func(string) (string, error) {
			return "v1", nil
		}

		// when
		index, err := fx.GetGalleryIndex("")

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Experiences, 1)
		assert.Equal(t, "name", index.Experiences[0].Name)
	})

	t.Run("get gallery index from client cache", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.indexCache.getLocalIndex = func(string) (*pb.RpcGalleryDownloadIndexResponse, error) {
			return nil, errors.New("failed to get local index")
		}

		// when
		index, err := fx.GetGalleryIndex("./testdata/client_cache.zip")

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Experiences, 1)
		assert.Equal(t, "get_started", index.Experiences[0].Name)
	})

	t.Run("get gallery index from remote", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.indexCache.indexURL = "http://localhost" + port + "/index.json"
		fx.indexCache.getLocalIndex = func(string) (*pb.RpcGalleryDownloadIndexResponse, error) {
			return nil, errors.New("failed to get local index")
		}

		// when
		index, err := fx.GetGalleryIndex("./testdata/client_cache.zip")

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Experiences, 1)
		assert.Equal(t, "name", index.Experiences[0].Name)
	})

	t.Run("failed to get index from all places", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.indexCache.getLocalIndex = func(string) (*pb.RpcGalleryDownloadIndexResponse, error) {
			return nil, errors.New("failed to get local index")
		}

		// when
		_, err := fx.GetGalleryIndex("invalid_path")

		// then
		assert.Error(t, err)
	})
}
