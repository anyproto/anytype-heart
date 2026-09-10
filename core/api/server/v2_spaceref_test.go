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

	// two spaces differing at the SIXTH character from the end of the CID
	// half: distinct short forms (q3oake / r3oake) over one shared
	// five-character tail that answers to neither alone
	cousinSpaceAFullId = "bafyreiay4rdeleruyuy6x575hvhtifedmjq4g3ojpffvpbgmuackq3oake.28y6mgnwgodt7"
	cousinSpaceAShort  = "q3oake"
	cousinSpaceBFullId = "bafyreiay4rdeleruyuy6x575hvhtifedmjq4g3ojpffvpbgmuackr3oake.28y6mgnwgodt7"
	cousinSharedTail   = "3oake"
)

// TestV2FullSpaceIdsThroughTheRealEngine walks the registered engine for
// §8.36. The package-local middleware tests build their own chain, so none
// of them can see whether apiv2.RegisterRoutes installs ensureIdsShape at
// all — this file is the one that fails if that line goes.
//
// It also covers the surfaces the chain reaches only through the real
// router: whoami (no space in its path, so nothing else exercises the
// parameter there) and the C10-paginated spaces list.
func TestV2FullSpaceIdsThroughTheRealEngine(t *testing.T) {
	t.Run("the spaces list serves the full id under ?ids=full", func(t *testing.T) {
		// given
		fx := newSpaceRefServerFixture(t)
		registerGrantTestSpace(t, fx, spaceRefFullId, "APIv2 eval")
		fx.KeyToToken = map[string]ApiSessionEntry{"k": {Token: "tok", Scope: model.AccountAuth_JsonAPI}}

		// when
		w := serveWithKey(fx, "GET", "/v2/spaces?ids=full", "k")

		// then: the id a caller can persist — and NOT the short reference,
		// which is only unique against the spaces visible right now
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"id":"`+spaceRefFullId+`"`)
		assert.NotContains(t, w.Body.String(), `"id":"`+spaceRefShort+`"`)
	})

	t.Run("the default is untouched — short stays the served shape", func(t *testing.T) {
		fx := newSpaceRefServerFixture(t)
		registerGrantTestSpace(t, fx, spaceRefFullId, "APIv2 eval")
		fx.KeyToToken = map[string]ApiSessionEntry{"k": {Token: "tok", Scope: model.AccountAuth_JsonAPI}}

		w := serveWithKey(fx, "GET", "/v2/spaces?ids=compact", "k")

		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"id":"`+spaceRefShort+`"`)
	})

	t.Run("GET-one accepts the short reference and serves the full id back", func(t *testing.T) {
		// given: the round trip a caller makes to turn a reference it was
		// served into one it can store
		fx := newSpaceRefServerFixture(t)
		registerGrantTestSpace(t, fx, spaceRefFullId, "APIv2 eval")
		fx.KeyToToken = map[string]ApiSessionEntry{"k": {Token: "tok", Scope: model.AccountAuth_JsonAPI}}

		// when
		w := serveWithKey(fx, "GET", "/v2/spaces/"+spaceRefShort+"?ids=full", "k")

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"id":"`+spaceRefFullId+`"`)
	})

	t.Run("whoami spells its grant echo in full", func(t *testing.T) {
		// given: a grant is keyed by the FULL id, and this is the surface
		// that tells a holder which spaces it holds
		fx := newSpaceRefServerFixture(t)
		registerGrantTestSpace(t, fx, spaceRefFullId, "APIv2 eval")
		grantedSession(fx, "scopedKey", &util.ApiGrant{
			Spaces: []string{spaceRefFullId}, Perms: util.GrantPermsRead,
		})

		// when
		full := serveWithKey(fx, "GET", "/v2/auth/whoami?ids=full", "scopedKey")
		short := serveWithKey(fx, "GET", "/v2/auth/whoami", "scopedKey")

		// then
		require.Equal(t, http.StatusOK, full.Code)
		require.Equal(t, http.StatusOK, short.Code)
		assert.Contains(t, full.Body.String(), `"id":"`+spaceRefFullId+`"`)
		assert.Contains(t, short.Body.String(), `"id":"`+spaceRefShort+`"`)
	})

	t.Run("the shape is registered IN FRONT of resolution — an ambiguity's candidates obey it", func(t *testing.T) {
		// given: two spaces that both answer to `3oake` and both have short
		// forms of their own. The candidate list is minted inside
		// ResolveSpaceRef, so it only obeys ?ids= if RegisterRoutes installs
		// ensureIdsShape BEFORE resolveSpaceRef.
		fx := newSpaceRefServerFixture(t)
		registerGrantTestSpace(t, fx, cousinSpaceAFullId, "Cousin A")
		registerGrantTestSpace(t, fx, cousinSpaceBFullId, "Cousin B")
		fx.KeyToToken = map[string]ApiSessionEntry{"k": {Token: "tok", Scope: model.AccountAuth_JsonAPI}}

		// when
		short := serveWithKey(fx, "GET", "/v2/spaces/"+cousinSharedTail+"/objects", "k")
		full := serveWithKey(fx, "GET", "/v2/spaces/"+cousinSharedTail+"/objects?ids=full", "k")

		// then
		require.Equal(t, http.StatusBadRequest, short.Code)
		require.Equal(t, http.StatusBadRequest, full.Code)
		assert.Contains(t, short.Body.String(), cousinSpaceAShort)
		assert.NotContains(t, short.Body.String(), cousinSpaceAFullId)
		assert.Contains(t, full.Body.String(), cousinSpaceAFullId)
		assert.Contains(t, full.Body.String(), cousinSpaceBFullId)
	})

	t.Run("an unknown ids value is a 400 on a space route too", func(t *testing.T) {
		// given: the parameter is validated once for the whole group, so the
		// refusal does not depend on the route owning it
		fx := newSpaceRefServerFixture(t)
		registerGrantTestSpace(t, fx, spaceRefFullId, "APIv2 eval")
		fx.KeyToToken = map[string]ApiSessionEntry{"k": {Token: "tok", Scope: model.AccountAuth_JsonAPI}}

		// when
		w := serveWithKey(fx, "GET", "/v2/spaces?ids=export", "k")

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "compact, full")
	})
}
