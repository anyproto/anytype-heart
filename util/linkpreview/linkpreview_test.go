package linkpreview

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
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
		assert.Equal(t, model.ModelLinkPreview{
			Url:         ts.URL,
			FaviconUrl:  ts.URL + "/favicon.ico",
			Title:       "Title",
			Description: "Description",
			ImageUrl:    "http://site.com/images/example.jpg",
			Type:        model.ModelLinkPreview_Page,
		}, info)
	})

	t.Run("html page and find description", func(t *testing.T) {
		ts := newTestServer("text/html", strings.NewReader(tetsHtmlWithoutDescription))
		defer ts.Close()
		lp := New()

		info, err := lp.Fetch(ctx, ts.URL)
		require.NoError(t, err)
		assert.Equal(t, model.ModelLinkPreview{
			Url:         ts.URL,
			FaviconUrl:  ts.URL + "/favicon.ico",
			Title:       "Title",
			Description: "Sed ut perspiciatis, unde omnis iste natus error sit voluptatem accusantium doloremque laudantium, totam rem aperiam eaque ipsa, quae ab illo inventore veritatis et quasi architecto beatae vitae dicta...",
			ImageUrl:    "http://site.com/images/example.jpg",
			Type:        model.ModelLinkPreview_Page,
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
		assert.Equal(t, model.ModelLinkPreview{
			Url:      url,
			Title:    "filename.jpg",
			ImageUrl: url,
			Type:     model.ModelLinkPreview_Image,
		}, info)
	})

	t.Run("binary", func(t *testing.T) {
		tr := testReader(0)
		ts := newTestServer("binary/octed-stream", &tr)
		defer ts.Close()
		url := ts.URL + "/filename.jpg"
		lp := New()
		info, err := lp.Fetch(ctx, url)
		require.NoError(t, err)
		assert.Equal(t, model.ModelLinkPreview{
			Url:   url,
			Title: "filename.jpg",
			Type:  model.ModelLinkPreview_Unknown,
		}, info)
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
Sed ut perspiciatis, unde omnis iste natus error sit voluptatem accusantium doloremque laudantium, totam rem aperiam eaque ipsa, quae ab illo inventore veritatis et quasi architecto beatae vitae dicta sunt, explicabo. Nemo enim ipsam voluptatem, quia voluptas sit, aspernatur aut odit aut fugit, sed quia consequuntur magni dolores eos, qui ratione voluptatem sequi nesciunt, neque porro quisquam est, qui do<b>lorem ipsum</b>, quia <b>dolor sit, amet, consectetur, adipisci</b> v<b>elit, sed</b> quia non numquam <b>eius mod</b>i <b>tempor</b>a <b>incidunt, ut labore et dolore magna</b>m <b>aliqua</b>m quaerat voluptatem. <b>Ut enim ad minim</b>a <b>veniam, quis nostru</b>m <b>exercitation</b>em <b>ullam co</b>rporis suscipit<b> labori</b>o<b>s</b>am, <b>nisi ut aliquid ex ea commod</b>i <b>consequat</b>ur? <b>Quis aute</b>m vel eum <b>iure reprehenderit,</b> qui <b>in</b> ea <b>voluptate velit esse</b>, quam nihil molestiae <b>c</b>onsequatur, vel <b>illum</b>, qui <b>dolore</b>m <b>eu</b>m <b>fugiat</b>, quo voluptas <b>nulla pariatur</b>? At vero eos et accusamus et iusto odio dignissimos ducimus, qui blanditiis praesentium voluptatum deleniti atque corrupti, quos dolores et quas molestias <b>exceptur</b>i <b>sint, obcaecat</b>i <b>cupiditat</b>e <b>non pro</b>v<b>ident</b>, similique <b>sunt in culpa</b>, <b>qui officia deserunt mollit</b>ia <b>anim</b>i, <b>id est laborum</b> et dolorum fuga. Et harum quidem rerum facilis est et expedita distinctio. Nam libero tempore, cum soluta nobis est eligendi optio, cumque nihil impedit, quo minus id, quod maxime placeat, facere possimus, omnis voluptas assumenda est, omnis dolor repellendus. Temporibus autem quibusdam et aut officiis debitis aut rerum necessitatibus saepe eveniet, ut et voluptates repudiandae sint et molestiae non recusandae. Itaque earum rerum hic tenetur a sapiente delectus, ut aut reiciendis voluptatibus maiores alias consequatur aut perferendis doloribus asperiores repellat.
</p></div></body></html>`
