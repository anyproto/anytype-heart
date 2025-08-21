package linkpreview

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var ctx = context.Background()

func TestLinkPreview_Fetch(t *testing.T) {
	t.Run("html page", func(t *testing.T) {
		ts := newTestServer("text/html", strings.NewReader(tetsHtml))
		defer ts.Close()
		lp := New()
		lp.Init(nil)

		info, _, isFile, err := lp.Fetch(ctx, ts.URL)
		require.NoError(t, err)
		assert.False(t, isFile)
		assert.Equal(t, model.LinkPreview{
			Url:         ts.URL,
			FaviconUrl:  ts.URL + "/favicon.ico",
			Title:       "Title",
			Description: "Description",
			ImageUrl:    "http://site.com/images/example.jpg",
			Type:        model.LinkPreview_Page,
		}, info)
	})

	t.Run("html page and find description", func(t *testing.T) {
		ts := newTestServer("text/html", strings.NewReader(tetsHtmlWithoutDescription))
		defer ts.Close()
		lp := New()
		lp.Init(nil)

		info, _, isFile, err := lp.Fetch(ctx, ts.URL)
		require.NoError(t, err)
		assert.Equal(t, model.LinkPreview{
			Url:         ts.URL,
			FaviconUrl:  ts.URL + "/favicon.ico",
			Title:       "Title",
			Description: "Sed ut perspiciatis, unde omnis iste natus error sit voluptatem accusantium doloremque laudantium, totam rem aperiam eaque ipsa, quae ab illo inventore veritatis et quasi architecto beatae vitae dicta...",
			ImageUrl:    "http://site.com/images/example.jpg",
			Type:        model.LinkPreview_Page,
		}, info)
		assert.False(t, isFile)
	})

	t.Run("binary image", func(t *testing.T) {
		tr := testReader(0)
		ts := newTestServer("image/jpg", &tr)
		defer ts.Close()
		url := ts.URL + "/filename.jpg"
		lp := New()
		lp.Init(nil)
		info, _, isFile, err := lp.Fetch(ctx, url)
		require.NoError(t, err)
		assert.Equal(t, model.LinkPreview{
			Url:        url,
			Title:      "filename.jpg",
			FaviconUrl: ts.URL + "/favicon.ico",
			ImageUrl:   url,
			Type:       model.LinkPreview_Image,
		}, info)
		assert.True(t, isFile)
	})

	t.Run("check content is file by extension", func(t *testing.T) {
		// given
		resp := &http.Response{Header: map[string][]string{}}

		// when
		isFile := checkFileType("http://site.com/images/example.jpg", resp, "")

		// then
		assert.True(t, isFile)
	})
	t.Run("check content is file by content-type", func(t *testing.T) {
		// given
		resp := &http.Response{Header: map[string][]string{}}

		// when
		isFile := checkFileType("htt://example.com/filepath", resp, "application/pdf")

		// then
		assert.True(t, isFile)
	})
	t.Run("check content is file by content-disposition", func(t *testing.T) {
		// given
		resp := &http.Response{Header: map[string][]string{"Content-Disposition": {"attachment filename=\"user.csv\""}}}

		// when
		isFile := checkFileType("htt://example.com/filepath", resp, "")

		// then
		assert.True(t, isFile)
	})
	t.Run("check content is not file", func(t *testing.T) {
		// given
		resp := &http.Response{Header: map[string][]string{}}

		// when
		isFile := checkFileType("htt://example.com/notfile", resp, "")

		// then
		assert.False(t, isFile)
	})
}

func newTestServer(contentType string, data io.Reader) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		io.Copy(w, data)
	}))
}

type testReader int

func (t *testReader) Read(p []byte) (n int, err error) {
	*t += testReader(len(p))
	return len(p), nil
}

const tetsHtml = `<html><head>
<title>Title</title>
<meta name="description" content="Description">
<meta property="og:image" content="http://site.com/images/example.jpg" />
</head></html>`

const tetsHtmlWithoutDescription = `<html><head>
<title>Title</title>
<meta property="og:image" content="http://site.com/images/example.jpg" />
</head><body><div id="content"">
<p>
Sed ut perspiciatis, unde omnis iste natus error sit voluptatem accusantium doloremque laudantium, totam rem aperiam eaque ipsa, quae ab illo inventore veritatis et quasi architecto beatae vitae dicta sunt, explicabo.
</p></div></body></html>`

func TestCheckPrivateLink(t *testing.T) {
	t.Run("nil response", func(t *testing.T) {
		err := checkPrivateLink(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "response is nil")
	})

	t.Run("no privacy headers - should be public", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{
				"Content-Type": {"text/html"},
				"Server":       {"nginx/1.18.0"},
			},
		}
		err := checkPrivateLink(resp)
		require.NoError(t, err)
	})

	t.Run("X-Frame-Options headers", func(t *testing.T) {
		testCases := []struct {
			name          string
			frameOptions  string
			shouldBeError bool
		}{
			{"deny directive", "deny", true},
			{"sameorigin directive", "sameorigin", true},
			{"allowall should pass", "allowall", false},
			{"case insensitive deny", "DENY", true},
			{"case insensitive sameorigin", "SAMEORIGIN", true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				resp := &http.Response{
					Header: http.Header{
						"X-Frame-Options": {tc.frameOptions},
					},
				}
				err := checkPrivateLink(resp)
				if tc.shouldBeError {
					require.Error(t, err)
					assert.ErrorIs(t, err, ErrPrivateLink)
					assert.Contains(t, err.Error(), "X-Frame-Options")
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("Content-Security-Policy headers", func(t *testing.T) {
		testCases := []struct {
			name          string
			csp           string
			shouldBeError bool
		}{
			{"default-src none", "default-src 'none'", true},
			{"frame-ancestors none", "frame-ancestors 'none'", true},
			{"both restrictive", "default-src 'none'; frame-ancestors 'none'", true},
			{"normal CSP", "default-src 'self'; script-src 'unsafe-inline'", false},
			{"case insensitive", "DEFAULT-SRC 'NONE'", true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				resp := &http.Response{
					Header: http.Header{
						"Content-Security-Policy": {tc.csp},
					},
				}
				err := checkPrivateLink(resp)
				if tc.shouldBeError {
					require.Error(t, err)
					assert.ErrorIs(t, err, ErrPrivateLink)
					assert.Contains(t, err.Error(), "Content-Security-Policy")
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("X-Robots-Tag headers", func(t *testing.T) {
		testCases := []struct {
			name          string
			robotsTag     string
			shouldBeError bool
		}{
			{"none - most restrictive", "none", true},
			{"noindex should be allowed", "noindex", false},
			{"nofollow should be allowed", "nofollow", false},
			{"noarchive should be allowed", "noarchive", false},
			{"index allowed", "index, follow", false},
			{"case insensitive none", "NONE", true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				resp := &http.Response{
					Header: http.Header{
						"X-Robots-Tag": {tc.robotsTag},
					},
				}
				err := checkPrivateLink(resp)
				if tc.shouldBeError {
					require.Error(t, err)
					assert.ErrorIs(t, err, ErrPrivateLink)
					assert.Contains(t, err.Error(), "X-Robots-Tag")
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("multiple headers with privacy indicators", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{
				"X-Frame-Options":         {"deny"},
				"Content-Security-Policy": {"default-src 'none'"},
				"X-Robots-Tag":            {"none"},
			},
		}
		err := checkPrivateLink(resp)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPrivateLink)
		// Should detect the first privacy directive it encounters
		assert.Contains(t, err.Error(), "private link detected")
	})

	t.Run("edge cases", func(t *testing.T) {
		testCases := []struct {
			name    string
			headers http.Header
			wantErr bool
		}{
			{
				"empty headers",
				http.Header{},
				false,
			},
			{
				"empty header values",
				http.Header{
					"X-Frame-Options": {""},
					"X-Robots-Tag":    {""},
				},
				false,
			},
			{
				"substring matching",
				http.Header{
					"X-Robots-Tag": {"nonexistent"}, // should not match "none"
				},
				false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				resp := &http.Response{Header: tc.headers}
				err := checkPrivateLink(resp)
				if tc.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})
}

func TestLinkPreview_Fetch_PrivateLink(t *testing.T) {
	t.Run("private link with X-Frame-Options", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("X-Frame-Options", "deny")
			w.Write([]byte(tetsHtml))
		}))
		defer ts.Close()

		lp := New()
		lp.Init(nil)

		_, _, _, err := lp.Fetch(ctx, ts.URL)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPrivateLink)
		assert.Contains(t, err.Error(), "private link detected")
	})

	t.Run("private link with X-Robots-Tag", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("X-Robots-Tag", "none")
			w.Write([]byte(tetsHtml))
		}))
		defer ts.Close()

		lp := New()
		lp.Init(nil)

		_, _, _, err := lp.Fetch(ctx, ts.URL)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPrivateLink)
	})

	t.Run("private image file", func(t *testing.T) {
		tr := testReader(0)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/jpg")
			w.Header().Set("X-Frame-Options", "sameorigin")
			io.Copy(w, &tr)
		}))
		defer ts.Close()

		lp := New()
		lp.Init(nil)

		_, _, _, err := lp.Fetch(ctx, ts.URL+"/filename.jpg")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPrivateLink)
	})

	t.Run("public link should work", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Cache-Control", "public, max-age=3600")
			w.Write([]byte(tetsHtml))
		}))
		defer ts.Close()

		lp := New()
		lp.Init(nil)

		info, _, isFile, err := lp.Fetch(ctx, ts.URL)
		require.NoError(t, err)
		assert.False(t, isFile)
		assert.Equal(t, ts.URL, info.Url)
		assert.Equal(t, "Title", info.Title)
	})
}
