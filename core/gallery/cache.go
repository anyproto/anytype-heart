package gallery

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/jsonpb"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	CacheCName = "gallery-index-cache"

	cacheGalleryDir = "cache/gallery"
	indexFileName   = "index.pb"
	eTagFileName    = "index.pb.etag"

	indexURI = "https://tools.gallery.any.coop/app-index.json"
)

type IndexCache interface {
	app.Component

	GetManifest(name string, timeoutInSeconds int) (info *model.ManifestInfo, err error)
	GetIndex(timeoutInSeconds int) (*pb.RpcGalleryDownloadIndexResponse, error)
}

type cache struct {
	storage  cacheStorage
	indexURL string
}

func NewCache() IndexCache {
	return &cache{}
}

func (c *cache) Init(a *app.App) error {
	path := filepath.Join(app.MustComponent[wallet.Wallet](a).RootPath(), cacheGalleryDir)
	if err := os.MkdirAll(path, 0777); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to init gallery index directory: %w", err)
	}

	c.storage = &storage{
		versionPath: filepath.Join(path, eTagFileName),
		indexPath:   filepath.Join(path, indexFileName),
	}

	c.indexURL = indexURI
	return nil
}

func (c *cache) Name() string {
	return CacheCName
}

func (c *cache) GetIndex(timeoutInSeconds int) (*pb.RpcGalleryDownloadIndexResponse, error) {
	return c.getRecentIndex(timeoutInSeconds, false)
}

func (c *cache) GetManifest(name string, timeoutInSeconds int) (info *model.ManifestInfo, err error) {
	index, err := c.getRecentIndex(timeoutInSeconds, true)
	if err != nil {
		return nil, err
	}
	return getManifestByName(index, name)
}

func (c *cache) getRecentIndex(timeout int, withManifestValidation bool) (*pb.RpcGalleryDownloadIndexResponse, error) {
	localIndex, err := c.storage.getIndex()
	if err != nil {
		log.Warn("failed to read local index. Need to refetch index from remote", zap.Error(err))
	}

	version := ""
	if localIndex != nil {
		version, err = c.storage.getVersion()
		if err != nil {
			log.Warn("failed to read local version. Need to refetch version from remote", zap.Error(err))
		}
	}

	index, newVersion, err := c.downloadGalleryIndex(timeout, version, withManifestValidation)
	if err == nil {
		c.storage.save(index, newVersion)
		return index, nil
	}

	if errors.Is(err, ErrNotModified) {
		return localIndex, nil
	}

	if localIndex != nil {
		log.Warn("failed to download index from remote. Returning local index", zap.Error(err))
		return localIndex, nil
	}
	return nil, err
}

func (c *cache) downloadGalleryIndex(
	timeoutInSeconds int, // timeout to wait for HTTP response
	version string, // Etag of gallery index, that allows to fetch index faster
	withManifestValidation bool, // a flag that indicates that every manifest should be validated
) (response *pb.RpcGalleryDownloadIndexResponse, newVersion string, err error) {
	raw, newVersion, err := getRawJson(c.indexURL, timeoutInSeconds, version)
	if err != nil {
		if errors.Is(err, ErrNotModified) {
			return nil, version, err
		}
		return nil, "", fmt.Errorf("%w: %w", ErrDownloadIndex, err)
	}

	response = &pb.RpcGalleryDownloadIndexResponse{}
	err = jsonpb.Unmarshal(bytes.NewReader(raw), response)
	if err != nil {
		return nil, "", fmt.Errorf("%w to get lists of categories and experiences from gallery index: %w", ErrUnmarshalJson, err)
	}

	if withManifestValidation {
		for _, info := range response.Experiences {
			stripTags(info)
			if err = validateManifest(info.Schema, info); err != nil {
				return nil, "", fmt.Errorf("manifest validation error: %w", err)
			}
		}
	}

	return response, newVersion, nil
}

type cacheStorage interface {
	getIndex() (*pb.RpcGalleryDownloadIndexResponse, error)
	getVersion() (string, error)
	save(index *pb.RpcGalleryDownloadIndexResponse, version string)
}

type storage struct {
	versionPath, indexPath string
}

func (s *storage) getIndex() (*pb.RpcGalleryDownloadIndexResponse, error) {
	rawData, err := os.ReadFile(s.indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read local gallery index: %w", err)
	}

	index := &pb.RpcGalleryDownloadIndexResponse{}
	if err = proto.Unmarshal(rawData, index); err != nil {
		return nil, fmt.Errorf("failed to unmarshal local gallery index: %w", err)
	}
	return index, nil
}

func (s *storage) getVersion() (string, error) {
	rawData, err := os.ReadFile(s.versionPath)
	if err != nil {
		return "", fmt.Errorf("failed to read local gallery index version: %w", err)
	}
	return string(rawData), nil
}

// TODO: Maybe we should save info to files async?
func (s *storage) save(index *pb.RpcGalleryDownloadIndexResponse, version string) {
	data, err := proto.Marshal(index)
	if err != nil {
		log.Error("failed to marshal local gallery index", zap.Error(err))
		return
	}

	if err = os.WriteFile(s.indexPath, data, 0600); err != nil {
		log.Error("failed to save local gallery index", zap.Error(err))
		return
	}

	if err = os.WriteFile(s.versionPath, []byte(version), 0600); err != nil {
		log.Error("failed to save local gallery version", zap.Error(err))
	}
}

func validateManifest(schema string, info *model.ManifestInfo) error {
	if err := validateSchema(schema, info); err != nil {
		return fmt.Errorf("manifest does not correspond scema: %w", err)
	}

	for _, urlToCheck := range append(info.Screenshots, info.DownloadLink) {
		if !isInWhitelist(urlToCheck) {
			return fmt.Errorf("URL '%s' provided in manifest is not in whitelist", urlToCheck)
		}
	}
	return nil
}
