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
)

type Validator interface {
	Validate(string) error
}

type Parser interface {
	ParseURI(string) *url.URL
}

type Normalizer interface {
	NormalizeURI(string) string
}

type Manager struct {
	Validator
	Parser
	Normalizer
}

var URIManager Manager

func (m Manager) Validate(uri string) error {
	uri = strings.TrimSpace(uri)

	if len(uri) == 0 {
		return fmt.Errorf("url is empty")
	} else if winFilepathPrefixRegex.MatchString(uri) {
		return fmt.Errorf("filepath not supported")
	} else if strings.HasPrefix(uri, string(os.PathSeparator)) || strings.HasPrefix(uri, ".") {
		return fmt.Errorf("filepath not supported")
	}

	_, err := url.Parse(uri)
	return err
}

func (m Manager) ParseURI(uri string) *url.URL {
	u, _ := url.Parse(uri)
	return u
}

func (m Manager) NormalizeURI(uri string) string {
	if noPrefixEmailRegexp.MatchString(uri) {
		return "mailto:" + uri
	} else if noPrefixTelRegexp.MatchString(uri) {
		return "tel:" + uri
	} else if noPrefixHttpRegex.MatchString(uri) {
		return "http://" + uri
	}
	return uri
}

func (m Manager) ValidateAndParseURI(uri string) (*url.URL, error) {
	uri = strings.TrimSpace(uri)

	if len(uri) == 0 {
		return nil, fmt.Errorf("url is empty")
	} else if winFilepathPrefixRegex.MatchString(uri) {
		return nil, fmt.Errorf("filepath not supported")
	} else if strings.HasPrefix(uri, string(os.PathSeparator)) || strings.HasPrefix(uri, ".") {
		return nil, fmt.Errorf("filepath not supported")
	}

	_, err := url.Parse(uri)
	return nil, err
}

func (m Manager) ValidateAndNormalizeURI(uri string) (string, error) {
	err := m.Validate(uri)
	if err != nil {
		return "", err
	}
	return m.NormalizeURI(uri), nil
}
