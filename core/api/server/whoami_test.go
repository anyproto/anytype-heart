package server

// whoami_test.go pins the P1c introspection surface end to end through the
// real engine: the exact whoami bodies for both key kinds, the
// Authorization-header-only credential rule, the plain 401 for unknown
// keys, the legacy-key deprecation signal on both route groups, and — the
// load-bearing one — the anti-drift test proving the whoami mirror and the
// space-grant gate cannot disagree about the same key.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// assertNoRfc9745Headers pins the spec's hardest prohibition: never emit
// RFC 9745 Deprecation or RFC 8594 Sunset. §2.2 scopes those to the RESOURCE
// in the response, so emitting them on /v1 would declare /v1 itself
// deprecated — the opposite of the grandfathering promise. The credential
// signal is Anytype-Key-Status and the date-free Link rel="deprecation".
func assertNoRfc9745Headers(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Empty(t, w.Header().Get("Deprecation"), "RFC 9745 Deprecation is forbidden — it deprecates the RESOURCE, not the credential")
	require.Empty(t, w.Header().Get("Sunset"), "RFC 8594 Sunset is forbidden — no sunset is committed, and it too names the resource")
}

func TestWhoami(t *testing.T) {
	t.Run("scoped key: the exact body, names from the grant-intersected list", func(t *testing.T) {
		// given: two live spaces, the grant covers one — the non-granted
		// space's name must not appear anywhere in the body
		fx := newV2ServerFixture(t)
		registerGrantTestSpace(t, fx, "spaceA", "Work")
		registerGrantTestSpace(t, fx, "spaceB", "Personal")
		fx.KeyToToken = map[string]ApiSessionEntry{
			"scopedKey": {
				Token: "tok", AppName: "Claude Desktop", Scope: model.AccountAuth_JsonAPI,
				Grant:     &util.ApiGrant{Spaces: []string{"spaceA"}, Perms: util.GrantPermsReadWrite},
				KeyId:     "hash1",
				CreatedAt: 1700000000,
			},
		}
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		want := fmt.Sprintf(`{
			"key": {"id":"hash1","name":"Claude Desktop","createdAt":"2023-11-14T22:13:20Z","expiresAt":null},
			"scope": "jsonApi",
			"grant": {"scoped":true,"permission":"readwrite",
			          "spaces":[{"id":"spaceA","name":"Work","permission":"readwrite"}]},
			"api": {"version":%q},
			"keyStatus": "scoped"
		}`, util.ApiVersion)

		// when
		w := serveWithKey(fx, "GET", "/v2/auth/whoami", "scopedKey")

		// then
		require.Equal(t, http.StatusOK, w.Code)
		require.JSONEq(t, want, w.Body.String())
		require.NotContains(t, w.Body.String(), "Personal")
		// the signal headers: status always present, the legacy-only pair absent
		assert.Equal(t, util.KeyStatusScoped, w.Header().Get(util.KeyStatusHeader))
		assert.Empty(t, w.Header().Get(util.NoticeHeader), "the notice header is legacy-only")
		assert.Empty(t, w.Header().Values("Link"), "the deprecation link is legacy-only")
		assertNoRfc9745Headers(t, w)
	})

	t.Run("legacy key: scoped false, spaces [], permission null, the signal in the body", func(t *testing.T) {
		// given: a nil-grant key. spaces MUST be [] and scoped an explicit
		// false — spaces:null would eventually be misread fail-open.
		fx := newV2ServerFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{
			"legacyKey": {
				Token: "tok", AppName: "old-script", Scope: model.AccountAuth_JsonAPI,
				KeyId:    "hash2",
				ExpireAt: 1900000000,
			},
		}
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		want := fmt.Sprintf(`{
			"key": {"id":"hash2","name":"old-script","createdAt":null,"expiresAt":"2030-03-17T17:46:40Z"},
			"scope": "jsonApi",
			"grant": {"scoped":false,"permission":null,"spaces":[]},
			"api": {"version":%q},
			"keyStatus": "legacy",
			"notice": %q
		}`, util.ApiVersion, util.LegacyKeyNotice)

		// when
		w := serveWithKey(fx, "GET", "/v2/auth/whoami", "legacyKey")

		// then
		require.Equal(t, http.StatusOK, w.Code)
		require.JSONEq(t, want, w.Body.String())
		// the raw JSON must carry the empty ARRAY, not null — JSONEq treats
		// them as different already, but pin the bytes to be explicit
		require.Contains(t, w.Body.String(), `"spaces":[]`)
		assert.Equal(t, util.KeyStatusLegacy, w.Header().Get(util.KeyStatusHeader))
		assert.Equal(t, util.LegacyKeyNotice, w.Header().Get(util.NoticeHeader))
		assert.Equal(t, []string{util.KeyDeprecationLink}, w.Header().Values("Link"))
		assertNoRfc9745Headers(t, w)
	})

	t.Run("a Full key is nil-grant but gets no remedial signal", func(t *testing.T) {
		// given: a Full-scope credential passes the /v2 scope gate but can
		// NEVER carry a grant (wallet.ValidateAppLinkGrant requires JsonAPI),
		// so the "re-issue as a scoped key" advice is impossible to follow —
		// the status stays legacy, the notice and the Link never appear
		fx := newV2ServerFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{
			"fullKey": {Token: "tok", AppName: "desktop", Scope: model.AccountAuth_Full, KeyId: "hashFull"},
		}
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		// when
		w := serveWithKey(fx, "GET", "/v2/auth/whoami", "fullKey")

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, util.KeyStatusLegacy, w.Header().Get(util.KeyStatusHeader))
		assert.Empty(t, w.Header().Get(util.NoticeHeader), "the notice is JsonAPI-only")
		assert.Empty(t, w.Header().Values("Link"), "the deprecation link is JsonAPI-only")
		assert.Contains(t, w.Body.String(), `"keyStatus":"legacy"`)
		assert.NotContains(t, w.Body.String(), `"notice"`, "the body notice is JsonAPI-only")
	})

	t.Run("the token is never accepted as a parameter", func(t *testing.T) {
		// given: a key that IS valid when presented in the Authorization
		// header — presented anywhere else it must count for nothing, or
		// whoami becomes the enumeration oracle RFC 7662 §4 warns about
		fx := newV2ServerFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{
			"validKey": {Token: "tok", Scope: model.AccountAuth_JsonAPI},
		}

		for _, probe := range []struct{ name, target, body string }{
			{"query key", "/v2/auth/whoami?key=validKey", ""},
			{"query token", "/v2/auth/whoami?token=validKey", ""},
			{"query access_token", "/v2/auth/whoami?access_token=validKey", ""},
			{"body token", "/v2/auth/whoami", `{"token":"validKey"}`},
		} {
			t.Run(probe.name, func(t *testing.T) {
				w := httptest.NewRecorder()
				req := httptest.NewRequest("GET", probe.target, strings.NewReader(probe.body))
				req.Host = localApiHost
				fx.Engine().ServeHTTP(w, req)

				require.Equal(t, http.StatusUnauthorized, w.Code)
				require.Equal(t, `Bearer realm="anytype"`, w.Header().Get("WWW-Authenticate"))
				require.NotContains(t, w.Body.String(), "grant")
			})
		}
	})

	t.Run("an unknown or revoked key gets the plain 401", func(t *testing.T) {
		// given: the shared auth middleware answers before the handler —
		// uniform across every /v2 route, no whoami-specific shape, no
		// RFC 7662 active:false body, no signal headers
		fx := newV2ServerFixture(t)
		fx.mwMock.On("WalletCreateSession", mock.Anything, mock.Anything).
			Return(&pb.RpcWalletCreateSessionResponse{
				Error: &pb.RpcWalletCreateSessionResponseError{
					Code: pb.RpcWalletCreateSessionResponseError_APP_TOKEN_NOT_FOUND_IN_THE_CURRENT_ACCOUNT,
				},
			}, nil).Once()

		// when
		w := serveWithKey(fx, "GET", "/v2/auth/whoami", "revokedKey")

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
		expectedJSON, err := json.Marshal(util.CodeToApiError(http.StatusUnauthorized, ErrInvalidApiKey.Error()))
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), w.Body.String())
		require.Equal(t, `Bearer realm="anytype", error="invalid_token"`, w.Header().Get("WWW-Authenticate"))
		require.Empty(t, w.Header().Get(util.KeyStatusHeader), "no credential, no credential-status signal")
		require.NotContains(t, w.Body.String(), "scoped")
	})
}

func TestWhoamiAgreesWithTheGate(t *testing.T) {
	// The anti-drift test: whoami is a MIRROR of the same grant record the
	// gate enforces, and this test makes disagreement a failure. The
	// expectations for the gate probes are derived ONLY from the whoami
	// body — if the mirror ever comes from a second derivation path, the
	// gate's answers stop matching it here.
	for _, tc := range []struct {
		name  string
		grant *util.ApiGrant
	}{
		{"read-only grant", &util.ApiGrant{Spaces: []string{"spaceA"}, Perms: util.GrantPermsRead}},
		{"readwrite grant", &util.ApiGrant{Spaces: []string{"spaceA"}, Perms: util.GrantPermsReadWrite}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			fx := newV2ServerFixture(t)
			registerGrantTestSpace(t, fx, "spaceA", "Work")
			registerGrantTestSpace(t, fx, "spaceB", "Personal")
			grantedSession(fx, "scopedKey", tc.grant)
			fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

			// when: ask the mirror first
			whoamiResp := serveWithKey(fx, "GET", "/v2/auth/whoami", "scopedKey")
			require.Equal(t, http.StatusOK, whoamiResp.Code)
			var mirror v2model.WhoamiResponse
			require.NoError(t, json.Unmarshal(whoamiResp.Body.Bytes(), &mirror))
			require.True(t, mirror.Grant.Scoped)
			require.NotNil(t, mirror.Grant.Permission)

			claimed := map[string]bool{}
			for _, space := range mirror.Grant.Spaces {
				claimed[space.Id] = true
			}

			// then: the gate must agree, space by space, in both directions
			for _, spaceId := range []string{"spaceA", "spaceB"} {
				w := serveWithKey(fx, "GET", "/v2/spaces/"+spaceId, "scopedKey")
				if claimed[spaceId] {
					require.Equal(t, http.StatusOK, w.Code,
						"whoami claims %s is granted — the gate must serve it", spaceId)
				} else {
					require.Equal(t, http.StatusForbidden, w.Code,
						"whoami omits %s — the gate must refuse it", spaceId)
					require.Contains(t, w.Body.String(), `"space_not_granted"`)
				}
			}

			// and the verb: a write probe on a granted space must be refused
			// exactly when the mirror says the permission is read-only
			write := serveWithKeyBody(fx, "POST", "/v2/spaces/spaceA/objects", "scopedKey", `{}`)
			if *mirror.Grant.Permission == util.GrantPermsReadWrite {
				require.NotEqual(t, http.StatusForbidden, write.Code,
					"whoami claims readwrite — the gate must not refuse the write")
			} else {
				require.Equal(t, http.StatusForbidden, write.Code,
					"whoami claims read-only — the gate must refuse the write")
				require.Contains(t, write.Body.String(), `"write_not_granted"`)
			}
		})
	}
}

func TestLegacyKeySignals(t *testing.T) {
	t.Run("the signal rides /v1 responses too", func(t *testing.T) {
		// given: legacy keys live on /v1 — the signal must reach them there,
		// not only on the /v2 surface they never call
		fx := newFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{
			"legacyKey": {Token: "tok", AppName: "legacy", Scope: model.AccountAuth_JsonAPI, KeyId: "hash3"},
		}
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}, nil).Once()
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		// when
		w := serveWithKey(fx, "GET", "/v1/spaces", "legacyKey")

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, util.KeyStatusLegacy, w.Header().Get(util.KeyStatusHeader))
		assert.Equal(t, util.LegacyKeyNotice, w.Header().Get(util.NoticeHeader))
		assert.Equal(t, []string{util.KeyDeprecationLink}, w.Header().Values("Link"))
		assertNoRfc9745Headers(t, w)
	})

	t.Run("a Limited key on /v1 gets the status but never the remedial signal", func(t *testing.T) {
		// given: a Limited (clipper) key is nil-grant forever — a grant is
		// only ever valid on JsonAPI scope — so the re-issue advice cannot be
		// followed and must not be given, and the usage log must not count it
		// (the metric exists to measure legacy JSON-API keys before a sunset)
		fx := newFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{
			"clipperKey": {Token: "tok", AppName: "clipper", Scope: model.AccountAuth_Limited, KeyId: "hashLimited"},
		}
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}, nil).Once()
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		// when
		w := serveWithKey(fx, "GET", "/v1/spaces", "clipperKey")

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, util.KeyStatusLegacy, w.Header().Get(util.KeyStatusHeader),
			"the status header stays unconditional — absence must never mean anything")
		assert.Empty(t, w.Header().Get(util.NoticeHeader), "the notice is JsonAPI-only")
		assert.Empty(t, w.Header().Values("Link"), "the deprecation link is JsonAPI-only")
		assert.NotContains(t, fx.legacyKeyLogSeen, "hashLimited",
			"a non-JSON-API key must not arm the legacy-usage log limiter")
	})

	t.Run("the notice is one printable single-line ASCII sentence", func(t *testing.T) {
		// npm's pattern: a client may print it verbatim, so it must never
		// need escaping and never interpolate user data
		require.NotContains(t, util.LegacyKeyNotice, "\n")
		require.NotContains(t, util.LegacyKeyNotice, "%")
		for _, r := range util.LegacyKeyNotice {
			require.True(t, r >= 0x20 && r < 0x7f, "non-printable-ASCII rune %q", r)
		}
	})

	t.Run("the legacy-usage log line is rate-limited per key", func(t *testing.T) {
		// given
		fx := newFixture(t)
		now := time.Now()

		// then: once per key per process start…
		require.True(t, fx.shouldLogLegacyKeyUse("hashA", now))
		require.False(t, fx.shouldLogLegacyKeyUse("hashA", now))
		require.False(t, fx.shouldLogLegacyKeyUse("hashA", now.Add(30*time.Minute)))
		// …re-armed hourly…
		require.True(t, fx.shouldLogLegacyKeyUse("hashA", now.Add(61*time.Minute)))
		// …and per KEY, so two legacy keys each get their line
		require.True(t, fx.shouldLogLegacyKeyUse("hashB", now))
	})
}
