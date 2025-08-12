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
Sed ut perspiciatis, unde omnis iste natus error sit voluptatem accusantium doloremque laudantium, totam rem aperiam eaque ipsa, quae ab illo inventore veritatis et quasi architecto beatae vitae dicta sunt, explicabo. Nemo enim ipsam voluptatem, quia voluptas sit, aspernatur aut odit aut fugit, sed quia consequuntur magni dolores eos, qui ratione voluptatem sequi nesciunt, neque porro quisquam est, qui do<b>lorem ipsum</b>, quia <b>dolor sit, amet, consectetur, adipisci</b> v<b>elit, sed</b> quia non numquam <b>eius mod</b>i <b>tempor</b>a <b>incidunt, ut labore et dolore magna</b>m <b>aliqua</b>m quaerat voluptatem. <b>Ut enim ad minim</b>a <b>veniam, quis nostru</b>m <b>exercitation</b>em <b>ullam co</b>rporis suscipit<b> labori</b>o<b>s</b>am, <b>nisi ut aliquid ex ea commod</b>i <b>consequat</b>ur? <b>Quis aute</b>m vel eum <b>iure reprehenderit,</b> qui <b>in</b> ea <b>voluptate velit esse</b>, quam nihil molestiae <b>c</b>onsequatur, vel <b>illum</b>, qui <b>dolore</b>m <b>eu</b>m <b>fugiat</b>, quo voluptas <b>nulla pariatur</b>? At vero eos et accusamus et iusto odio dignissimos ducimus, qui blanditiis praesentium voluptatum deleniti atque corrupti, quos dolores et quas molestias <b>exceptur</b>i <b>sint, obcaecat</b>i <b>cupiditat</b>e <b>non pro</b>v<b>ident</b>, similique <b>sunt in culpa</b>, <b>qui officia deserunt mollit</b>ia <b>anim</b>i, <b>id est laborum</b> et dolorum fuga. Et harum quidemi rerum facilis est et expedita distinctio. Nam libero tempore, cum soluta nobis est eligendi optio, cumque nihil impedit, quo minus id, quod maxime placeat, facere possimus, omnis voluptas assumenda est, omnis dolor repellendus. Temporibus autem quibusdam et aut officiis debitis aut rerum necessitatibus saepe eveniet, ut et voluptates repudiandae sint et molestiae non recusandae. Itaque earum rerum hic tenetur a sapiente delectus, ut aut reiciendis voluptatibus maiores alias consequatur aut perferendis doloribus asperiores repellat.
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

	t.Run("Cache-Control private header", func(t *testing.T) {
		testCases := []struct {
			name          string
			cacheControl  string
			shouldBeError bool
		}{
			{"private directive", "private", true},
			{"no-store directive", "no-store", true},
			{"no-cache directive", "no-cache", true},
			{"must-revalidate directive", "must-revalidate", true},
			{"mixed with private", "public, private, max-age=3600", true},
			{"mixed with no-store", "max-age=0, no-store", true},
			{"public only", "public, max-age=3600", false},
			{"max-age only", "max-age=3600", false},
			{"case insensitive", "PRIVATE, MAX-AGE=0", true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				resp := &http.Response{
					Header: http.Header{
						"Cache-Control": {tc.cacheControl},
					},
				}
				err := checkPrivateLink(resp)
				if tc.shouldBeError {
					require.Error(t, err)
					assert.ErrorIs(t, err, ErrPrivateLink)
					assert.Contains(t, err.Error(), "Cache-Control")
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
			{"prefetch-src none", "prefetch-src 'none'", true},
			{"both restrictive", "default-src 'none'; prefetch-src 'none'", true},
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

	t.Run("Referrer-Policy headers", func(t *testing.T) {
		testCases := []struct {
			name          string
			policy        string
			shouldBeError bool
		}{
			{"no-referrer", "no-referrer", true},
			{"strict-origin", "strict-origin", false},
			{"same-origin", "same-origin", false},
			{"case insensitive", "NO-REFERRER", true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				resp := &http.Response{
					Header: http.Header{
						"Referrer-Policy": {tc.policy},
					},
				}
				err := checkPrivateLink(resp)
				if tc.shouldBeError {
					require.Error(t, err)
					assert.ErrorIs(t, err, ErrPrivateLink)
					assert.Contains(t, err.Error(), "Referrer-Policy")
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
			{"noindex", "noindex", true},
			{"nofollow", "nofollow", true},
			{"noarchive", "noarchive", true},
			{"none", "none", true},
			{"multiple directives", "noindex, nofollow", true},
			{"index allowed", "index, follow", false},
			{"case insensitive", "NOINDEX", true},
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
				"Cache-Control":           {"private, no-store"},
				"Content-Security-Policy": {"default-src 'none'"},
				"Referrer-Policy":         {"no-referrer"},
				"X-Robots-Tag":            {"noindex, nofollow"},
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
					"Cache-Control": {""},
					"X-Robots-Tag":  {""},
				},
				false,
			},
			{
				"substring matching",
				http.Header{
					"Cache-Control": {"public-private"}, // should not match "private"
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
	t.Run("private link with Cache-Control", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Cache-Control", "private, no-store")
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
			w.Header().Set("X-Robots-Tag", "noindex, nofollow")
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
			w.Header().Set("Cache-Control", "private")
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
