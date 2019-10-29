package linkpreview

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func TestLinkPreview_Fetch(t *testing.T) {
	t.Run("html page", func(t *testing.T) {
		ts := newTestServer("text/html", strings.NewReader(tetsHtml))
		defer ts.Close()
		lp := New()

		info, err := lp.Fetch(ctx, ts.URL)
		require.NoError(t, err)
		assert.Equal(t, pb.LinkPreviewResponse{
			Title:       "Title",
			Description: "Description",
			ImageUrl:    "http://site.com/images/example.jpg",
			Type:        pb.LinkPreviewResponse_PAGE,
		}, info)
	})

	t.Run("binary image", func(t *testing.T) {
		tr := testReader(0)
		ts := newTestServer("image/jpg", &tr)
		defer ts.Close()
		url := ts.URL + "/filename.jpg"
		lp := New()
		info, err := lp.Fetch(ctx, url)
		require.NoError(t, err)
		assert.Equal(t, pb.LinkPreviewResponse{
			Title:    "filename.jpg",
			ImageUrl: url,
			Type:     pb.LinkPreviewResponse_IMAGE,
		}, info)
		assert.True(t, int(tr) <= maxBytesToRead)
	})

	t.Run("binary", func(t *testing.T) {
		tr := testReader(0)
		ts := newTestServer("binary/octed-stream", &tr)
		defer ts.Close()
		url := ts.URL + "/filename.jpg"
		lp := New()
		info, err := lp.Fetch(ctx, url)
		require.NoError(t, err)
		assert.Equal(t, pb.LinkPreviewResponse{
			Title: "filename.jpg",
			Type:  pb.LinkPreviewResponse_UNEXPECTED,
		}, info)
		assert.True(t, int(tr) <= maxBytesToRead)
	})
}

func newTestServer(contentType string, data io.Reader) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		io.Copy(w, data)
	}))
}

const tetsHtml = `<html><head>
<title>Title</title>
<meta name="description" content="Description">
<meta property="og:image" content="http://site.com/images/example.jpg" />
</head></html>`

type testReader int

func (t *testReader) Read(p []byte) (n int, err error) {
	*t += testReader(len(p))
	return len(p), nil
}
