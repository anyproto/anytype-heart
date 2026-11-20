package gallery

import (
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "gallery-service"

var (
	ErrUnmarshalJson = fmt.Errorf("failed to unmarshall json")
	ErrDownloadIndex = fmt.Errorf("failed to download gallery index")
)

// Service is the offline no-op implementation of the gallery service.
type Service interface {
	app.Component
	GetManifest(url string) (*model.ManifestInfo, error)
	GetGalleryIndex() (*pb.RpcGalleryDownloadIndexResponse, error)
}

type service struct{}

func New() Service {
	return &service{}
}

func (s *service) Name() string { return CName }

func (s *service) Init(a *app.App) error { return nil }

func (s *service) GetManifest(url string) (*model.ManifestInfo, error) {
	return nil, nil
}

func (s *service) GetGalleryIndex() (*pb.RpcGalleryDownloadIndexResponse, error) {
	return &pb.RpcGalleryDownloadIndexResponse{}, nil
}

// IsInWhitelist always returns false in offline builds.
func IsInWhitelist(url string) bool { return false }
