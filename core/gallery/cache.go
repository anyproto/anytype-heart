package gallery

import (
	"encoding/json"
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
	CacheCName = "gallery-index-cache"

	galleryDir    = "gallery"
	indexFileName = "index.pb"
	verFileName   = "ver"

	indexURI = "https://tools.gallery.any.coop/app-index.json"
)

type IndexCache interface {
	app.Component

	GetManifest(downloadLink string, timeoutInSeconds int) (info *model.ManifestInfo, err error)
	GetIndex(timeoutInSeconds int) (*pb.RpcGalleryDownloadIndexResponse, error)
}

type cache struct {
	indexURL string
	path     string

	getLocalVersion func(path string) (string, error)
	getLocalIndex   func(path string) (*pb.RpcGalleryDownloadIndexResponse, error)
	save            func(path string, index *pb.RpcGalleryDownloadIndexResponse, version string)
}

func NewCache() IndexCache {
	return &cache{}
}

func (c *cache) Init(a *app.App) error {
	c.path = filepath.Join(app.MustComponent[wallet.Wallet](a).RepoPath(), galleryDir)
	if err := os.Mkdir(c.path, 0777); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to init gallery index directory: %w", err)
	}
	c.indexURL = indexURI
	c.getLocalVersion = getLocalVersion
	c.getLocalIndex = getLocalIndex
	c.save = saveIndexAndVersion
	return nil
}

func (c *cache) Name() string {
	return CacheCName
}

func (c *cache) GetIndex(timeoutInSeconds int) (*pb.RpcGalleryDownloadIndexResponse, error) {
	localIndex, err := c.getLocalIndex(c.path)
	if err != nil {
		log.Warn("failed to read local index. Need to refetch index from remote", zap.Error(err))
	}

	version := ""
	if localIndex != nil {
		version, err = c.getLocalVersion(c.path)
		if err != nil {
			log.Warn("failed to read local version. Need to refetch version from remote", zap.Error(err))
		}
	}

	index, newVersion, err := downloadGalleryIndex(c.indexURL, timeoutInSeconds, version, false)
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

	go c.save(c.path, index, newVersion)

	return index, nil
}

func (c *cache) GetManifest(downloadLink string, timeoutInSeconds int) (info *model.ManifestInfo, err error) {
	localIndex, err := c.getLocalIndex(c.path)
	if err != nil {
		log.Warn("failed to read local index. Need to refetch index from remote", zap.Error(err))
	}

	version := ""
	if localIndex != nil {
		version, err = c.getLocalVersion(c.path)
		if err != nil {
			log.Warn("failed to read local version. Need to refetch version from remote", zap.Error(err))
		}
	}

	index, newVersion, err := downloadGalleryIndex(c.indexURL, timeoutInSeconds, version, true)
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

	go c.save(c.path, index, newVersion)

	manifest, err := getManifestByDownloadLink(index, downloadLink)
	if err != nil {
		return nil, err
	}
	return manifest, nil
}

func getLocalIndex(path string) (*pb.RpcGalleryDownloadIndexResponse, error) {
	indexPath := filepath.Join(path, indexFileName)
	rawData, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read local gallery index: %w", err)
	}

	index := &pb.RpcGalleryDownloadIndexResponse{}
	if err = proto.Unmarshal(rawData, index); err != nil {
		return nil, fmt.Errorf("failed to unmarshal local gallery index: %w", err)
	}
	return index, nil
}

func getLocalVersion(path string) (string, error) {
	verPath := filepath.Join(path, verFileName)
	rawData, err := os.ReadFile(verPath)
	if err != nil {
		return "", fmt.Errorf("failed to read local gallery index version: %w", err)
	}
	return string(rawData), nil
}

func saveIndexAndVersion(path string, index *pb.RpcGalleryDownloadIndexResponse, version string) {
	data, err := proto.Marshal(index)
	if err != nil {
		log.Error("failed to marshal local gallery index", zap.Error(err))
		return
	}

	indexPath := filepath.Join(path, indexFileName)
	if err = os.WriteFile(indexPath, data, 0777); err != nil {
		log.Error("failed to save local gallery index", zap.Error(err))
		return
	}

	verPath := filepath.Join(path, verFileName)
	if err = os.WriteFile(verPath, []byte(version), 0777); err != nil {
		log.Error("failed to save local gallery version", zap.Error(err))
	}
}

func downloadGalleryIndex(
	indexURL string,
	timeoutInSeconds int, // timeout to wait for HTTP response
	version string, // Etag of gallery index, that allows us to fetch index faster
	withManifestValidation bool, // a flag that indicates that every manifest should be validated
) (response *pb.RpcGalleryDownloadIndexResponse, newVersion string, err error) {
	raw, newVersion, err := getRawJson(indexURL, timeoutInSeconds, version)
	if err != nil {
		if errors.Is(err, ErrNotModified) {
			return nil, version, err
		}
		return nil, "", fmt.Errorf("%w: %w", ErrDownloadIndex, err)
	}

	response = &pb.RpcGalleryDownloadIndexResponse{}
	err = json.Unmarshal(raw, &response)
	if err != nil {
		return nil, "", fmt.Errorf("%w to get lists of categories and experiences from gallery index: %w", ErrUnmarshalJson, err)
	}

	if withManifestValidation {
		schemas := &schemaList{}
		err = json.Unmarshal(raw, schemas)
		if err != nil {
			return nil, "", fmt.Errorf("%w to get list of manifest schemas from gallery index: %w", ErrUnmarshalJson, err)
		}

		if len(schemas.Experiences) != len(response.Experiences) {
			return nil, "", fmt.Errorf("invalid number of manifests with schema. Expected: %d, Actual: %d", len(response.Experiences), len(schemas.Experiences))
		}

		for i, info := range response.Experiences {
			if err = validateManifest(schemas.Experiences[i].Schema, info); err != nil {
				return nil, "", fmt.Errorf("manifest validation error: %w", err)
			}
		}
	}

	return response, newVersion, nil
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

	info.Description = stripTags(info.Description)
	return nil
}
