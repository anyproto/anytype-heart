package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// inProcessClient builds an http.Client over the fixture's engine, the way
// the API component does for the tools host.
func inProcessClient(fx *fixture) *http.Client {
	return &http.Client{Transport: NewInProcessTransport(func() (http.Handler, string, error) {
		return fx.Engine(), fx.InternalKey(), nil
	})}
}

func TestInProcessTransport(t *testing.T) {
	t.Run("whoami round trip through the engine with no listener", func(t *testing.T) {
		// given
		fx := newV2ServerFixture(t)
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		req, err := http.NewRequest("GET", InProcessBaseURL+"/v2/auth/whoami", nil)
		require.NoError(t, err)

		// when
		resp, err := inProcessClient(fx).Do(req)

		// then
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
		var got v2model.WhoamiResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
		assert.Equal(t, "full", got.Scope)
		assert.Equal(t, InternalAppName, got.Key.Name)
	})

	t.Run("the transport's bearer wins over the client's", func(t *testing.T) {
		// given: a client that sends its own (foreign) key
		fx := newV2ServerFixture(t)
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		req, err := http.NewRequest("GET", InProcessBaseURL+"/v2/auth/whoami", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer foreign-key")

		// when
		resp, err := inProcessClient(fx).Do(req)

		// then: no mint was attempted (the mw mock has no expectation and
		// would fail the test), and the answer is the internal session
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("request bodies reach the handler", func(t *testing.T) {
		// given
		fx := newV2ServerFixture(t)
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		req, err := http.NewRequest("POST", InProcessBaseURL+"/v2/validate", strings.NewReader(`{"formatVersion":"2.0","blocks":[]}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		// when
		resp, err := inProcessClient(fx).Do(req)

		// then
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var body struct {
			Issues []any `json:"issues"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Empty(t, body.Issues)
	})

	t.Run("an unknown route is a 404 through the same path", func(t *testing.T) {
		// given
		fx := newV2ServerFixture(t)
		req, err := http.NewRequest("GET", InProcessBaseURL+"/v2/no-such-route", nil)
		require.NoError(t, err)

		// when
		resp, err := inProcessClient(fx).Do(req)

		// then
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("a resolve error is a transport error", func(t *testing.T) {
		// given: no engine (no account running)
		client := &http.Client{Transport: NewInProcessTransport(func() (http.Handler, string, error) {
			return nil, "", errors.New("api engine is not built")
		})}
		req, err := http.NewRequest("GET", InProcessBaseURL+"/v2/spaces", nil)
		require.NoError(t, err)

		// when
		resp, err := client.Do(req)

		// then: http.Client wraps it in *url.Error, which is what the
		// wrapper classifies as "unreachable"
		require.Nil(t, resp)
		var ue *url.Error
		require.ErrorAs(t, err, &ue)
		assert.Contains(t, err.Error(), "api engine is not built")
	})
}
