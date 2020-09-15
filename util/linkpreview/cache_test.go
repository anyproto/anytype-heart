package linkpreview

import (
	"strings"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_Fetch(t *testing.T) {
	ts := newTestServer("text/html", strings.NewReader(tetsHtml))
	lp := NewWithCache()

	info, err := lp.Fetch(ctx, ts.URL)
	require.NoError(t, err)
	assert.Equal(t, model.LinkPreview{
		Url:         ts.URL,
		FaviconUrl:  ts.URL + "/favicon.ico",
		Title:       "Title",
		Description: "Description",
		ImageUrl:    "http://site.com/images/example.jpg",
		Type:        model.LinkPreview_Page,
	}, info)

	ts.Close()

	info, err = lp.Fetch(ctx, ts.URL)
	require.NoError(t, err)
	assert.Equal(t, model.LinkPreview{
		Url:         ts.URL,
		FaviconUrl:  ts.URL + "/favicon.ico",
		Title:       "Title",
		Description: "Description",
		ImageUrl:    "http://site.com/images/example.jpg",
		Type:        model.LinkPreview_Page,
	}, info)

}
