package linkpreview

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

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
	Fetch(ctx context.Context, url string) (model.ModelLinkPreview, error)
}

type linkPreview struct {
	bmPolicy *bluemonday.Policy
}

func (l *linkPreview) Fetch(ctx context.Context, url string) (model.ModelLinkPreview, error) {
	rt := &proxyRoundTripper{RoundTripper: http.DefaultTransport}
	client := &http.Client{Transport: rt}
	og, err := opengraph.FetchWithContext(ctx, url, client)
	if err != nil {
		if resp := rt.lastResponse; resp != nil && resp.StatusCode == http.StatusOK {
			return l.makeNonHtml(url, resp)
		}
		return model.ModelLinkPreview{}, err
	}
	res := l.convertOGToInfo(og)
	if len(res.Description) == 0 {
		res.Description = l.findContent(rt.lastBody)
	}
	return res, nil
}

func (l *linkPreview) convertOGToInfo(og *opengraph.OpenGraph) (i model.ModelLinkPreview) {
	og.ToAbsURL()
	i = model.ModelLinkPreview{
		Url:         og.URL.String(),
		Title:       og.Title,
		Description: og.Description,
		Type:        model.ModelLinkPreview_Page,
		FaviconUrl:  og.Favicon,
	}
	if len(og.Image) != 0 {
		i.ImageUrl = og.Image[0].URL
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

func (l *linkPreview) makeNonHtml(url string, resp *http.Response) (i model.ModelLinkPreview, err error) {
	ct := resp.Header.Get("Content-Type")
	i.Url = url
	i.Title = filepath.Base(url)
	if strings.HasPrefix(ct, "image/") {
		i.Type = model.ModelLinkPreview_Image
		i.ImageUrl = url
	} else if strings.HasPrefix(ct, "text/") {
		i.Type = model.ModelLinkPreview_Text
	} else {
		i.Type = model.ModelLinkPreview_Unknown
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
