package gallery

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/xeipuuv/gojsonschema"

	"github.com/anyproto/anytype-heart/pb"
)

const timeout = time.Second * 2

type schemaHandler struct {
	Schema string `json:"$schema"`
}

func DownloadManifest(url string) (info *pb.RpcGalleryDownloadManifestResponseManifestInfo, err error) {
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

	schemaWrapper := schemaHandler{}
	err = json.Unmarshal(body, &schemaWrapper)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall json to get schema: %w", err)
	}

	err = json.Unmarshal(body, &info)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall json to get manifest: %w", err)
	}

	if schemaWrapper.Schema != "" {
		var result *gojsonschema.Result
		schemaLoader := gojsonschema.NewReferenceLoader(schemaWrapper.Schema)
		jsonLoader := gojsonschema.NewGoLoader(info)
		result, err = gojsonschema.Validate(schemaLoader, jsonLoader)
		if err != nil {
			return nil, err
		}
		if !result.Valid() {
			return nil, fmt.Errorf("manifest does not correspond provided schema")
		}
		info.Schema = schemaWrapper.Schema
	}

	return info, nil
}
