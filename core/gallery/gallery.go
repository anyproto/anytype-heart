package gallery

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/xeipuuv/gojsonschema"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/uri"
)

const timeout = time.Second * 30

type schemaResponse struct {
	Schema string `json:"$schema"`
}

var whitelist = []string{
	"raw.githubusercontent.com/anyproto",
	"community.anytype.io",
	"anytype.io",
	"gallery.any.coop",
	"github.com/anyproto",
}

func DownloadManifest(url string) (info *pb.RpcDownloadManifestResponseManifestInfo, err error) {
	if err = uri.ValidateURI(url); err != nil {
		return nil, fmt.Errorf("provided URL is not valid: %w", err)
	}
	if !isInWhitelist(url) {
		return nil, fmt.Errorf("URL '%s' is not in whitelist", url)
	}
	client := http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return
	}

	res, err := client.Do(req)
	if err != nil {
		return
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	schemaResp := schemaResponse{}
	err = json.Unmarshal(body, &schemaResp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall json to get schema: %w", err)
	}

	err = json.Unmarshal(body, &info)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall json to get manifest: %w", err)
	}

	if schemaResp.Schema != "" {
		var result *gojsonschema.Result
		schemaLoader := gojsonschema.NewReferenceLoader(schemaResp.Schema)
		jsonLoader := gojsonschema.NewGoLoader(info)
		result, err = gojsonschema.Validate(schemaLoader, jsonLoader)
		if err != nil {
			return nil, err
		}
		if !result.Valid() {
			return nil, fmt.Errorf("manifest does not correspond provided schema")
		}
		info.Schema = schemaResp.Schema
	}

	for _, urlToCheck := range append(info.Screenshots, info.DownloadLink) {
		if !isInWhitelist(urlToCheck) {
			return nil, fmt.Errorf("URL '%s' provided in manifest is not in whitelist", url)
		}
	}

	return info, nil
}

func isInWhitelist(url string) bool {
	if len(whitelist) == 0 {
		return true
	}
	for _, wlElement := range whitelist {
		if strings.Contains(url, wlElement) {
			return true
		}
	}
	return false
}
