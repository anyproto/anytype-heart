package gallery

import (
	_ "embed"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const port = "37373"

//go:embed testdata/schema.json
var schemaJSON []byte

//go:embed testdata/client_cache/get_started.zip
var testZip []byte

type testCacheStorage struct {
	version    string
	index      *pb.RpcGalleryDownloadIndexResponse
	assertSave func(index *pb.RpcGalleryDownloadIndexResponse, version string)
}

func (tcs *testCacheStorage) getIndex() (*pb.RpcGalleryDownloadIndexResponse, error) {
	if tcs.index != nil {
		return tcs.index, nil
	}
	return nil, errors.New("failed to get index")
}

func (tcs *testCacheStorage) getVersion() (string, error) {
	if tcs.version != "" {
		return tcs.version, nil
	}
	return "", errors.New("failed to get version")
}

func (tcs *testCacheStorage) save(index *pb.RpcGalleryDownloadIndexResponse, version string) {
	if tcs.assertSave == nil {
		panic("no need to save cache")
	}
	tcs.assertSave(index, version)
}

func buildServer(t *testing.T, hash string) *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/index.json", func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get(versionHeader) == "v2" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set(eTagHeader, "v2")
		w.WriteHeader(http.StatusOK)
		info := buildIndex(hash)
		rawInfo, err := json.Marshal(info)
		require.NoError(t, err)
		_, err = w.Write(rawInfo)
		require.NoError(t, err)
	})
	mux.HandleFunc("/schema.json", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(schemaJSON)
		require.NoError(t, err)
	})
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		info := buildInfo(hash)
		rawInfo, err := json.Marshal(info)
		require.NoError(t, err)
		_, err = w.Write(rawInfo)
		require.NoError(t, err)
	})
	mux.HandleFunc("/get_started.zip", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(testZip)
		require.NoError(t, err)
	})
	mux.HandleFunc("/experience.zip", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(testZip)
		require.NoError(t, err)
	})

	ts := httptest.NewUnstartedServer(mux)
	l, err := net.Listen("tcp", "127.0.0.1:"+port)
	require.NoError(t, err)
	require.NoError(t, ts.Listener.Close())
	ts.Listener = l
	ts.Start()

	return ts
}

func buildInfo(hash string) *model.ManifestInfo {
	return &model.ManifestInfo{
		Schema:       "http://127.0.0.1:" + port + "/schema.json",
		Id:           "id",
		Name:         "name",
		Author:       "https://github.com/anyproto",
		License:      "MIT",
		Title:        "Name",
		Description:  "Description of usecase",
		Screenshots:  []string{"https://anytype.io/assets/usecases/Knowledge%20base.jpg", "https://anytype.io/assets/usecases/Knowledge%20base_movie.jpg"},
		DownloadLink: "https://github.com/anyproto/gallery/raw/main/experiences/knowledge_base/knowledge_base.zip",
		FileSize:     42,
		Categories:   []string{"Education", "Work"},
		Language:     "hi-IN",
		Hash:         hash,
	}
}

func buildIndex(hash string) *pb.RpcGalleryDownloadIndexResponse {
	return &pb.RpcGalleryDownloadIndexResponse{
		Experiences: []*model.ManifestInfo{
			buildInfo(hash),
		},
	}
}
