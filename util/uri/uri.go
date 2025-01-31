package uri

import (
	"fmt"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// RFC 5322 mail regex
	noPrefixEmailRegexp = regexp.MustCompile(`^(?:[a-z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+(?:\.[a-z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+)*|"(?:[\x01-\x08\x0b\x0c\x0e-\x1f\x21\x23-\x5b\x5d-\x7f]|\\[\x01-\x09\x0b\x0c\x0e-\x7f])*")@(?:(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]*[a-z0-9])?|\[(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?|[a-z0-9-]*[a-z0-9]:(?:[\x01-\x08\x0b\x0c\x0e-\x1f\x21-\x5a\x53-\x7f]|\\[\x01-\x09\x0b\x0c\x0e-\x7f])+)\])$`)
	// RFC 3966 tel regex
	noPrefixTelRegexp      = regexp.MustCompile(`^((?:\+[\d().-]*\d[\d().-]*|[0-9A-F*#().-]*[0-9A-F*#][0-9A-F*#().-]*(?:;[a-z\d-]+(?:=(?:[a-z\d\[\]\/:&+$_!~*'().-]|%[\dA-F]{2})+)?)*;phone-context=(?:\+[\d().-]*\d[\d().-]*|(?:[a-z0-9]\.|[a-z0-9][a-z0-9-]*[a-z0-9]\.)*(?:[a-z]|[a-z][a-z0-9-]*[a-z0-9])))(?:;[a-z\d-]+(?:=(?:[a-z\d\[\]\/:&+$_!~*'().-]|%[\dA-F]{2})+)?)*(?:,(?:\+[\d().-]*\d[\d().-]*|[0-9A-F*#().-]*[0-9A-F*#][0-9A-F*#().-]*(?:;[a-z\d-]+(?:=(?:[a-z\d\[\]\/:&+$_!~*'().-]|%[\dA-F]{2})+)?)*;phone-context=\+[\d().-]*\d[\d().-]*)(?:;[a-z\d-]+(?:=(?:[a-z\d\[\]\/:&+$_!~*'().-]|%[\dA-F]{2})+)?)*)*)$`)
	noPrefixHttpRegex      = regexp.MustCompile(`^[\pL\d.-]+(?:\.[\pL\\d.-]+)+[\pL\-\._~:/?#[\]@!\$&'\(\)\*\+,;=.\/\d]+$`)
	winFilepathPrefixRegex = regexp.MustCompile(`^[a-zA-Z]:[\\\/]`)

	// errors
	errURLEmpty             = fmt.Errorf("url is empty")
	ErrFilepathNotSupported = fmt.Errorf("filepath not supported")
)

func excludePathAndEmptyURIs(uri string) error {
	switch {
	case len(uri) == 0:
		return errURLEmpty
	case winFilepathPrefixRegex.MatchString(uri):
		return ErrFilepathNotSupported
	case strings.HasPrefix(uri, string(os.PathSeparator)):
		return ErrFilepathNotSupported
	case strings.HasPrefix(uri, "."):
		return ErrFilepathNotSupported
	}

	return nil
}

func normalizeURI(uri string) string {
	switch {
	case noPrefixEmailRegexp.MatchString(uri):
		return "mailto:" + uri
	case noPrefixTelRegexp.MatchString(uri):
		return "tel:" + uri
	case noPrefixHttpRegex.MatchString(uri):
		return "http://" + uri
	}

	return uri
}

func ValidateURI(uri string) error {
	uri = strings.TrimSpace(uri)
	if err := excludePathAndEmptyURIs(uri); err != nil {
		return err
	}

	_, err := url.Parse(uri)
	return err
}

func ParseURI(uri string) (*url.URL, error) {
	uri = strings.TrimSpace(uri)
	if err := excludePathAndEmptyURIs(uri); err != nil {
		return nil, err
	}

	return url.Parse(uri)
}

func NormalizeURI(uri string) (string, error) {
	if err := ValidateURI(uri); err != nil {
		return "", err
	}

	return normalizeURI(uri), nil
}

func NormalizeAndParseURI(uri string) (*url.URL, error) {
	uri = strings.TrimSpace(uri)
	if err := excludePathAndEmptyURIs(uri); err != nil {
		return nil, err
	}

	return url.Parse(normalizeURI(uri))
}

var preferredExtensions = map[string]string{
	"image/jpeg": ".jpeg",
	"audio/mpeg": ".mp3",
	// Add more preferred mappings if needed
}

func GetFileNameFromURLAndContentType(u *url.URL, contentType string) string {
	var host string
	if u != nil {

		lastSegment := filepath.Base(u.Path)
		// Determine if this looks like a real filename. We'll say it's real if it has a dot or is a hidden file starting with a dot.
		if lastSegment == "." || lastSegment == "" || (!strings.HasPrefix(lastSegment, ".") && !strings.Contains(lastSegment, ".")) {
			// Not a valid filename
			lastSegment = ""
		}

		if lastSegment != "" {
			// A plausible filename was found directly in the URL
			return lastSegment
		}

		// No filename, fallback to host-based
		host = strings.TrimPrefix(u.Hostname(), "www.")
		host = strings.ReplaceAll(host, ".", "_")
		if host == "" {
			host = "file"
		}
	}

	// Try to get a preferred extension for the content type
	var ext string
	if preferred, ok := preferredExtensions[contentType]; ok {
		ext = preferred
	} else {
		extensions, err := mime.ExtensionsByType(contentType)
		if err != nil || len(extensions) == 0 {
			// Fallback if no known extension
			extensions = []string{".bin"}
		}
		ext = extensions[0]
	}

	// Determine a base name from content type
	base := "file"
	if strings.HasPrefix(contentType, "image/") {
		base = "image"
	} else if strings.HasPrefix(contentType, "audio/") {
		base = "audio"
	} else if strings.HasPrefix(contentType, "video/") {
		base = "video"
	}

	var res strings.Builder
	if host != "" {
		res.WriteString(host)
		res.WriteString("_")
	}
	res.WriteString(base)
	if ext != "" {
		res.WriteString(ext)
	}
	return res.String()
}
