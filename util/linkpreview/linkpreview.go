package linkpreview

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/anyproto/any-sync/app"
	"github.com/go-shiori/go-readability"
	"github.com/microcosm-cc/bluemonday"
	"github.com/otiai10/opengraph/v2"
	"golang.org/x/net/html/charset"

	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/text"
	"github.com/anyproto/anytype-heart/util/uri"
)

const (
	CName       = "linkpreview"
	utfEncoding = "utf-8"
)

func New() LinkPreview {
	return &linkPreview{}
}

const (
	// read no more than 10 mb
	maxBytesToRead     = 10 * 1024 * 1024
	maxDescriptionSize = 200
)

var log = logging.Logger("link-preview")

type LinkPreview interface {
	Fetch(ctx context.Context, url string) (linkPreview model.LinkPreview, responseBody []byte, isFile bool, err error)
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

func (l *linkPreview) Fetch(ctx context.Context, fetchUrl string) (linkPreview model.LinkPreview, responseBody []byte, isFile bool, err error) {
	og, rt := buildOpenGraph(ctx, fetchUrl)
	err = og.Fetch()

	resp := rt.lastResponse
	if resp == nil {
		return model.LinkPreview{}, nil, false, fmt.Errorf("no response")
	}

	if err != nil {
		if resp.StatusCode == http.StatusOK {
			preview, isFile, err := l.makeNonHtml(fetchUrl, resp)
			if err != nil {
				return preview, nil, false, err
			}
			return preview, rt.lastBody, isFile, nil
		}
		sendMetricsEvent(resp.StatusCode)
		return model.LinkPreview{}, nil, false, fmt.Errorf("invalid http code %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		sendMetricsEvent(resp.StatusCode)
		return model.LinkPreview{}, nil, false, fmt.Errorf("invalid http code %d", resp.StatusCode)
	}

	res := l.convertOGToInfo(fetchUrl, og, rt)
	decodedResponse, err := decodeResponse(rt)
	if err != nil {
		log.Errorf("failed to decode request %s", err)
	}
	return res, decodedResponse, false, nil
}

func decodeResponse(response *proxyRoundTripper) ([]byte, error) {
	contentType := response.lastResponse.Header.Get("Content-Type")
	enc, name, _ := charset.DetermineEncoding(response.lastBody, contentType)
	if name == utfEncoding {
		return response.lastBody, nil
	}
	decodedResponse, err := enc.NewDecoder().Bytes(response.lastBody)
	if err != nil {
		return response.lastBody, err
	}
	return decodedResponse, nil
}

func (l *linkPreview) convertOGToInfo(fetchUrl string, og *opengraph.OpenGraph, rt *proxyRoundTripper) (i model.LinkPreview) {
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

	if len(i.Description) == 0 {
		i.Description = l.findContent(rt.lastBody)
	}
	if !utf8.ValidString(i.Title) {
		i.Title = ""
	}
	if !utf8.ValidString(i.Description) {
		i.Description = ""
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

func (l *linkPreview) makeNonHtml(fetchUrl string, resp *http.Response) (i model.LinkPreview, isFile bool, err error) {
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
	isFile = checkFileType(fetchUrl, resp, ct)
	pURL, e := uri.ParseURI(fetchUrl)
	if e == nil {
		pURL.Path = "favicon.ico"
		pURL.RawQuery = ""
		i.FaviconUrl = pURL.String()
	}
	return
}

func checkFileType(url string, resp *http.Response, contentType string) bool {
	ext := filepath.Ext(url)
	mimeType := mime.TypeByExtension(ext)
	return isContentFile(resp, contentType, mimeType)
}

func isContentFile(resp *http.Response, contentType, mimeType string) bool {
	return contentType != "" || strings.Contains(resp.Header.Get("Content-Disposition"), "filename") ||
		mimeType != ""
}

func buildOpenGraph(ctx context.Context, fetchUrl string) (og *opengraph.OpenGraph, rt *proxyRoundTripper) {
	rt = &proxyRoundTripper{RoundTripper: http.DefaultTransport}
	client := &http.Client{Transport: rt}
	og = opengraph.New(fetchUrl)
	og.URL = fetchUrl
	og.Intent.Context = ctx
	og.Intent.HTTPClient = client
	return og, rt
}

type proxyRoundTripper struct {
	http.RoundTripper
	lastResponse *http.Response
	lastBody     []byte
}

func (p *proxyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AnytypeBot/1.0; +https://anytype.io/bot)")
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

func sendMetricsEvent(code int) {
	metrics.Service.SendSampled(&metrics.LinkPreviewStatusEvent{StatusCode: code})
	statusClass := getStatusClass(code)
	metrics.LinkPreviewStatusCounter.WithLabelValues(fmt.Sprintf("%d", code), statusClass).Inc()
}

func getStatusClass(statusCode int) string {
	switch {
	case statusCode >= 100 && statusCode < 200:
		return "1xx"
	case statusCode >= 200 && statusCode < 300:
		return "2xx"
	case statusCode >= 300 && statusCode < 400:
		return "3xx"
	case statusCode >= 400 && statusCode < 500:
		return "4xx"
	case statusCode >= 500 && statusCode < 600:
		return "5xx"
	default:
		return "unknown"
	}
}
