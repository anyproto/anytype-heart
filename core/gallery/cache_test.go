package gallery

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var ctx = context.Background()

func TestIndexCache_GetIndex(t *testing.T) {
	server := startHttpServer()
	defer server.Shutdown(nil)

	t.Run("get index from cache, no url provided", func(t *testing.T) {
		// given
		c := cache{
			getLocalIndex: func(string) (*pb.RpcGalleryDownloadIndexResponse, error) {
				return &pb.RpcGalleryDownloadIndexResponse{}, nil
			},
			getLocalVersion: func(string) (string, error) {
				return "v1", nil
			},
			save: func(path string, index *pb.RpcGalleryDownloadIndexResponse, version string) {
				panic("no need to save cache")
			},
		}

		// when
		_, err := c.GetIndex(0)

		// then
		assert.NoError(t, err)
	})

	t.Run("get index from remote, version differs", func(t *testing.T) {
		// given
		c := cache{
			indexURL: "http://localhost" + port + "/index.json",
			getLocalIndex: func(string) (*pb.RpcGalleryDownloadIndexResponse, error) {
				return &pb.RpcGalleryDownloadIndexResponse{}, nil
			},
			getLocalVersion: func(string) (string, error) {
				return "v1", nil
			},
			save: func(path string, index *pb.RpcGalleryDownloadIndexResponse, version string) {
				assert.Equal(t, "v2", version)
				assert.NotNil(t, index)
				assert.Len(t, index.Experiences, 1)
			},
		}

		// when
		index, err := c.GetIndex(0)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Experiences, 1)
	})

	t.Run("get index from remote, version is the same", func(t *testing.T) {
		// given
		c := cache{
			indexURL: "http://localhost" + port + "/index.json",
			getLocalIndex: func(string) (*pb.RpcGalleryDownloadIndexResponse, error) {
				return &pb.RpcGalleryDownloadIndexResponse{}, nil
			},
			getLocalVersion: func(string) (string, error) {
				return "v2", nil
			},
			save: func(path string, index *pb.RpcGalleryDownloadIndexResponse, version string) {
				panic("no need to save cache")
			},
		}

		// when
		index, err := c.GetIndex(0)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Experiences, 0)
	})

	t.Run("failed to read local index", func(t *testing.T) {
		// given
		c := cache{
			indexURL: "http://localhost" + port + "/index.json",
			getLocalIndex: func(string) (*pb.RpcGalleryDownloadIndexResponse, error) {
				return nil, errors.New("error on read")
			},
			getLocalVersion: func(string) (string, error) {
				return "v1", nil
			},
			save: func(path string, index *pb.RpcGalleryDownloadIndexResponse, version string) {
				assert.Equal(t, "v2", version)
				assert.NotNil(t, index)
				assert.Len(t, index.Experiences, 1)
			},
		}

		// when
		index, err := c.GetIndex(0)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, index)
		assert.Len(t, index.Experiences, 1)
	})

	t.Run("failed to both read local index and download remote one", func(t *testing.T) {
		// given
		c := cache{
			getLocalIndex: func(string) (*pb.RpcGalleryDownloadIndexResponse, error) {
				return nil, errors.New("error on read")
			},
			save: func(path string, index *pb.RpcGalleryDownloadIndexResponse, version string) {
				panic("nothing to save")
			},
		}

		// when
		_, err := c.GetIndex(0)

		// then
		assert.Error(t, err)
	})
}

func TestIndexCache_GetManifest(t *testing.T) {
	server := startHttpServer()
	defer server.Shutdown(nil)

	t.Run("get manifest from cache, no url provided", func(t *testing.T) {
		// given
		c := cache{
			getLocalIndex: func(string) (*pb.RpcGalleryDownloadIndexResponse, error) {
				return &pb.RpcGalleryDownloadIndexResponse{Experiences: []*model.ManifestInfo{{
					DownloadLink: "test.link",
					Name:         "test",
				}}}, nil
			},
			getLocalVersion: func(string) (string, error) {
				return "v1", nil
			},
			save: func(path string, index *pb.RpcGalleryDownloadIndexResponse, version string) {
				panic("no need to save cache")
			},
		}

		// when
		info, err := c.GetManifest("test.link", 0)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, "test", info.Name)
	})

	t.Run("get manifest from remote, version differs", func(t *testing.T) {
		// given
		c := cache{
			indexURL: "http://localhost" + port + "/index.json",
			getLocalIndex: func(string) (*pb.RpcGalleryDownloadIndexResponse, error) {
				return &pb.RpcGalleryDownloadIndexResponse{}, nil
			},
			getLocalVersion: func(string) (string, error) {
				return "v1", nil
			},
			save: func(path string, index *pb.RpcGalleryDownloadIndexResponse, version string) {
				assert.Equal(t, "v2", version)
				assert.NotNil(t, index)
				assert.Len(t, index.Experiences, 1)
			},
		}

		// when
		info, err := c.GetManifest("https://github.com/anyproto/gallery/raw/main/experiences/knowledge_base/knowledge_base.zip", 0)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, "name", info.Name)
	})

	t.Run("get manifest from remote, version is the same", func(t *testing.T) {
		// given
		c := cache{
			indexURL: "http://localhost" + port + "/index.json",
			getLocalIndex: func(string) (*pb.RpcGalleryDownloadIndexResponse, error) {
				return &pb.RpcGalleryDownloadIndexResponse{Experiences: []*model.ManifestInfo{{
					DownloadLink: "test.link",
					Name:         "test",
				}}}, nil
			},
			getLocalVersion: func(string) (string, error) {
				return "v2", nil
			},
			save: func(path string, index *pb.RpcGalleryDownloadIndexResponse, version string) {
				panic("no need to save cache")
			},
		}

		// when
		info, err := c.GetManifest("test.link", 0)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, "test", info.Name)
	})

	t.Run("failed to read local index", func(t *testing.T) {
		// given
		c := cache{
			indexURL: "http://localhost" + port + "/index.json",
			getLocalIndex: func(string) (*pb.RpcGalleryDownloadIndexResponse, error) {
				return nil, errors.New("error on read")
			},
			getLocalVersion: func(string) (string, error) {
				return "v1", nil
			},
			save: func(path string, index *pb.RpcGalleryDownloadIndexResponse, version string) {
				assert.Equal(t, "v2", version)
				assert.NotNil(t, index)
				assert.Len(t, index.Experiences, 1)
			},
		}

		// when
		info, err := c.GetManifest("https://github.com/anyproto/gallery/raw/main/experiences/knowledge_base/knowledge_base.zip", 0)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, "name", info.Name)
	})

	t.Run("failed to both read local index and download remote one", func(t *testing.T) {
		// given
		c := cache{
			getLocalIndex: func(string) (*pb.RpcGalleryDownloadIndexResponse, error) {
				return nil, errors.New("error on read")
			},
			save: func(path string, index *pb.RpcGalleryDownloadIndexResponse, version string) {
				panic("nothing to save")
			},
		}

		// when
		_, err := c.GetManifest("link", 0)

		// then
		assert.Error(t, err)
	})
}
