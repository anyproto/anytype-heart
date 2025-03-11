package gallery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/jsonpb"
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
	eTagHeader    = "ETag"
)

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

// GetGalleryIndex tries to get gallery index from different places in following order:
// 1. Middleware index cache
// 2. Remote index (with HTTP request timeout = 1 second)
// 3. Client cache, path to which is passed by argument
// 4. Remote index (with default HTTP request timeout)
func (s *service) GetGalleryIndex(clientCachePath string) (index *pb.RpcGalleryDownloadIndexResponse, err error) {
	index, err = s.indexCache.GetIndex(1)
	if err == nil {
		return index, nil
	}

	log.Warn("failed to get gallery index. Getting it from client cache", zap.Error(err))

	// TODO: GO-4131 Maybe we should not return index from client cache, as it could be reduced (need to be discussed)
	_, index, err = readArtifact(clientCachePath, true)
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

func (s *service) GetManifest(url string, checkWhitelist bool) (info *model.ManifestInfo, err error) {
	if err = uri.ValidateURI(url); err != nil {
		return nil, fmt.Errorf("provided URL is not valid: %w", err)
	}
	if checkWhitelist && !isInWhitelist(url) {
		return nil, fmt.Errorf("URL '%s' is not in whitelist", url)
	}
	raw, _, err := getRawJson(url, 0, "")
	if err != nil {
		return nil, err
	}

	info = &model.ManifestInfo{}
	err = jsonpb.Unmarshal(bytes.NewReader(raw), info)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall json to get schema: %w", err)
	}

	err = json.Unmarshal(raw, &info)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall json to get manifest: %w", err)
	}

	if err = validateSchema(info.Schema, info); err != nil {
		return nil, err
	}

	for _, urlToCheck := range append(info.Screenshots, info.DownloadLink) {
		if !isInWhitelist(urlToCheck) {
			return nil, fmt.Errorf("URL '%s' provided in manifest is not in whitelist", urlToCheck)
		}
	}

	stripTags(info)
	return info, nil
}

func isInWhitelist(url string) bool {
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
	return nil
}

func stripTags(info *model.ManifestInfo) {
	description := info.Description
	if _, err := html.Parse(strings.NewReader(description)); err != nil {
		return
	}
	info.Description = strip.StripTags(description)
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
