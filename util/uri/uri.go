package uri

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

var (
	// RFC 5322 mail regex
	noPrefixEmailRegexp = regexp.MustCompile(`^(?:[a-z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+(?:\.[a-z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+)*|"(?:[\x01-\x08\x0b\x0c\x0e-\x1f\x21\x23-\x5b\x5d-\x7f]|\\[\x01-\x09\x0b\x0c\x0e-\x7f])*")@(?:(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]*[a-z0-9])?|\[(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?|[a-z0-9-]*[a-z0-9]:(?:[\x01-\x08\x0b\x0c\x0e-\x1f\x21-\x5a\x53-\x7f]|\\[\x01-\x09\x0b\x0c\x0e-\x7f])+)\])$`)
	// RFC 3966 tel regex
	noPrefixTelRegexp      = regexp.MustCompile(`^((?:\+[\d().-]*\d[\d().-]*|[0-9A-F*#().-]*[0-9A-F*#][0-9A-F*#().-]*(?:;[a-z\d-]+(?:=(?:[a-z\d\[\]\/:&+$_!~*'().-]|%[\dA-F]{2})+)?)*;phone-context=(?:\+[\d().-]*\d[\d().-]*|(?:[a-z0-9]\.|[a-z0-9][a-z0-9-]*[a-z0-9]\.)*(?:[a-z]|[a-z][a-z0-9-]*[a-z0-9])))(?:;[a-z\d-]+(?:=(?:[a-z\d\[\]\/:&+$_!~*'().-]|%[\dA-F]{2})+)?)*(?:,(?:\+[\d().-]*\d[\d().-]*|[0-9A-F*#().-]*[0-9A-F*#][0-9A-F*#().-]*(?:;[a-z\d-]+(?:=(?:[a-z\d\[\]\/:&+$_!~*'().-]|%[\dA-F]{2})+)?)*;phone-context=\+[\d().-]*\d[\d().-]*)(?:;[a-z\d-]+(?:=(?:[a-z\d\[\]\/:&+$_!~*'().-]|%[\dA-F]{2})+)?)*)*)$`)
	noPrefixHttpRegex      = regexp.MustCompile(`^[\pL\d.-]+(?:\.[\pL\\d.-]+)+[\pL\-\._~:/?#[\]@!\$&'\(\)\*\+,;=.\/\d]+$`)
	haveUriSchemeRegex     = regexp.MustCompile(`^([a-zA-Z][A-Za-z0-9+.-]*):[\S]+`)
	winFilepathPrefixRegex = regexp.MustCompile(`^[a-zA-Z]:[\\\/]`)

	// errors
	errURLEmpty             = fmt.Errorf("url is empty")
	errFilepathNotSupported = fmt.Errorf("filepath not supported")
)

func excludePathAndEmptyURIs(uri string) error {
	switch {
	case len(uri) == 0:
		return errURLEmpty
	case winFilepathPrefixRegex.MatchString(uri):
		return errFilepathNotSupported
	case strings.HasPrefix(uri, string(os.PathSeparator)):
		return errFilepathNotSupported
	case strings.HasPrefix(uri, "."):
		return errFilepathNotSupported
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
