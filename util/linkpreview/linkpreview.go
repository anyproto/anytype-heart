package linkpreview

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/otiai10/opengraph"
)

func New() LinkPreview {
	return &linkPreview{}
}

type LinkType string

const (
	LinkTypeHtml       LinkType = "html"
	LinkTypeImage      LinkType = "image"
	LinkTypeVideo      LinkType = "video"
	LinkTypeText       LinkType = "text"
	LinkTypeUnexpected LinkType = "unexpected"

	// read no more than 400 kb
	maxBytesToRead = 400000
)

type LinkPreview interface {
	Fetch(ctx context.Context, url string) (Info, error)
}

type Info struct {
	Title       string
	Description string
	ImageUrl    string
	Type        LinkType
}

type linkPreview struct{}

func (l *linkPreview) Fetch(ctx context.Context, url string) (Info, error) {
	rt := &proxyRoundTripper{RoundTripper: http.DefaultTransport}
	client := &http.Client{Transport: rt}
	og, err := opengraph.FetchWithContext(ctx, url, client)
	if err != nil {
		if resp := rt.lastResponse; resp != nil && resp.StatusCode == http.StatusOK {
			return l.makeNonHtml(url, resp)
		}
		return Info{}, err
	}
	return l.convertOGToInfo(og), nil
}

func (l *linkPreview) convertOGToInfo(og *opengraph.OpenGraph) (i Info) {
	i = Info{
		Title:       og.Title,
		Description: og.Description,
		Type:        LinkTypeHtml,
	}
	if len(og.Image) != 0 {
		i.ImageUrl = og.Image[0].URL
	}
	return
}

func (l *linkPreview) makeNonHtml(url string, resp *http.Response) (i Info, err error) {
	ct := resp.Header.Get("Content-Type")
	i.Title = filepath.Base(url)
	if strings.HasPrefix(ct, "image/") {
		i.Type = LinkTypeImage
		i.ImageUrl = url
	} else if strings.HasPrefix(ct, "text/") {
		i.Type = LinkTypeText
	} else {
		i.Type = LinkTypeUnexpected
	}
	return
}

type proxyRoundTripper struct {
	http.RoundTripper
	lastResponse *http.Response
}

func (p *proxyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := p.RoundTripper.RoundTrip(req)
	if err == nil {
		p.lastResponse = resp
		resp.Body = http.MaxBytesReader(nil, resp.Body, maxBytesToRead)
	}
	return resp, err
}
