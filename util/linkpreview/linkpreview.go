package linkpreview

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-middleware/util/uri"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/mauidude/go-readability"
	"github.com/microcosm-cc/bluemonday"
	"github.com/otiai10/opengraph"
)

func New() LinkPreview {
	return &linkPreview{bmPolicy: bluemonday.NewPolicy().AddSpaceWhenStrippingTag(true)}
}

const (
	// read no more than 400 kb
	maxBytesToRead     = 400000
	maxDescriptionSize = 200
)

type LinkPreview interface {
	Fetch(ctx context.Context, url string) (model.LinkPreview, error)
}

type linkPreview struct {
	bmPolicy *bluemonday.Policy
}

func (l *linkPreview) Fetch(ctx context.Context, fetchUrl string) (model.LinkPreview, error) {
	rt := &proxyRoundTripper{RoundTripper: http.DefaultTransport}
	client := &http.Client{Transport: rt}
	og, err := opengraph.FetchWithContext(ctx, fetchUrl, client)
	if err != nil {
		if resp := rt.lastResponse; resp != nil && resp.StatusCode == http.StatusOK {
			return l.makeNonHtml(fetchUrl, resp)
		}
		return model.LinkPreview{}, err
	}
	res := l.convertOGToInfo(og)
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

func (l *linkPreview) convertOGToInfo(og *opengraph.OpenGraph) (i model.LinkPreview) {
	og.ToAbsURL()
	i = model.LinkPreview{
		Url:         og.URL.String(),
		Title:       og.Title,
		Description: og.Description,
		Type:        model.LinkPreview_Page,
		FaviconUrl:  og.Favicon,
	}
	if len(i.FaviconUrl) == 0 {
		og.URL.Path = "favicon.ico"
		og.URL.RawQuery = ""
		i.FaviconUrl = og.URL.String()
	}

	if len(og.Image) != 0 {
		url, err := uri.ProcessURI(og.Image[0].URL)
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
	doc, err := readability.NewDocument(string(data))
	if err != nil {
		return
	}
	content = doc.Content()
	content = strings.TrimSpace(l.bmPolicy.Sanitize(content))
	content = strings.Join(strings.Fields(content), " ") // removes repetitive whitespaces
	if utf8.RuneCountInString(content) > maxDescriptionSize {
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
	pUrl, e := url.Parse(fetchUrl)
	if e == nil {
		pUrl.Path = "favicon.ico"
		pUrl.RawQuery = ""
		i.FaviconUrl = pUrl.String()
	}
	return
}

type proxyRoundTripper struct {
	http.RoundTripper
	lastResponse *http.Response
	lastBody     []byte
}

func (p *proxyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
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
