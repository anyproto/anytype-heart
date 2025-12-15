package linkpreview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
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
	// read no more than 10 mb
	maxBytesToRead     = 10 * 1024 * 1024
	maxDescriptionSize = 200

	xRobotsTag = "X-Robots-Tag"
	cspTag     = "Content-Security-Policy"
)

type linkEntry struct {
	genericTitles  []string
	titleSelectors []string
}

var (
	ErrPrivateLink = fmt.Errorf("link is private and cannot be previewed")
	log            = logging.Logger(CName)

	genericTitleCandidates = map[string]linkEntry{
		"www.reddit.com": {
			genericTitles: []string{
				"Reddit - The heart of the internet",
				"Reddit - Dive into anything",
				"Reddit",
			},
			titleSelectors: []string{
				"h1[slot='title']",
				"[data-test-id='post-content'] h3",
				"shreddit-post h1",
				"h1:contains('r/')",
			},
		},
	}
)

func New() LinkPreview {
	return &linkPreview{}
}

type LinkPreview interface {
	Fetch(ctx context.Context, url string, withResponseBody bool) (linkPreview model.LinkPreview, responseBody []byte, isFile bool, err error)
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

func (l *linkPreview) Fetch(
	ctx context.Context, fetchUrl string, withResponseBody bool,
) (linkPreview model.LinkPreview, responseBody []byte, isFile bool, err error) {
	og, rt := buildOpenGraph(ctx, fetchUrl)
	err = og.Fetch()

	resp := rt.lastResponse
	if resp == nil {
		return model.LinkPreview{}, nil, false, fmt.Errorf("no response")
	}

	cspRules, errCheck := checkResponseHeaders(resp)
	if errCheck != nil {
		return model.LinkPreview{}, nil, false, errCheck
	}

	// og.Fetch could fail because of non "text/html" content. Let's try to parse file content
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
	applyCSPRules(cspRules, &res)

	var decodedResponse []byte
	if withResponseBody {
		decodedResponse, err = decodeResponse(rt)
		if err != nil {
			log.Errorf("failed to decode request %s", err)
		}
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

	replaceGenericTitle(&i, rt.lastBody)

	if len(og.Image) != 0 {
		url, err := uri.NormalizeURI(og.Image[0].URL)
		if err == nil {
			i.ImageUrl = url
		}
	}

	if len(i.Description) == 0 {
		i.Description = l.findContent(rt.lastBody)
	}
	if !utf8.ValidString(i.Description) {
		i.Description = ""
	}
	i.Description = text.TruncateEllipsized(i.Description, maxDescriptionSize)
	if !utf8.ValidString(i.Title) {
		i.Title = ""
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
	return content
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

func checkResponseHeaders(resp *http.Response) (cspRules []string, err error) {
	if resp == nil {
		return nil, fmt.Errorf("response is nil")
	}

	xRobotsDirectives := resp.Header.Get(xRobotsTag)
	if xRobotsDirectives != "" {
		err = parseXRobotsTag(xRobotsDirectives)
		if err != nil {
			return nil, err
		}
	}

	cspDirectives := resp.Header.Get(cspTag)
	if cspDirectives != "" {
		cspRules = parseCSPTag(cspDirectives)
	}
	return
}

// parsing tag according https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/X-Robots-Tag#syntax
func parseXRobotsTag(value string) error {
	directives := strings.Split(value, ",")
	for _, directive := range directives {
		parts := strings.Split(directive, ":")
		parts = strings.Split(strings.TrimSpace(parts[len(parts)-1]), " ")
		for _, part := range parts {
			if strings.ToLower(part) == "none" {
				return errors.Join(ErrPrivateLink, fmt.Errorf("private link detected due to %s header", xRobotsTag))
			}
		}
	}
	return nil
}

// parsing tag according https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Content-Security-Policy#syntax
func parseCSPTag(value string) (cspRules []string) {
	for _, directive := range strings.Split(value, ";") {
		directive = strings.ToLower(directive)
		parts := strings.Split(strings.TrimSpace(directive), " ")
		if len(parts) < 2 {
			continue
		}
		switch parts[0] {
		case "default-src":
			if len(cspRules) == 0 {
				cspRules = parts[1:]
			}
		case "img-src":
			cspRules = parts[1:]
		}
	}
	return cspRules
}

func applyCSPRules(cspRules []string, preview *model.LinkPreview) {
	if len(cspRules) == 0 {
		return
	}

	validate, err := buildValidator(cspRules, preview.Url)
	if err != nil {
		log.Errorf("failed to validate CSP rules: %v", err)
		return
	}

	if !validate(preview.ImageUrl) {
		preview.ImageUrl = ""
	}

	if !validate(preview.FaviconUrl) {
		preview.FaviconUrl = ""
	}
}

func buildValidator(cspRules []string, originUrl string) (validate func(string) bool, err error) {
	var (
		allowedSchemes = make(map[string]bool)
		hostPatterns   []string
	)

	originParsed, err := url.Parse(originUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse origin URL: %w", err)
	}

	for _, rule := range cspRules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}

		switch {
		case rule == "'self'":
			hostPatterns = append(hostPatterns, regexp.QuoteMeta(originParsed.Host))
		case rule == "*":
			return func(string) bool { return true }, nil // wildcard
		case rule == "'none'":
			return func(string) bool { return false }, nil
		case strings.HasSuffix(rule, ":"):
			scheme := strings.TrimSuffix(rule, ":") // Scheme (data: blob: https:)
			allowedSchemes[scheme] = true
		case strings.HasPrefix(rule, "*."):
			domain := strings.TrimPrefix(rule, "*.")
			pattern := ".*\\." + regexp.QuoteMeta(domain)
			hostPatterns = append(hostPatterns, pattern, regexp.QuoteMeta(domain))
		default:
			hostPatterns = append(hostPatterns, regexp.QuoteMeta(rule))
		}
	}

	var hostRegex *regexp.Regexp
	if len(hostPatterns) > 0 {
		pattern := "^(" + strings.Join(hostPatterns, "|") + ")$"
		hostRegex, err = regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to compile host regex: %w", err)
		}
	}

	return func(linkToValidate string) bool {
		linkURL, err := url.Parse(linkToValidate)
		if err != nil {
			return false
		}

		if allowedSchemes[linkURL.Scheme] {
			return true
		}

		if hostRegex != nil && hostRegex.MatchString(linkURL.Host) {
			return true
		}

		return len(cspRules) == 0
	}, nil
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

func replaceGenericTitle(preview *model.LinkPreview, htmlContent []byte) {
	if len(htmlContent) == 0 {
		return
	}

	parsedURL, err := url.Parse(preview.Url)
	if err != nil {
		return
	}
	hostname := parsedURL.Hostname()

	var selectors []string
	isTitleGeneric := func() bool {
		candidate, found := genericTitleCandidates[hostname]
		if !found {
			return false
		}
		for _, genericTitle := range candidate.genericTitles {
			if strings.EqualFold(preview.Title, genericTitle) || strings.Contains(preview.Title, genericTitle) {
				selectors = candidate.titleSelectors
				return true
			}
		}
		return false
	}

	if !isTitleGeneric() {
		return
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(htmlContent))
	if err != nil {
		return
	}

	for _, selector := range selectors {
		if title := doc.Find(selector).First().Text(); title != "" {
			title = text.TruncateEllipsized(strings.TrimSpace(title), 100)
			if len(title) > 5 {
				preview.Title = title
				return
			}
		}
	}
}
