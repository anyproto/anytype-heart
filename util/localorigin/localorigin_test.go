package localorigin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPolicy_AllowOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		// Absent Origin cannot come from a web page: browsers always attach it
		// on cross-origin fetch/XHR and on every WebSocket handshake.
		{"empty origin from a native client", "", true},

		{"loopback ipv4", "http://127.0.0.1:3030", true},
		{"loopback ipv4 other octet", "http://127.13.37.1:8080", true},
		{"loopback ipv6", "http://[::1]:3030", true},
		{"localhost", "http://localhost:8080", true},
		{"localhost https", "https://localhost", true},
		{"localhost no port", "http://localhost", true},
		{"trailing slash tolerated", "http://localhost:8080/", true},
		{"uppercase scheme and host", "HTTP://LOCALHOST:8080", true},
		{"trailing dot on localhost", "http://localhost.:8080", true},

		// The attacker cases from the report.
		{"remote http origin", "http://evil.com", false},
		{"remote https origin", "https://evil.com", false},
		{"remote origin on the api port", "https://evil.com:31008", false},
		{"opaque origin from sandboxed iframe or data url", "null", false},

		// Hostnames that merely look loopback.
		{"localhost as a subdomain label", "http://localhost.evil.com", false},
		{"loopback ip as a subdomain label", "http://127.0.0.1.evil.com", false},
		{"non-loopback private ip", "http://192.168.1.5:3030", false},
		{"public ip", "http://8.8.8.8", false},

		// file:// is off unless AllowFileOrigin is set.
		{"file origin denied by default", "file://", false},
		{"file origin with a host is never an origin", "file://evil.com", false},

		// Shapes that are not serialized origins.
		{"origin with a path", "http://localhost:8080/evil", false},
		{"origin with userinfo", "http://localhost@evil.com", false},
		{"scheme only", "http://", false},
		{"bare hostname", "localhost", false},
		{"unknown scheme", "chrome-extension://abcdef", false},
		{"javascript scheme", "javascript://localhost", false},
	}

	policy := New("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, policy.AllowOrigin(tt.origin))
		})
	}
}

func TestPolicy_AllowFileOrigin(t *testing.T) {
	// given the packaged Electron renderer, whose WebSocket handshake from a
	// file:// page carries Origin: file://
	policy := New("", AllowFileOrigin())

	assert.True(t, policy.AllowOrigin("file://"))
	assert.True(t, policy.AllowOrigin("FILE://"))
	// A file:// origin is still not a licence for anything else.
	assert.False(t, policy.AllowOrigin("file://evil.com"))
	assert.False(t, policy.AllowOrigin("null"))
}

func TestPolicy_AllowWebclipperExtension(t *testing.T) {
	policy := New("", AllowWebclipperExtension())

	assert.True(t, policy.AllowOrigin("chrome-extension://jbnammhjiplhpjfncnlejjjejghimdkf"))
	assert.True(t, policy.AllowOrigin("chrome-extension://jkmhmgghdjjbafmkgjmplhemjjnkligf"))
	assert.True(t, policy.AllowOrigin("chrome-extension://lcamkcmpcofgmbmloefimnelnjpcdpfn"))
	// Not a licence for any chrome-extension origin, only the Webclipper's own.
	assert.False(t, policy.AllowOrigin("chrome-extension://abcdef"))
	// ...and off unless the option is set.
	assert.False(t, New("").AllowOrigin("chrome-extension://jbnammhjiplhpjfncnlejjjejghimdkf"))
}

func TestPolicy_ExtraAllowedOrigins(t *testing.T) {
	t.Run("comma separated list with whitespace", func(t *testing.T) {
		policy := New(" http://192.168.1.5:3030 , https://app.example.com ")

		assert.True(t, policy.AllowOrigin("http://192.168.1.5:3030"))
		assert.True(t, policy.AllowOrigin("https://app.example.com"))
		assert.False(t, policy.AllowOrigin("http://192.168.1.6:3030"))
	})

	t.Run("empty entries are ignored", func(t *testing.T) {
		policy := New(",,")
		assert.False(t, policy.AllowOrigin("http://evil.com"))
	})

	t.Run("null can never be allowlisted", func(t *testing.T) {
		policy := New("null")
		assert.False(t, policy.AllowOrigin("null"))
	})
}

func TestAllowHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want bool
	}{
		{"loopback ipv4 with port", "127.0.0.1:31008", true},
		{"loopback ipv6 with port", "[::1]:31008", true},
		{"localhost with port", "localhost:31008", true},
		{"bare loopback ip", "127.0.0.1", true},
		{"empty host", "", true},
		// The proxy may be bound to 0.0.0.0 in docker; reaching it by IP is fine
		// because DNS rebinding needs a name it can re-point.
		{"lan ip", "192.168.1.5:31008", true},

		// DNS rebinding: the browser thinks it is same-origin with the attacker.
		{"attacker hostname", "evil.com:31008", false},
		{"hostname resolving to loopback", "localtest.me:31008", false},
		{"localhost as a subdomain label", "localhost.evil.com:31008", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AllowHost(tt.host))
		})
	}
}

func TestPolicy_AllowRequest(t *testing.T) {
	newRequest := func(host string, headers map[string]string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/anytype.ClientCommands/AppGetVersion", nil)
		r.Host = host
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		return r
	}

	tests := []struct {
		name    string
		host    string
		headers map[string]string
		want    bool
	}{
		{
			// Measured against the real Electron runtime: a gRPC-Web POST from
			// the packaged file:// renderer carries no Origin at all.
			name:    "packaged electron renderer sends no origin",
			host:    "127.0.0.1:31008",
			headers: map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "cors"},
			want:    true,
		},
		{
			name:    "native client sends neither origin nor fetch metadata",
			host:    "127.0.0.1:31008",
			headers: nil,
			want:    true,
		},
		{
			name:    "web build on a loopback origin",
			host:    "127.0.0.1:31008",
			headers: map[string]string{"Origin": "http://127.0.0.1:3030", "Sec-Fetch-Site": "cross-site"},
			want:    true,
		},
		{
			name:    "malicious site",
			host:    "127.0.0.1:31008",
			headers: map[string]string{"Origin": "https://evil.com", "Sec-Fetch-Site": "cross-site"},
			want:    false,
		},
		{
			name:    "sandboxed iframe",
			host:    "127.0.0.1:31008",
			headers: map[string]string{"Origin": "null", "Sec-Fetch-Site": "cross-site"},
			want:    false,
		},
		{
			// Rebinding makes the page and the proxy look same-origin, so the
			// Host header is the only thing left that still names the attacker.
			name:    "dns rebinding with an origin header",
			host:    "evil.com:31008",
			headers: map[string]string{"Origin": "http://evil.com:31008", "Sec-Fetch-Site": "same-origin"},
			want:    false,
		},
		{
			// Some browsers omit Origin on same-origin POSTs.
			name:    "dns rebinding without an origin header",
			host:    "evil.com:31008",
			headers: map[string]string{"Sec-Fetch-Site": "same-origin"},
			want:    false,
		},
		{
			// Belt and braces: even if the Host check were bypassed, a browser
			// claiming same-origin without an Origin header is not a real client.
			name:    "same-origin browser request without an origin header",
			host:    "127.0.0.1:31008",
			headers: map[string]string{"Sec-Fetch-Site": "same-origin"},
			want:    false,
		},
		{
			name:    "same-site browser request without an origin header",
			host:    "127.0.0.1:31008",
			headers: map[string]string{"Sec-Fetch-Site": "same-site"},
			want:    false,
		},
	}

	policy := New("", AllowFileOrigin())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, policy.AllowRequest(newRequest(tt.host, tt.headers)))
		})
	}

	t.Run("websocket handshake from the packaged renderer", func(t *testing.T) {
		// given the Origin the real Electron runtime sends on a WS handshake
		r := newRequest("127.0.0.1:31008", map[string]string{
			"Origin":                 "file://",
			"Upgrade":                "websocket",
			"Sec-Websocket-Protocol": "grpc-websockets",
		})

		assert.True(t, policy.AllowRequest(r))
		// ...and the same handshake is refused when file:// is not trusted.
		assert.False(t, New("").AllowRequest(r))
	})
}
