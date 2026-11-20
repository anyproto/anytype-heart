package offlinenet

import (
	"fmt"
	"net/http"
	"strings"
)

// loopbackOnly wraps an http.RoundTripper and refuses any non-loopback request.
type loopbackOnly struct{ base http.RoundTripper }

func (l loopbackOnly) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil {
		return nil, fmt.Errorf("offline: nil request URL")
	}
	host := req.URL.Hostname()
	if host != "" && host != "localhost" && !strings.HasPrefix(host, "127.") && host != "::1" && host != "::" {
		return nil, fmt.Errorf("offline: non-loopback request to %s rejected", req.URL.Host)
	}
	return l.base.RoundTrip(req)
}

func init() {
	http.DefaultTransport = loopbackOnly{base: http.DefaultTransport}
}
