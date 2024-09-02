package gallery

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	strip "github.com/grokify/html-strip-tags-go"
	"github.com/xeipuuv/gojsonschema"
	"go.uber.org/zap"
	"golang.org/x/net/html"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/uri"
)

const (
	defaultTimeout = time.Second * 30

	versionHeader = "If-None-Match"

	indexURI = "https://tools.gallery.any.coop/app-index.json"
)

type schemaResponse struct {
	Schema string `json:"$schema"`
}

type schemaList struct {
	Experiences []schemaResponse `json:"experiences"`
}

var (
	ErrUnmarshalJson = fmt.Errorf("failed to unmarshall json")
	ErrDownloadIndex = fmt.Errorf("failed to download gallery index")
	ErrNotModified   = fmt.Errorf("resource is not modified")
)

// whitelist maps allowed hosts to regular expressions of URL paths
var whitelist = map[string]*regexp.Regexp{
	"raw.githubusercontent.com": regexp.MustCompile(`^/anyproto.*`),
	"github.com":                regexp.MustCompile(`^/anyproto.*`),
	"community.anytype.io":      regexp.MustCompile(`.*`),
	"anytype.io":                regexp.MustCompile(`.*`),
	"gallery.any.coop":          regexp.MustCompile(`.*`),
	"tools.gallery.any.coop":    regexp.MustCompile(`.*`),
	"storage.gallery.any.coop":  regexp.MustCompile(`.*`),
}

// GetGalleryIndex first tries to get gallery index from different places in following order:
// 1. Middleware index cache
// 2. Remote index (with HTTP request timeout = 1 second)
// 3. Client cache, path to which is passed by argument
// 4. Remote index (with HTTP request timeout more than one second)
func (s *service) GetGalleryIndex(clientCachePath string) (index *pb.RpcGalleryDownloadIndexResponse, err error) {
	index, err = s.indexCache.GetIndex(1)
	if err == nil {
		return index, nil
	}

	log.Warn("failed to get gallery index. Getting it from client cache", zap.Error(err))
	_, index, err = readClientCache(clientCachePath, "")
	if err == nil {
		return index, nil
	}

	log.Warn("failed to get gallery index from client cache. Getting it from mw cache one more time", zap.Error(err))
	index, err = s.indexCache.GetIndex(0)
	if err != nil {
		return nil, err
	}
	return index, nil
}

func DownloadManifest(url string, checkWhitelist bool) (info *model.ManifestInfo, err error) {
	if err = uri.ValidateURI(url); err != nil {
		return nil, fmt.Errorf("provided URL is not valid: %w", err)
	}
	if checkWhitelist && !IsInWhitelist(url) {
		return nil, fmt.Errorf("URL '%s' is not in whitelist", url)
	}
	raw, _, err := getRawJson(url, 0, "")
	if err != nil {
		return nil, err
	}

	schemaResp := schemaResponse{}
	err = json.Unmarshal(raw, &schemaResp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall json to get schema: %w", err)
	}

	err = json.Unmarshal(raw, &info)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall json to get manifest: %w", err)
	}

	if err = validateSchema(schemaResp.Schema, info); err != nil {
		return nil, err
	}

	for _, urlToCheck := range append(info.Screenshots, info.DownloadLink) {
		if !IsInWhitelist(urlToCheck) {
			return nil, fmt.Errorf("URL '%s' provided in manifest is not in whitelist", urlToCheck)
		}
	}

	info.Description = stripTags(info.Description)
	return info, nil
}

// downloadGalleryIndex accepts
// timeoutInSeconds - timeout to wait for HTTP response
// version - eTag of gallery index, that allows us to fetch index faster
// withManifestValidation - a flag that indicates that every manifest should be validated
func downloadGalleryIndex(timeoutInSeconds int, version string, withManifestValidation bool) (response *pb.RpcGalleryDownloadIndexResponse, newVersion string, err error) {
	raw, newVersion, err := getRawJson(indexURI, timeoutInSeconds, version)
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

func getRawJson(url string, timeoutInSeconds int, currentVersion string) (body []byte, newVersion string, err error) {
	timeout := defaultTimeout
	if timeoutInSeconds != 0 {
		timeout = time.Duration(timeoutInSeconds) * time.Second
	}
	client := http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}

	if currentVersion != "" {
		req.Header.Add(versionHeader, currentVersion)
	}

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

	newVersion = res.Header.Get(versionHeader)

	if res.Body != nil {
		defer res.Body.Close()
	}

	body, err = io.ReadAll(res.Body)
	if err != nil {
		return nil, "", err
	}
	return body, newVersion, nil
}

func validateManifest(schema string, info *model.ManifestInfo) error {
	if err := validateSchema(schema, info); err != nil {
		return fmt.Errorf("manifest does not correspond scema: %w", err)
	}

	for _, urlToCheck := range append(info.Screenshots, info.DownloadLink) {
		if !IsInWhitelist(urlToCheck) {
			return fmt.Errorf("URL '%s' provided in manifest is not in whitelist", urlToCheck)
		}
	}

	info.Description = stripTags(info.Description)
	return nil
}

func validateSchema(schema string, info *model.ManifestInfo) (err error) {
	if schema == "" {
		return
	}
	var result *gojsonschema.Result
	schemaLoader := gojsonschema.NewReferenceLoader(schema)
	jsonLoader := gojsonschema.NewGoLoader(info)
	result, err = gojsonschema.Validate(schemaLoader, jsonLoader)
	if err != nil {
		return err
	}
	if !result.Valid() {
		return buildResultError(result)
	}
	info.Schema = schema
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
