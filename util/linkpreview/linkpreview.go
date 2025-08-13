package linkpreview

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"github.com/anyproto/any-sync/app"
	"github.com/go-shiori/go-readability"
	"github.com/microcosm-cc/bluemonday"
	"github.com/otiai10/opengraph/v2"
	"golang.org/x/net/html/charset"

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

var (
	log = logging.Logger("link-preview")

	genericTitles = map[string][]string{
		"reddit.com": {
			"Reddit - The heart of the internet",
			"Reddit - Dive into anything",
			"Reddit",
		},
		"twitter.com": {
			"Twitter",
			"X",
		},
		"x.com": {
			"Twitter",
			"X",
		},
		"youtube.com": {
			"YouTube",
		},
		"github.com": {
			"GitHub",
		},
		"stackoverflow.com": {
			"Stack Overflow",
		},
	}

	titleSelectors = map[string][]string{
		"reddit.com": {
			"h1[slot='title']",                 // New Reddit
			"h1._eYtD2XCVieq6emjKBH3m",         // New Reddit class
			"h1.s1a5i67p-0",                    // New Reddit
			"p.title a.title",                  // Old Reddit
			"a.title",                          // Old Reddit
			"[data-test-id='post-content'] h3", // Mobile/app
			"shreddit-post h1",                 // Shreddit component
			"h1[data-testid='post-title']",     // React components
			"h1:contains('r/')",                // Generic h1 containing subreddit
		},
		"twitter.com": {
			"[data-testid='tweetText']",
			"[data-testid='tweet'] [lang]",
			".tweet-text",
			".js-tweet-text",
		},
		"x.com": {
			"[data-testid='tweetText']",
			"[data-testid='tweet'] [lang]",
			".tweet-text",
			".js-tweet-text",
		},
		"youtube.com": {
			"meta[name='title']",
			"h1.title yt-formatted-string",
			"h1.ytd-video-primary-info-renderer",
			".watch-main-col h1",
		},
		"github.com": {
			"[data-pjax='#repo-content-pjax-container'] h1",
			".js-issue-title",
			".gh-header-title",
			"h1.public strong",
			"h1 strong a",
		},
		"stackoverflow.com": {
			"h1[data-se-element='title']",
			".question-hyperlink",
			"h1 a",
		},
	}
)

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
	rt := &proxyRoundTripper{RoundTripper: http.DefaultTransport}
	client := &http.Client{Transport: rt}
	og := opengraph.New(fetchUrl)
	og.URL = fetchUrl
	og.Intent.Context = ctx
	og.Intent.HTTPClient = client
	err = og.Fetch()
	if err != nil {
		if resp := rt.lastResponse; resp != nil && resp.StatusCode == http.StatusOK {
			preview, isFile, err := l.makeNonHtml(fetchUrl, resp)
			if err != nil {
				return preview, nil, false, err
			}
			return preview, rt.lastBody, isFile, nil
		}
		return model.LinkPreview{}, nil, false, err
	}

	if resp := rt.lastResponse; resp != nil && resp.StatusCode != http.StatusOK {
		return model.LinkPreview{}, nil, false, fmt.Errorf("invalid http code %d", resp.StatusCode)
	}
	res := l.convertOGToInfo(fetchUrl, og)

	if l.isGenericTitle(res.Title, fetchUrl) {
		if enhancedTitle := l.extractEnhancedTitle(rt.lastBody, fetchUrl); enhancedTitle != "" {
			res.Title = enhancedTitle
		}
	}

	if len(res.Description) == 0 {
		res.Description = l.findContent(rt.lastBody)
	}
	if !utf8.ValidString(res.Title) {
		res.Title = ""
	}
	if !utf8.ValidString(res.Description) {
		res.Description = ""
	}
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

func (l *linkPreview) isGenericTitle(title, fetchUrl string) bool {
	if title == "" {
		return true
	}

	parsedURL, err := url.Parse(fetchUrl)
	if err != nil {
		return false
	}

	hostname := parsedURL.Hostname()

	for domain, titles := range genericTitles {
		if strings.Contains(hostname, domain) {
			for _, genericTitle := range titles {
				if strings.EqualFold(title, genericTitle) || strings.Contains(title, genericTitle) {
					return true
				}
			}
		}
	}

	return false
}

func (l *linkPreview) extractEnhancedTitle(htmlContent []byte, fetchUrl string) string {
	if len(htmlContent) == 0 {
		return ""
	}

	parsedURL, err := url.Parse(fetchUrl)
	if err != nil {
		return ""
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(htmlContent))
	if err != nil {
		return ""
	}

	hostname := parsedURL.Hostname()

	var selectors []string
	switch {
	case strings.Contains(hostname, "reddit.com"):
		selectors = titleSelectors["reddit.com"]
	case strings.Contains(hostname, "twitter.com") || strings.Contains(hostname, "x.com"):
		selectors = titleSelectors["twitter.com"]
	case strings.Contains(hostname, "youtube.com"):
		selectors = titleSelectors["youtube.com"]
	case strings.Contains(hostname, "github.com"):
		selectors = titleSelectors["github.com"]
	case strings.Contains(hostname, "stackoverflow.com"):
		selectors = titleSelectors["stackoverflow.com"]
	default:
		return ""
	}

	for _, selector := range selectors {
		if title := doc.Find(selector).First().Text(); title != "" {
			title = strings.TrimSpace(title)
			if len(title) > 100 {
				title = title[:97] + "..."
			}
			if len(title) > 5 {
				return title
			}
		}
	}

	return ""
}
