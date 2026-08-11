package server

// v2_spaceref_test.go walks the REAL engine for the short space reference
// (APIV2.md §8.35). The unit tests in core/api/v2 build their own middleware
// chain, so they cannot see whether apiv2.RegisterRoutes actually installs
// the resolution middleware, or whether it installs it in front of the grant
// gate. This file is the one that fails if the router wiring goes.

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	// a real space id off the eval account: 59-char base32 CID, a dot, and
	// the base36 replication key every space on that account shares
	spaceRefFullId = "bafyreihwvsaekzzyb54o7um4hdpvpn5b2invn75lmijhhtghblvphxwz2i.28y6mgnwgodt7"
	spaceRefShort  = "hxwz2i"
)

// newSpaceRefServerFixture is newV2ServerFixture with the analytics
// broadcast tolerated: these probes hit real (non-refused) routes, which the
// analytics middleware reports.
func newSpaceRefServerFixture(t *testing.T) *fixture {
	fx := newV2ServerFixture(t)
	fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
	return fx
}

func TestV2ShortSpaceRefThroughTheRealEngine(t *testing.T) {
	t.Run("the spaces list serves the short reference", func(t *testing.T) {
		// given
		fx := newSpaceRefServerFixture(t)
		registerGrantTestSpace(t, fx, spaceRefFullId, "APIv2 eval")
		fx.KeyToToken = map[string]ApiSessionEntry{"k": {Token: "tok", Scope: model.AccountAuth_JsonAPI}}

		// when
		w := serveWithKey(fx, "GET", "/v2/spaces", "k")

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"id":"`+spaceRefShort+`"`)
		assert.NotContains(t, w.Body.String(), spaceRefFullId)
	})

	t.Run("a short reference on a path param resolves through the registered chain", func(t *testing.T) {
		// given: this is the router-wiring assertion — removing
		// resolveSpaceRef from RegisterRoutes turns this into a 404
		fx := newSpaceRefServerFixture(t)
		registerGrantTestSpace(t, fx, spaceRefFullId, "APIv2 eval")
		fx.KeyToToken = map[string]ApiSessionEntry{"k": {Token: "tok", Scope: model.AccountAuth_JsonAPI}}

		// when
		w := serveWithKey(fx, "GET", "/v2/spaces/"+spaceRefShort, "k")

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"id":"`+spaceRefShort+`"`)
	})

	t.Run("the full id keeps working through the same chain", func(t *testing.T) {
		fx := newSpaceRefServerFixture(t)
		registerGrantTestSpace(t, fx, spaceRefFullId, "APIv2 eval")
		fx.KeyToToken = map[string]ApiSessionEntry{"k": {Token: "tok", Scope: model.AccountAuth_JsonAPI}}

		w := serveWithKey(fx, "GET", "/v2/spaces/"+spaceRefFullId, "k")

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("a granted key reaches its space by the short reference", func(t *testing.T) {
		// given: the grant holds the FULL id, as grants always do. If
		// resolution ran after the gate, the gate would see "hxwz2i", find
		// it in no grant, and answer 403.
		fx := newSpaceRefServerFixture(t)
		registerGrantTestSpace(t, fx, spaceRefFullId, "APIv2 eval")
		grantedSession(fx, "scopedKey", &util.ApiGrant{
			Spaces: []string{spaceRefFullId}, Perms: util.GrantPermsRead,
		})

		// when
		w := serveWithKey(fx, "GET", "/v2/spaces/"+spaceRefShort, "scopedKey")

		// then
		require.Equal(t, http.StatusOK, w.Code, "resolution must run BEFORE the grant gate")
	})

	t.Run("a short reference is not a way past the grant", func(t *testing.T) {
		// given: two live spaces, the key granted only the other one
		fx := newSpaceRefServerFixture(t)
		registerGrantTestSpace(t, fx, spaceRefFullId, "APIv2 eval")
		registerGrantTestSpace(t, fx, otherSpaceRefFullId, "Project Tracker")
		grantedSession(fx, "scopedKey", &util.ApiGrant{
			Spaces: []string{spaceRefFullId}, Perms: util.GrantPermsRead,
		})

		// when: the NON-granted space's tail
		w := serveWithKey(fx, "GET", "/v2/spaces/"+otherSpaceRefShort, "scopedKey")

		// then: refused, quoting the caller's value and never naming the
		// space the tail belongs to
		require.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), otherSpaceRefShort)
		assert.NotContains(t, w.Body.String(), otherSpaceRefFullId)
	})
}

const (
	otherSpaceRefFullId = "bafyreia4znhhjvxek2iux7enfzilekj5vwlgddgogpfgnx5qnim7bugaxa.28y6mgnwgodt7"
	otherSpaceRefShort  = "bugaxa"
)
