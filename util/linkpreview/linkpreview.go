package linkpreview

import (
	"bytes"
	"context"
	"github.com/anyproto/anytype-heart/util/text"
	"github.com/go-shiori/go-readability"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/anyproto/anytype-heart/util/uri"
	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/microcosm-cc/bluemonday"
	"github.com/otiai10/opengraph/v2"
)

const CName = "linkpreview"

func New() LinkPreview {
	return &linkPreview{}
}

const (
	// read no more than 400 kb
	maxBytesToRead     = 400000
	maxDescriptionSize = 200
)

type LinkPreview interface {
	Fetch(ctx context.Context, url string) (model.LinkPreview, error)
	app.Component
}

type linkPreview struct {
	bmPolicy *bluemonday.Policy
}

func (l *linkPreview) Init(_ *app.App) (err error) {
	l.bmPolicy = bluemonday.NewPolicy().AddSpaceWhenStrippingTag(true)
	return
}

func (l *linkPreview) Name() (name string) {
	return CName
}

func (l *linkPreview) Fetch(ctx context.Context, fetchUrl string) (model.LinkPreview, error) {
	rt := &proxyRoundTripper{RoundTripper: http.DefaultTransport}
	client := &http.Client{Transport: rt}
	og := opengraph.New(fetchUrl)
	og.URL = fetchUrl
	og.Intent.Context = ctx
	og.Intent.HTTPClient = client
	err := og.Fetch()
	if err != nil {
		if resp := rt.lastResponse; resp != nil && resp.StatusCode == http.StatusOK {
			return l.makeNonHtml(fetchUrl, resp)
		}
		return model.LinkPreview{}, err
	}
	res := l.convertOGToInfo(fetchUrl, og)
	if len(res.Description) == 0 {
		res.Description = l.findContent(rt.lastBody)
	}
	if !utf8.ValidString(res.Title) {
		res.Title = ""
	}
	if !utf8.ValidString(res.Description) {
		res.Description = ""
	}
	return res, nil
}

func (l *linkPreview) convertOGToInfo(fetchUrl string, og *opengraph.OpenGraph) (i model.LinkPreview) {
	og.ToAbs()
	i = model.LinkPreview{
		Url:         fetchUrl,
		Title:       og.Title,
		Description: og.Description,
		Type:        model.LinkPreview_Page,
		FaviconUrl:  og.Favicon.URL,
	}

	if len(og.Image) != 0 {
		url, err := uri.NormalizeURI(og.Image[0].URL)
		if err == nil {
			i.ImageUrl = url
		}
	}

	return
}

func (l *linkPreview) findContent(data []byte) (content string) {
	defer func() {
		if e := recover(); e != nil {
			// ignore possible panic while html parsing
		}
	}()

	article, err := readability.FromReader(bytes.NewReader(data), nil)
	if err != nil {
		return
	}
	content = article.TextContent
	content = strings.TrimSpace(l.bmPolicy.Sanitize(content))
	content = strings.Join(strings.Fields(content), " ") // removes repetitive whitespaces
	if text.UTF16RuneCountString(content) > maxDescriptionSize {
		content = string([]rune(content)[:maxDescriptionSize]) + "..."
	}
	return
}

func (l *linkPreview) makeNonHtml(fetchUrl string, resp *http.Response) (i model.LinkPreview, err error) {
	ct := resp.Header.Get("Content-Type")
	i.Url = fetchUrl
	i.Title = filepath.Base(fetchUrl)
	if strings.HasPrefix(ct, "image/") {
		i.Type = model.LinkPreview_Image
		i.ImageUrl = fetchUrl
	} else if strings.HasPrefix(ct, "text/") {
		i.Type = model.LinkPreview_Text
	} else {
		i.Type = model.LinkPreview_Unknown
	}
	pURL, e := uri.ParseURI(fetchUrl)
	if e == nil {
		pURL.Path = "favicon.ico"
		pURL.RawQuery = ""
		i.FaviconUrl = pURL.String()
	}
	return
}

type proxyRoundTripper struct {
	http.RoundTripper
	lastResponse *http.Response
	lastBody     []byte
}

func (p *proxyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")
	resp, err := p.RoundTripper.RoundTrip(req)
	if err == nil {
		p.lastResponse = resp
		resp.Body = &limitReader{ReadCloser: resp.Body, rt: p}
	}
	return resp, err
}

type limitReader struct {
	rt     *proxyRoundTripper
	nTotal int
	io.ReadCloser
}

func (l *limitReader) Read(p []byte) (n int, err error) {
	if l.nTotal > maxBytesToRead {
		return 0, io.EOF
	}
	n, err = l.ReadCloser.Read(p)
	if err == nil || err == io.EOF {
		l.rt.lastBody = append(l.rt.lastBody, p[:n]...)
	}
	l.nTotal += n
	return
}
