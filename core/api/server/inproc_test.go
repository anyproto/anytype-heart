package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// echoEngine records what the engine saw of one request and answers with
// a fixed status, header and body.
func echoEngine(t *testing.T, seen *http.Request) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Any("/echo", func(c *gin.Context) {
		*seen = *c.Request
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		c.Header("X-Echo", "yes")
		c.String(http.StatusTeapot, "got %s %s body=%q", c.Request.Method, c.Request.URL.Path, string(body))
	})
	return engine
}

func inProcessClient(resolve EngineResolver) *http.Client {
	return &http.Client{Transport: NewInProcessTransport(resolve)}
}

func TestInProcessTransport(t *testing.T) {
	t.Run("serves the request through the engine and returns its response whole", func(t *testing.T) {
		// given
		var seen http.Request
		engine := echoEngine(t, &seen)
		client := inProcessClient(func() (http.Handler, error) { return engine, nil })
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, InProcessBaseURL+"/echo?q=1", strings.NewReader(`{"a":1}`))
		require.NoError(t, err)

		// when
		resp, err := client.Do(req)

		// then
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusTeapot, resp.StatusCode)
		assert.Equal(t, "yes", resp.Header.Get("X-Echo"))
		assert.Equal(t, `got POST /echo body="{\"a\":1}"`, string(body))
		assert.Equal(t, int64(len(body)), resp.ContentLength)
		assert.Equal(t, "1", seen.URL.Query().Get("q"))
	})

	t.Run("stamps the loopback remote address", func(t *testing.T) {
		// given
		var seen http.Request
		engine := echoEngine(t, &seen)
		client := inProcessClient(func() (http.Handler, error) { return engine, nil })

		// when
		resp, err := client.Get(InProcessBaseURL + "/echo")

		// then
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, inProcessRemoteAddr, seen.RemoteAddr)
	})

	t.Run("passes the authorization header through exactly as the client sent it", func(t *testing.T) {
		// given
		var seen http.Request
		engine := echoEngine(t, &seen)
		client := inProcessClient(func() (http.Handler, error) { return engine, nil })
		req, err := http.NewRequest(http.MethodGet, InProcessBaseURL+"/echo", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer caller-key")

		// when
		resp, err := client.Do(req)

		// then
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, "Bearer caller-key", seen.Header.Get("Authorization"))
	})

	t.Run("injects no credential into a request that carries none", func(t *testing.T) {
		// given — the security property of §3.3: the transport is engine-only
		var seen http.Request
		engine := echoEngine(t, &seen)
		client := inProcessClient(func() (http.Handler, error) { return engine, nil })

		// when
		resp, err := client.Get(InProcessBaseURL + "/echo")

		// then
		require.NoError(t, err)
		resp.Body.Close()
		_, present := seen.Header["Authorization"]
		assert.False(t, present, "the in-process transport must never supply a bearer of its own")
	})

	t.Run("a resolver error surfaces as a transport error", func(t *testing.T) {
		// given
		boom := errors.New("no account running")
		client := inProcessClient(func() (http.Handler, error) { return nil, boom })

		// when
		_, err := client.Get(InProcessBaseURL + "/echo")

		// then
		var ue *url.Error
		require.ErrorAs(t, err, &ue, "http.Client wraps a RoundTrip error in *url.Error, which the wrapper reads as unreachable")
		assert.ErrorIs(t, err, boom)
	})

	t.Run("resolves the engine per request", func(t *testing.T) {
		// given — ReassignAddress swaps the engine; a cached one would be stale
		var seenA, seenB http.Request
		engineA := echoEngine(t, &seenA)
		engineB := echoEngine(t, &seenB)
		current := http.Handler(engineA)
		client := inProcessClient(func() (http.Handler, error) { return current, nil })

		// when
		resp, err := client.Get(InProcessBaseURL + "/echo")
		require.NoError(t, err)
		resp.Body.Close()
		current = engineB
		resp, err = client.Get(InProcessBaseURL + "/echo")
		require.NoError(t, err)
		resp.Body.Close()

		// then
		assert.Equal(t, "/echo", seenA.URL.Path)
		assert.Equal(t, "/echo", seenB.URL.Path)
	})

	t.Run("a response past the cap fails the round trip", func(t *testing.T) {
		// given
		gin.SetMode(gin.TestMode)
		engine := gin.New()
		engine.GET("/big", func(c *gin.Context) {
			c.Status(http.StatusOK)
			chunk := strings.Repeat("x", 1<<20)
			for i := 0; i <= InProcessMaxResponseBytes/len(chunk); i++ {
				if _, err := c.Writer.WriteString(chunk); err != nil {
					return
				}
			}
		})
		client := inProcessClient(func() (http.Handler, error) { return engine, nil })

		// when
		_, err := client.Get(InProcessBaseURL + "/big")

		// then
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInProcessResponseTooLarge)
	})

	t.Run("a handler that writes no status answers 200", func(t *testing.T) {
		// given
		gin.SetMode(gin.TestMode)
		engine := gin.New()
		engine.GET("/empty", func(c *gin.Context) {})
		client := inProcessClient(func() (http.Handler, error) { return engine, nil })

		// when
		resp, err := client.Get(InProcessBaseURL + "/empty")

		// then
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

// TestInProcessTransportRunsTheRealMiddleware sends an unauthenticated
// request through the real router: the origin policy admits the IP-literal
// host and the bearer gate refuses the call — proof that nothing between
// the transport and the service is skipped, and that no key was injected.
func TestInProcessTransportRunsTheRealMiddleware(t *testing.T) {
	// given
	fx := newV2ServerFixture(t)
	engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
	client := inProcessClient(func() (http.Handler, error) { return engine, nil })

	// when
	resp, err := client.Get(InProcessBaseURL + "/v2/spaces")

	// then
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("WWW-Authenticate"))
}
