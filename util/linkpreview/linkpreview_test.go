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

		info, _, isFile, err := lp.Fetch(ctx, ts.URL, false)
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

		info, _, isFile, err := lp.Fetch(ctx, ts.URL, false)
		require.NoError(t, err)
		assert.Equal(t, model.LinkPreview{
			Url:         ts.URL,
			FaviconUrl:  ts.URL + "/favicon.ico",
			Title:       "Title",
			Description: "Sed ut perspiciatis, unde omnis iste natus error sit voluptatem accusantium doloremque laudantium, totam rem aperiam eaque ipsa, quae ab illo inventore veritatis et quasi architecto beatae vitae …",
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
		info, _, isFile, err := lp.Fetch(ctx, url, false)
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

func TestCheckResponseHeaders(t *testing.T) {
	t.Run("nil response", func(t *testing.T) {
		whitelist, err := checkResponseHeaders(nil)
		require.Error(t, err)
		assert.Empty(t, whitelist)
		assert.Contains(t, err.Error(), "response is nil")
	})

	t.Run("no privacy headers - should be public", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{
				"Content-Type": {"text/html"},
				"Server":       {"nginx/1.18.0"},
			},
		}
		whitelist, err := checkResponseHeaders(resp)
		require.NoError(t, err)
		assert.Empty(t, whitelist)
	})

	t.Run("Content-Security-Policy headers", func(t *testing.T) {
		testCases := []struct {
			name  string
			csp   string
			rules []string
		}{
			{"default-src none", "default-src 'none'", []string{"'none'"}},
			{"normal CSP", "default-src 'self'; script-src 'unsafe-inline'", []string{"'self'"}},
			{"case insensitive", "DEFAULT-SRC 'NONE'", []string{"'none'"}},
			{"reach content", "img-src example.com sample.net 'self'; default-src 'none'", []string{"example.com", "sample.net", "'self'"}},
			{"img-src is preferable", "img-src example.com; default-src 'self'", []string{"example.com"}},
			{"img-src is restrictive", "img-src 'none'; default-src 'self'", []string{"'none'"}},
			{"only img-src", "img-src 'self'", []string{"'self'"}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				resp := &http.Response{
					Header: http.Header{
						"Content-Security-Policy": {tc.csp},
					},
				}
				cspRules, err := checkResponseHeaders(resp)
				require.NoError(t, err)
				assert.Equal(t, tc.rules, cspRules)
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
			{"robot name include directive option", "nonebot: all", false},
			{"robot with name", "Verter: all, Bender: none", true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				resp := &http.Response{
					Header: http.Header{
						"X-Robots-Tag": {tc.robotsTag},
					},
				}
				_, err := checkResponseHeaders(resp)
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
		_, err := checkResponseHeaders(resp)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPrivateLink)
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
				_, err := checkResponseHeaders(resp)
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
	t.Run("private link with X-Robots-Tag", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("X-Robots-Tag", "none")
			w.Write([]byte(tetsHtml))
		}))
		defer ts.Close()

		lp := New()
		lp.Init(nil)

		_, _, _, err := lp.Fetch(ctx, ts.URL, false)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPrivateLink)
	})

	t.Run("private image file", func(t *testing.T) {
		tr := testReader(0)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/jpg")
			w.Header().Set("X-Robots-Tag", "none")
			io.Copy(w, &tr)
		}))
		defer ts.Close()

		lp := New()
		lp.Init(nil)

		_, _, _, err := lp.Fetch(ctx, ts.URL+"/filename.jpg", false)
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

		info, _, isFile, err := lp.Fetch(ctx, ts.URL, false)
		require.NoError(t, err)
		assert.False(t, isFile)
		assert.Equal(t, ts.URL, info.Url)
		assert.Equal(t, "Title", info.Title)
	})
}

func TestCheckLinksWhitelist(t *testing.T) {
	t.Run("empty whitelist should allow all", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:        "https://example.com/page",
			ImageUrl:   "https://cdn.example.com/image.jpg",
			FaviconUrl: "https://static.example.com/favicon.ico",
		}
		var emptyWhitelist []string

		// when
		applyCSPRules(emptyWhitelist, preview)

		// then
		assert.NotEmpty(t, preview.ImageUrl)
		assert.NotEmpty(t, preview.FaviconUrl)
	})

	t.Run("nil whitelist should allow all", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:        "https://example.com/page",
			ImageUrl:   "https://cdn.example.com/image.jpg",
			FaviconUrl: "https://static.example.com/favicon.ico",
		}

		// when
		applyCSPRules(nil, preview)

		// then
		assert.NotEmpty(t, preview.ImageUrl)
		assert.NotEmpty(t, preview.FaviconUrl)
	})

	t.Run("'self' directive should expand to main URL host", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:        "https://example.com/page",
			ImageUrl:   "https://example.com/image.jpg",   // same host as main URL
			FaviconUrl: "https://example.com/favicon.ico", // same host as main URL
		}
		whitelist := []string{"'self'"}

		// when
		applyCSPRules(whitelist, preview)

		// then
		assert.NotEmpty(t, preview.ImageUrl)
		assert.NotEmpty(t, preview.FaviconUrl)
	})

	t.Run("'self' directive should reject different hosts", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:        "https://example.com/page",
			ImageUrl:   "https://cdn.example.com/image.jpg", // different host
			FaviconUrl: "https://example.com/favicon.ico",   // same host
		}
		whitelist := []string{"'self'"}

		// when
		applyCSPRules(whitelist, preview)

		// then
		assert.Empty(t, preview.ImageUrl)
		assert.NotEmpty(t, preview.FaviconUrl)
	})

	t.Run("explicit host whitelist should allow matching hosts", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:        "https://example.com/page",
			ImageUrl:   "https://cdn.example.com/image.jpg",
			FaviconUrl: "https://static.example.com/favicon.ico",
		}
		whitelist := []string{"cdn.example.com", "static.example.com"}

		// when
		applyCSPRules(whitelist, preview)

		// then
		assert.NotEmpty(t, preview.ImageUrl)
		assert.NotEmpty(t, preview.FaviconUrl)
	})

	t.Run("explicit host whitelist should reject non-matching hosts", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:        "https://example.com/page",
			ImageUrl:   "https://malicious.com/image.jpg",        // not in whitelist
			FaviconUrl: "https://static.example.com/favicon.ico", // in whitelist
		}
		whitelist := []string{"static.example.com", "allowed.com"}

		// when
		applyCSPRules(whitelist, preview)

		// then
		assert.Empty(t, preview.ImageUrl)
		assert.NotEmpty(t, preview.FaviconUrl)
	})

	t.Run("combined 'self' and explicit hosts", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:        "https://example.com/page",
			ImageUrl:   "https://example.com/image.jpg",       // matches 'self'
			FaviconUrl: "https://cdn.trusted.com/favicon.ico", // matches explicit host
		}
		whitelist := []string{"'self'", "cdn.trusted.com"}

		// when
		applyCSPRules(whitelist, preview)

		// then
		assert.NotEmpty(t, preview.ImageUrl)
		assert.NotEmpty(t, preview.FaviconUrl)
	})

	t.Run("mixed wildcard and specific domains", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:        "https://example.com/page",
			ImageUrl:   "https://example.com/image.jpg",
			FaviconUrl: "https://specific.com/favicon.ico",
		}
		whitelist := []string{"*", "specific.com", "'self'"}

		// when
		applyCSPRules(whitelist, preview)

		// then
		assert.NotEmpty(t, preview.ImageUrl)
		assert.NotEmpty(t, preview.FaviconUrl)
	})

	t.Run("template in whitelist", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			ImageUrl:   "https://img.example.com/image.jpg",
			FaviconUrl: "https://fav.example.com/favicon.ico",
		}
		whitelist := []string{"*.example.com"}

		// when
		applyCSPRules(whitelist, preview)

		// then
		assert.NotEmpty(t, preview.ImageUrl)
		assert.NotEmpty(t, preview.FaviconUrl)
	})

	t.Run("schema in whitelist", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			ImageUrl:   "https://img.example.com/image.jpg",
			FaviconUrl: "https://fav.example.com/favicon.ico",
		}
		whitelist := []string{"https:"}

		// when
		applyCSPRules(whitelist, preview)

		// then
		assert.NotEmpty(t, preview.ImageUrl)
		assert.NotEmpty(t, preview.FaviconUrl)
	})
}

func TestReplaceGenericTitle(t *testing.T) {
	t.Run("should not change title when empty HTML content", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://www.reddit.com/r/golang/test",
			Title: "Reddit - The heart of the internet",
		}
		var emptyHTML []byte

		// when
		replaceGenericTitle(preview, emptyHTML)

		// then
		assert.Equal(t, "Reddit - The heart of the internet", preview.Title)
	})

	t.Run("should not change title when not a tracked domain", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://example.com/test",
			Title: "Some Generic Title",
		}
		htmlContent := []byte(`<html><body><h1>Specific Title</h1></body></html>`)

		// when
		replaceGenericTitle(preview, htmlContent)

		// then
		assert.Equal(t, "Some Generic Title", preview.Title)
	})

	t.Run("should not change title when title is not generic", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://www.reddit.com/r/golang/test",
			Title: "Let's play tetris!",
		}
		htmlContent := []byte(`<html><body><h1 slot="title">Tetris is full of fun!</h1></body></html>`)

		// when
		replaceGenericTitle(preview, htmlContent)

		// then
		assert.Equal(t, "Let's play tetris!", preview.Title)
	})

	t.Run("should replace Reddit generic title with h1[slot='title']", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://www.reddit.com/r/golang/comments/123/test",
			Title: "Reddit - The heart of the internet",
		}
		htmlContent := []byte(`
			<html>
				<body>
					<h1 slot="title">How to write better Go code</h1>
					<div>Other content</div>
				</body>
			</html>
		`)

		// when
		replaceGenericTitle(preview, htmlContent)

		// then
		assert.Equal(t, "How to write better Go code", preview.Title)
	})

	t.Run("should replace Reddit generic title with shreddit-post h1", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://www.reddit.com/r/golang",
			Title: "Reddit",
		}
		htmlContent := []byte(`
			<html>
				<body>
					<shreddit-post>
						<h1>Discussion: Go 1.21 Features</h1>
					</shreddit-post>
				</body>
			</html>
		`)

		// when
		replaceGenericTitle(preview, htmlContent)

		// then
		assert.Equal(t, "Discussion: Go 1.21 Features", preview.Title)
	})

	t.Run("should replace Reddit generic title with h1 containing 'r/'", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://www.reddit.com/r/golang",
			Title: "Reddit - The heart of the internet",
		}
		htmlContent := []byte(`
			<html>
				<body>
					<h1>r/golang: The Go Programming Language</h1>
					<h2>Other heading</h2>
				</body>
			</html>
		`)

		// when
		replaceGenericTitle(preview, htmlContent)

		// then
		assert.Equal(t, "r/golang: The Go Programming Language", preview.Title)
	})

	t.Run("should use first matching selector", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://www.reddit.com/test",
			Title: "Reddit - The heart of the internet",
		}
		htmlContent := []byte(`
			<html>
				<body>
					<h1 slot="title">First Title</h1>
					<div data-test-id="post-content">
						<h3>Second Title</h3>
					</div>
					<shreddit-post>
						<h1>Third Title</h1>
					</shreddit-post>
				</body>
			</html>
		`)

		// when
		replaceGenericTitle(preview, htmlContent)

		// then
		assert.Equal(t, "First Title", preview.Title)
	})

	t.Run("should trim whitespace from extracted title", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://www.reddit.com/test",
			Title: "Reddit - Dive into anything",
		}
		htmlContent := []byte(`
			<html>
				<body>
					<h1 slot="title">   Trimmed Title   </h1>
				</body>
			</html>
		`)

		// when
		replaceGenericTitle(preview, htmlContent)

		// then
		assert.Equal(t, "Trimmed Title", preview.Title)
	})

	t.Run("should truncate long titles", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://www.reddit.com/test",
			Title: "Reddit - The heart of the internet",
		}
		longTitle := "This is a very long title that exceeds the maximum length limit of 100 characters and should be truncated"
		htmlContent := []byte(`
			<html>
				<body>
					<h1 slot="title">` + longTitle + `</h1>
				</body>
			</html>
		`)

		// when
		replaceGenericTitle(preview, htmlContent)

		// then
		assert.True(t, len(preview.Title) <= 100)
	})

	t.Run("should not use titles shorter than 5 characters", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://www.reddit.com/test",
			Title: "Reddit - The heart of the internet",
		}
		htmlContent := []byte(`
			<html>
				<body>
					<h1 slot="title">    </h1>
					<div data-test-id="post-content">
						<h3>Hi</h3>
					</div>
					<shreddit-post>
						<h1>Good Title</h1>
					</shreddit-post>
				</body>
			</html>
		`)

		// when
		replaceGenericTitle(preview, htmlContent)

		// then
		assert.Equal(t, "Good Title", preview.Title)
	})

	t.Run("should not change title if no valid replacement found", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://www.reddit.com/test",
			Title: "Reddit - The heart of the internet",
		}
		htmlContent := []byte(`
			<html>
				<body>
					<h1 slot="title">   </h1>
					<div data-test-id="post-content">
						<h3></h3>
					</div>
					<div>No matching selectors here</div>
				</body>
			</html>
		`)

		// when
		replaceGenericTitle(preview, htmlContent)

		// then
		assert.Equal(t, "Reddit - The heart of the internet", preview.Title)
	})

	t.Run("should handle malformed HTML gracefully", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://www.reddit.com/test",
			Title: "Reddit - The heart of the internet",
		}
		malformedHTML := []byte(`<html><body><h1 slot="title">Valid Title</h1><unclosed><tag></body></html>`)

		// when
		replaceGenericTitle(preview, malformedHTML)

		// then
		assert.Equal(t, "Valid Title", preview.Title)
	})

	t.Run("should handle invalid HTML gracefully", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://www.reddit.com/test",
			Title: "Reddit - The heart of the internet",
		}
		invalidHTML := []byte(`not html at all`)

		// when
		replaceGenericTitle(preview, invalidHTML)

		// then
		assert.Equal(t, "Reddit - The heart of the internet", preview.Title)
	})

	// Test case-insensitive matching
	t.Run("should match generic title case-insensitively", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://www.reddit.com/test",
			Title: "REDDIT - THE HEART OF THE INTERNET",
		}
		htmlContent := []byte(`
			<html>
				<body>
					<h1 slot="title">Actual Post Title</h1>
				</body>
			</html>
		`)

		// when
		replaceGenericTitle(preview, htmlContent)

		// then
		assert.Equal(t, "Actual Post Title", preview.Title)
	})

	t.Run("should match partial generic title", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://www.reddit.com/test",
			Title: "Are you sure Reddit - The heart of the internet ?",
		}
		htmlContent := []byte(`
			<html>
				<body>
					<h1 slot="title">Yes we are sure</h1>
				</body>
			</html>
		`)

		// when
		replaceGenericTitle(preview, htmlContent)

		// then
		assert.Equal(t, "Yes we are sure", preview.Title)
	})

	t.Run("should not match similar domains", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://notreddit.com/test",
			Title: "Reddit - The heart of the internet",
		}
		htmlContent := []byte(`<html><body><h1 slot="title">Should Not Replace</h1></body></html>`)

		// when
		replaceGenericTitle(preview, htmlContent)

		// then
		assert.Equal(t, "Reddit - The heart of the internet", preview.Title)
	})

	t.Run("should handle empty title from HTML", func(t *testing.T) {
		// given
		preview := &model.LinkPreview{
			Url:   "https://www.reddit.com/test",
			Title: "Reddit - The heart of the internet",
		}
		htmlContent := []byte(`
			<html>
				<body>
					<h1 slot="title"></h1>
					<div data-test-id="post-content">
						<h3>   </h3>
					</div>
					<shreddit-post>
						<h1>Final Good Title</h1>
					</shreddit-post>
				</body>
			</html>
		`)

		// when
		replaceGenericTitle(preview, htmlContent)

		// then
		assert.Equal(t, "Final Good Title", preview.Title)
	})
}
