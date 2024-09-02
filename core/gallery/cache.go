package gallery

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	CacheCName    = "gallery-index-cache"
	galleryDir    = "gallery"
	indexFileName = "index.pb"
	verFileName   = "ver"
)

type IndexCache interface {
	app.Component

	GetManifest(downloadLink string, timeoutInSeconds int) (info *model.ManifestInfo, err error)
	GetIndex(timeoutInSeconds int) (*pb.RpcGalleryDownloadIndexResponse, error)
}

type cache struct {
	indexPath string
	verPath   string
}

func NewCache() IndexCache {
	return &cache{}
}

func (c *cache) Init(a *app.App) error {
	dir := filepath.Join(app.MustComponent[wallet.Wallet](a).RepoPath(), galleryDir)
	if err := os.Mkdir(dir, 0777); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to init gallery index directory: %w", err)
	}
	c.indexPath = filepath.Join(dir, indexFileName)
	c.verPath = filepath.Join(dir, verFileName)
	return nil
}

func (c *cache) Name() string {
	return CacheCName
}

func (c *cache) GetIndex(timeoutInSeconds int) (*pb.RpcGalleryDownloadIndexResponse, error) {
	localIndex, err := c.getLocalIndex()
	if err != nil {
		log.Warn("failed to read local index. Need to refetch index from remote", zap.Error(err))
	}

	version := ""
	if localIndex != nil {
		version, err = c.getLocalVersion()
		if err != nil {
			log.Warn("failed to read local version. Need to refetch version from remote", zap.Error(err))
		}
	}

	index, newVersion, err := downloadGalleryIndex(timeoutInSeconds, version, false)
	if err != nil {
		if errors.Is(err, ErrNotModified) {
			return localIndex, nil
		}

		if localIndex != nil {
			log.Warn("failed to download index from remote. Returning local index", zap.Error(err))
			return localIndex, nil
		}

		return nil, err
	}

	go c.saveIndexAndVersion(index, newVersion)

	return index, nil
}

func (c *cache) GetManifest(downloadLink string, timeoutInSeconds int) (info *model.ManifestInfo, err error) {
	localIndex, err := c.getLocalIndex()
	if err != nil {
		log.Warn("failed to read local index. Need to refetch index from remote", zap.Error(err))
	}

	version := ""
	if localIndex != nil {
		version, err = c.getLocalVersion()
		if err != nil {
			log.Warn("failed to read local version. Need to refetch version from remote", zap.Error(err))
		}
	}

	index, newVersion, err := downloadGalleryIndex(timeoutInSeconds, version, true)
	if err != nil {
		if errors.Is(err, ErrNotModified) {
			manifest, err := getManifestByDownloadLink(localIndex, downloadLink)
			if err != nil {
				return nil, err
			}
			return manifest, nil
		}

		if localIndex != nil {
			log.Warn("failed to download index from remote. Returning local index", zap.Error(err))
			manifest, err := getManifestByDownloadLink(localIndex, downloadLink)
			if err != nil {
				return nil, err
			}
			return manifest, nil
		}

		return nil, err
	}

	go c.saveIndexAndVersion(index, newVersion)

	manifest, err := getManifestByDownloadLink(index, downloadLink)
	if err != nil {
		return nil, err
	}
	return manifest, nil
}

func (c *cache) getLocalIndex() (*pb.RpcGalleryDownloadIndexResponse, error) {
	rawData, err := os.ReadFile(c.indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read local gallery index: %w", err)
	}

	index := &pb.RpcGalleryDownloadIndexResponse{}
	if err = proto.Unmarshal(rawData, index); err != nil {
		return nil, fmt.Errorf("failed to unmarshal local gallery index: %w", err)
	}
	return index, nil
}

func (c *cache) getLocalVersion() (string, error) {
	rawData, err := os.ReadFile(c.verPath)
	if err != nil {
		return "", fmt.Errorf("failed to read local gallery index version: %w", err)
	}
	return string(rawData), nil
}

func (c *cache) saveIndexAndVersion(index *pb.RpcGalleryDownloadIndexResponse, version string) {
	data, err := proto.Marshal(index)
	if err != nil {
		log.Error("failed to marshal local gallery index", zap.Error(err))
		return
	}

	if err = os.WriteFile(c.indexPath, data, 0777); err != nil {
		log.Error("failed to save local gallery index", zap.Error(err))
		return
	}

	if err = os.WriteFile(c.verPath, []byte(version), 0777); err != nil {
		log.Error("failed to save local gallery version", zap.Error(err))
	}
}
