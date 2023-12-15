package gallery

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	strip "github.com/grokify/html-strip-tags-go"
	"github.com/xeipuuv/gojsonschema"
	"golang.org/x/net/html"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/uri"
)

const timeout = time.Second * 30

type schemaResponse struct {
	Schema string `json:"$schema"`
}

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

func DownloadManifest(url string, checkWhitelist bool) (info *pb.RpcDownloadManifestResponseManifestInfo, err error) {
	if err = uri.ValidateURI(url); err != nil {
		return nil, fmt.Errorf("provided URL is not valid: %w", err)
	}
	if checkWhitelist && !IsInWhitelist(url) {
		return nil, fmt.Errorf("URL '%s' is not in whitelist", url)
	}
	raw, err := getRawManifest(url)
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

	if err = validateSchema(schemaResp, info); err != nil {
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

func getRawManifest(url string) ([]byte, error) {
	client := http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func validateSchema(schemaResp schemaResponse, info *pb.RpcDownloadManifestResponseManifestInfo) (err error) {
	if schemaResp.Schema == "" {
		return
	}
	var result *gojsonschema.Result
	schemaLoader := gojsonschema.NewReferenceLoader(schemaResp.Schema)
	jsonLoader := gojsonschema.NewGoLoader(info)
	result, err = gojsonschema.Validate(schemaLoader, jsonLoader)
	if err != nil {
		return err
	}
	if !result.Valid() {
		return buildResultError(result)
	}
	info.Schema = schemaResp.Schema
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
