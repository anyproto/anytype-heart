package linkpreview

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/otiai10/opengraph"
)

func New() LinkPreview {
	return &linkPreview{}
}

type LinkType string

const (
	// read no more than 400 kb
	maxBytesToRead = 400000
)

type LinkPreview interface {
	Fetch(ctx context.Context, url string) (pb.LinkPreviewResponse, error)
}

type linkPreview struct{}

func (l *linkPreview) Fetch(ctx context.Context, url string) (pb.LinkPreviewResponse, error) {
	rt := &proxyRoundTripper{RoundTripper: http.DefaultTransport}
	client := &http.Client{Transport: rt}
	og, err := opengraph.FetchWithContext(ctx, url, client)
	if err != nil {
		if resp := rt.lastResponse; resp != nil && resp.StatusCode == http.StatusOK {
			return l.makeNonHtml(url, resp)
		}
		return pb.LinkPreviewResponse{}, err
	}
	return l.convertOGToInfo(og), nil
}

func (l *linkPreview) convertOGToInfo(og *opengraph.OpenGraph) (i pb.LinkPreviewResponse) {
	og.ToAbsURL()
	i = pb.LinkPreviewResponse{
		Url:         og.URL.String(),
		Title:       og.Title,
		Description: og.Description,
		Type:        pb.LinkPreviewResponse_PAGE,
		FaviconUrl:  og.Favicon,
	}
	if len(og.Image) != 0 {
		i.ImageUrl = og.Image[0].URL
	}
	return
}

func (l *linkPreview) makeNonHtml(url string, resp *http.Response) (i pb.LinkPreviewResponse, err error) {
	ct := resp.Header.Get("Content-Type")
	i.Url = url
	i.Title = filepath.Base(url)
	if strings.HasPrefix(ct, "image/") {
		i.Type = pb.LinkPreviewResponse_IMAGE
		i.ImageUrl = url
	} else if strings.HasPrefix(ct, "text/") {
		i.Type = pb.LinkPreviewResponse_TEXT
	} else {
		i.Type = pb.LinkPreviewResponse_UNEXPECTED
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
