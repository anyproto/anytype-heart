package gallery

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	strip "github.com/grokify/html-strip-tags-go"
	"github.com/xeipuuv/gojsonschema"
	"go.uber.org/zap"
	"golang.org/x/net/html"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/uri"
)

const (
	CName = "gallery-service"

	defaultTimeout    = time.Second * 5
	indexUrl          = "https://tools.gallery.any.coop/app-index.json"
	ifNoneMatchHeader = "If-None-Match"
	eTagHeader        = "ETag"

	cacheGalleryDir = "cache/gallery"
	indexFileName   = "index.pb"
	eTagFileName    = "index.pb.etag"
)

var (
	log = logger.NewNamed(CName)

	ErrUnmarshalJson = fmt.Errorf("failed to unmarshall json")
	ErrDownloadIndex = fmt.Errorf("failed to download gallery index")
	ErrNotModified   = fmt.Errorf("resource is not modified")
)

type Service interface {
	app.Component
	GetManifest(url string) (*model.ManifestInfo, error)
	GetGalleryIndex() (*pb.RpcGalleryDownloadIndexResponse, error)
}

func New() Service {
	return &service{}
}

type service struct {
	indexPath, versionPath string
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) error {
	path := filepath.Join(app.MustComponent[wallet.Wallet](a).RootPath(), cacheGalleryDir)
	if err := os.MkdirAll(path, 0777); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to init gallery index directory: %w", err)
	}
	s.indexPath = filepath.Join(path, indexFileName)
	s.versionPath = filepath.Join(path, eTagFileName)
	return nil
}

// whitelist maps allowed hosts to regular expressions of URL paths
var whitelist = map[string]*regexp.Regexp{
	"localhost":                                   regexp.MustCompile(`.*`),
	"127.0.0.1":                                   regexp.MustCompile(`.*`),
	"raw.githubusercontent.com":                   regexp.MustCompile(`^/anyproto.*`),
	"github.com":                                  regexp.MustCompile(`^/anyproto.*`),
	"community.anytype.io":                        regexp.MustCompile(`.*`),
	"anytype.io":                                  regexp.MustCompile(`.*`),
	"gallery.any.coop":                            regexp.MustCompile(`.*`),
	"tools.gallery.any.coop":                      regexp.MustCompile(`.*`),
	"storage.gallery.any.coop":                    regexp.MustCompile(`.*`),
	"stage1-anytype-spark.anytype.io":             regexp.MustCompile(`.*`),
	"stage1-anytype-spark.storage.googleapis.com": regexp.MustCompile(`.*`),
}

func (s *service) GetManifest(url string) (info *model.ManifestInfo, err error) {
	return s.getManifest(url, true, true)
}

func (s *service) getManifest(url string, checkWhitelist, validateSchema bool) (info *model.ManifestInfo, err error) {
	if err = uri.ValidateURI(url); err != nil {
		return nil, fmt.Errorf("provided URL is not valid: %w", err)
	}
	if checkWhitelist && !IsInWhitelist(url) {
		return nil, fmt.Errorf("URL is not in whitelist")
	}
	raw, _, err := getRawJson(url, "", defaultTimeout)
	if err != nil {
		return nil, err
	}

	info = &model.ManifestInfo{}
	err = jsonpb.Unmarshal(bytes.NewReader(raw), info)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall json to get manifest: %w", err)
	}

	if validateSchema {
		if err = validateManifestSchema(info); err != nil {
			return nil, err
		}
	}

	for _, urlToCheck := range append(info.Screenshots, info.DownloadLink) {
		if !IsInWhitelist(urlToCheck) {
			return nil, fmt.Errorf("URL provided in manifest is not in whitelist")
		}
	}

	info.Description = stripTags(info.Description)
	return info, nil
}

func (s *service) GetGalleryIndex() (index *pb.RpcGalleryDownloadIndexResponse, err error) {
	return s.getGalleryIndex(indexUrl, defaultTimeout)
}

func (s *service) getGalleryIndex(indexURL string, timeout time.Duration) (index *pb.RpcGalleryDownloadIndexResponse, err error) {
	localIndex, err := s.readIndex()
	if err != nil {
		log.Warn("failed to read local index. Need to re-fetch index from remote", zap.Error(err))
	}

	var currentVersion string
	if localIndex != nil {
		currentVersion, err = s.readVersion()
		if err != nil {
			log.Warn("failed to read local version. Need to re-fetch version from remote", zap.Error(err))
		}
	}

	raw, newVersion, err := getRawJson(indexURL, currentVersion, timeout)
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

	index = &pb.RpcGalleryDownloadIndexResponse{}
	err = jsonpb.Unmarshal(bytes.NewReader(raw), index)
	if err != nil {
		if localIndex != nil {
			log.Warn("failed to parse remote index. Returning local index", zap.Error(err))
			return localIndex, nil
		}
		return nil, fmt.Errorf("%w to get lists of categories and experiences from gallery index: %w", ErrUnmarshalJson, err)
	}

	s.saveIndexAndVersion(index, newVersion)
	return index, nil
}

func (s *service) readIndex() (*pb.RpcGalleryDownloadIndexResponse, error) {
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

func (s *service) readVersion() (string, error) {
	rawData, err := os.ReadFile(s.versionPath)
	if err != nil {
		return "", fmt.Errorf("failed to read local gallery index version: %w", err)
	}
	return string(rawData), nil
}

func (s *service) saveIndexAndVersion(index *pb.RpcGalleryDownloadIndexResponse, version string) {
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

func IsInWhitelist(url string) bool {
	if len(whitelist) == 0 {
		return true
	}
	parsedURL, err := uri.ParseURI(url)
	if err != nil {
		return false
	}
	for host, pathRegexp := range whitelist {
		if strings.Contains(parsedURL.Host, host) {
			return pathRegexp.MatchString(parsedURL.Path)
		}
	}
	return false
}

func getRawJson(url string, currentVersion string, timeout time.Duration) (body []byte, newVersion string, err error) {
	client := http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}

	if currentVersion != "" {
		req.Header.Add(ifNoneMatchHeader, currentVersion)
	}
	req.Close = true
	res, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}

	if res.StatusCode != http.StatusOK {
		if res.StatusCode == http.StatusNotModified {
			return nil, currentVersion, ErrNotModified
		}
		return nil, "", fmt.Errorf("failed to get json file. Status: %s", res.Status)
	}

	newVersion = res.Header.Get(eTagHeader)

	if res.Body != nil {
		defer res.Body.Close()
	}

	body, err = io.ReadAll(res.Body)
	if err != nil {
		return nil, "", err
	}
	return body, newVersion, nil
}

func validateManifestSchema(info *model.ManifestInfo) (err error) {
	if info.Schema == "" {
		return
	}
	var result *gojsonschema.Result
	schemaLoader := gojsonschema.NewReferenceLoader(info.Schema)
	jsonLoader := gojsonschema.NewGoLoader(info)
	result, err = gojsonschema.Validate(schemaLoader, jsonLoader)
	if err != nil {
		return err
	}
	if !result.Valid() {
		return buildResultError(result)
	}
	return nil
}

func stripTags(str string) string {
	if _, err := html.Parse(strings.NewReader(str)); err != nil {
		return str
	}
	return strip.StripTags(str)
}

func buildResultError(result *gojsonschema.Result) error {
	var description strings.Builder
	n := len(result.Errors()) - 1
	for i, e := range result.Errors() {
		description.WriteString(e.Context().String())
		description.WriteString(" - ")
		description.WriteString(e.Description())
		if i < n {
			description.WriteString("; ")
		}
	}
	return fmt.Errorf("manifest does not correspond provided schema: %s", description.String())
}
