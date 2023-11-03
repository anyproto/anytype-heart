package gallery

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/anyproto/anytype-heart/pb"
)

func DownloadManifest(url string) (info *pb.RpcGalleryDownloadManifestResponseManifestInfo, err error) {
	spaceClient := http.Client{
		Timeout: time.Second * 2, // Timeout after 2 seconds
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return
	}

	req.Header.Set("User-Agent", "spacecount-tutorial")

	res, getErr := spaceClient.Do(req)
	if getErr != nil {
		return
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	body, readErr := io.ReadAll(res.Body)
	if readErr != nil {
		return nil, readErr
	}

	jsonErr := json.Unmarshal(body, &info)
	if jsonErr != nil {
		return nil, jsonErr
	}
	return info, nil
}
