package v2service

// spaceref_test.go pins the short space reference (APIV2.md §8.35).
//
// THE FIXTURE IS THE TEST. Every space id below is built the way a REAL
// account's is: a 59-character base32 CID, a dot, and a base36 replication
// key that is THE SAME for every space. That is not a simplification — a
// derived space hashes the account key and every client-created space
// reuses the personal space's, so all 17 spaces on the eval account end
// `.28y6mgnwgodt7`. A fixture with DIFFERING replication keys would pass
// every assertion in this file while production served no short references
// at all: the composite tails would be identical, the collision rule would
// fire on every pair, and the degradation would be silent. So the
// identical-key case is the pinned one, and computing the census over the
// CID half is what makes it work.

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// testReplKey is the eval account's replication key — the one every space
// on that account shares.
const testReplKey = ".28y6mgnwgodt7"

// Four real space ids off the eval account, verbatim. Their CIDs differ in
// every position that matters; their replication keys do not differ at all.
const (
	realSpaceTracker                          = "bafyreia4znhhjvxek2iux7enfzilekj5vwlgddgogpfgnx5qnim7bugaxa" + testReplKey
	realSpacePersonal, realSpacePersonalShort = "bafyreiay4rdeleruyuy6x575hvhtifedmjq4g3ojpffvpbgmuackq3oake" + testReplKey, "q3oake"
	realSpaceEval                             = "bafyreihwvsaekzzyb54o7um4hdpvpn5b2invn75lmijhhtghblvphxwz2i" + testReplKey
	realSpaceEngineering                      = "bafyreie3qxj7466htdh75x5jxgh3qpa4e2t75oyplwffdbsjdpijrnjln4" + testReplKey
)

// twinPersonal shares realSpacePersonal's last six CID characters and
// differs in the ninth from the end — the collision case.
const twinPersonal = "bafyreiby4rdeleruyuy6x575hvhtifedmjq4g3ojpffvpbgmuackq3oake" + testReplKey

func realSpaceIds() []string {
	return []string{realSpaceTracker, realSpacePersonal, realSpaceEval, realSpaceEngineering}
}

func TestShortSpaceRefs(t *testing.T) {
	t.Run("an account whose spaces share one replication key still shortens every space", func(t *testing.T) {
		// given: the production shape — CIDs differ, replication keys do not
		ids := realSpaceIds()

		// when
		got := shortSpaceRefs(ids)

		// then: every space has a short form, and they are all distinct
		require.Len(t, got, len(ids), "a shared replication key must not suppress the short form")
		distinct := map[string]bool{}
		for _, id := range ids {
			short, ok := got[id]
			require.True(t, ok, "no short form for %s", id)
			assert.Len(t, short, spaceShortRefLen)
			assert.NotContains(t, short, ".", "the short form carries no dot — that is the point")
			distinct[short] = true
		}
		assert.Len(t, distinct, len(ids))
	})

	t.Run("the census runs over the CID half — composite tails would collapse to one", func(t *testing.T) {
		// given: this is the regression. Feeding the COMPOSITE ids into a
		// tail census gives one bucket for the whole account, because the
		// tail is the shared replication key.
		ids := realSpaceIds()
		composite := map[string]int{}
		for _, id := range ids {
			composite[id[len(id)-spaceShortRefLen:]]++
		}
		require.Len(t, composite, 1, "premise: every composite tail is the same")

		// when / then: the real mint does not collapse
		assert.Len(t, shortSpaceRefs(ids), len(ids))
	})

	t.Run("colliding CID tails keep BOTH full spellings", func(t *testing.T) {
		// given: two spaces whose CID halves end alike
		ids := []string{realSpacePersonal, twinPersonal, realSpaceEval}

		// when
		got := shortSpaceRefs(ids)

		// then: neither twin shortens; the third space is unaffected
		assert.NotContains(t, got, realSpacePersonal)
		assert.NotContains(t, got, twinPersonal)
		assert.Contains(t, got, realSpaceEval)
	})

	t.Run("an id that is not space-shaped never shortens", func(t *testing.T) {
		// given: synthetic ids — a fixture's, and a dotted-but-not-base36 one
		ids := []string{"spaceLive", "space1", "not-a-space.id", realSpaceEval}

		// when
		got := shortSpaceRefs(ids)

		// then: only the real one is in the mechanism at all
		assert.Equal(t, map[string]string{realSpaceEval: "hxwz2i"}, got)
	})

	t.Run("the short form is a suffix of the CID half, not of the composite id", func(t *testing.T) {
		short := shortSpaceRefs([]string{realSpacePersonal})[realSpacePersonal]
		assert.Equal(t, realSpacePersonalShort, short)
		assert.True(t, len(realSpacePersonal) > len(short))
	})
}

func TestMatchSpaceRef(t *testing.T) {
	ids := realSpaceIds()

	t.Run("the exact full id wins", func(t *testing.T) {
		idx, matches := matchSpaceRef(ids, realSpaceEval)
		require.Equal(t, 1, matches)
		assert.Equal(t, realSpaceEval, ids[idx])
	})

	t.Run("a unique CID tail resolves", func(t *testing.T) {
		idx, matches := matchSpaceRef(ids, "hxwz2i")
		require.Equal(t, 1, matches)
		assert.Equal(t, realSpaceEval, ids[idx])
	})

	t.Run("the whole CID half resolves — the §8.34 truncation is a suffix of itself", func(t *testing.T) {
		truncated := spaceIdCid(realSpaceEval)
		idx, matches := matchSpaceRef(ids, truncated)
		require.Equal(t, 1, matches)
		assert.Equal(t, realSpaceEval, ids[idx])
	})

	t.Run("an ambiguous tail reports every claimant", func(t *testing.T) {
		_, matches := matchSpaceRef([]string{realSpacePersonal, twinPersonal}, realSpacePersonalShort)
		assert.Equal(t, 2, matches)
	})

	t.Run("an unshaped id answers to its exact id only", func(t *testing.T) {
		unshaped := []string{"spaceLive"}
		_, exact := matchSpaceRef(unshaped, "spaceLive")
		assert.Equal(t, 1, exact)
		_, suffix := matchSpaceRef(unshaped, "ceLive")
		assert.Zero(t, suffix, "a synthetic id must not answer to a tail")
	})

	t.Run("the empty reference matches nothing", func(t *testing.T) {
		_, matches := matchSpaceRef(ids, "")
		assert.Zero(t, matches)
	})
}

func TestResolveSpaceRef(t *testing.T) {
	registerReal := func(t *testing.T, fx *v2Fixture, ids ...string) {
		for i, id := range ids {
			fx.registerSpaceView(t, id, "Space "+string(rune('A'+i)), "")
		}
	}

	t.Run("a short reference resolves to the full id", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		registerReal(t, fx, realSpaceIds()...)

		// when
		got, err := fx.ResolveSpaceRef(context.Background(), "hxwz2i")

		// then
		require.NoError(t, err)
		assert.Equal(t, realSpaceEval, got)
	})

	t.Run("a full id passes through without reading the space list", func(t *testing.T) {
		// given: NO space views at all — a lookup would find nothing, so a
		// full id coming back proves the common path never consults the store
		fx := newV2FixtureBare(t)

		// when
		got, err := fx.ResolveSpaceRef(context.Background(), realSpaceEval)

		// then
		require.NoError(t, err)
		assert.Equal(t, realSpaceEval, got)
	})

	t.Run("an unknown reference comes back unchanged, for the ordinary refusal", func(t *testing.T) {
		fx := newV2FixtureBare(t)
		registerReal(t, fx, realSpaceEval)

		got, err := fx.ResolveSpaceRef(context.Background(), "zzzzzz")

		require.NoError(t, err)
		assert.Equal(t, "zzzzzz", got)
	})

	t.Run("an ambiguous reference is refused 400 with the candidates listed", func(t *testing.T) {
		// given: the twins, which have no short form of their own
		fx := newV2FixtureBare(t)
		registerReal(t, fx, realSpacePersonal, twinPersonal)

		// when
		_, err := fx.ResolveSpaceRef(context.Background(), realSpacePersonalShort)

		// then
		var apiErr *v2model.Error
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		require.Len(t, apiErr.Issues, 1)
		// the candidates are named in the form the list SERVES them: both
		// twins collide, so both are named in full
		assert.Contains(t, apiErr.Issues[0].Message, realSpacePersonal)
		assert.Contains(t, apiErr.Issues[0].Message, twinPersonal)
	})

	t.Run("a short reference cannot reach a space the key was not granted", func(t *testing.T) {
		// given: two live spaces, one granted. This is the probe guard: if
		// resolution ran over ALL spaces rather than the caller's visible
		// ones, the non-granted tail would resolve here and the grant check
		// downstream would then answer 403 — confirming the space exists —
		// instead of the same "not found" any other unknown value gets.
		fx := newV2FixtureBare(t)
		registerReal(t, fx, realSpaceEval, realSpaceTracker)
		grant := &util.ApiGrant{Spaces: []string{realSpaceEval}, Perms: util.GrantPermsRead}
		ctx := util.CtxWithApiGrant(context.Background(), grant)

		// when: the granted space's tail, then the non-granted one's
		granted, err := fx.ResolveSpaceRef(ctx, "hxwz2i")
		require.NoError(t, err)
		notGranted, err := fx.ResolveSpaceRef(ctx, "bugaxa")
		require.NoError(t, err)

		// then
		assert.Equal(t, realSpaceEval, granted)
		assert.Equal(t, "bugaxa", notGranted, "a non-granted tail must not resolve")

		// and the grant backstop still refuses the unresolved value
		require.Error(t, ensureSpaceGranted(ctx, notGranted))
	})

	t.Run("the full id of a non-granted space is still refused by the grant, not hidden", func(t *testing.T) {
		// given: resolution is additive — it must not change what a full id does
		fx := newV2FixtureBare(t)
		registerReal(t, fx, realSpaceEval, realSpaceTracker)
		grant := &util.ApiGrant{Spaces: []string{realSpaceEval}, Perms: util.GrantPermsRead}
		ctx := util.CtxWithApiGrant(context.Background(), grant)

		// when
		got, err := fx.ResolveSpaceRef(ctx, realSpaceTracker)

		// then
		require.NoError(t, err)
		assert.Equal(t, realSpaceTracker, got)
		require.Error(t, ensureSpaceGranted(ctx, got))
	})
}

func TestSpacesSurfaceServesShortRefs(t *testing.T) {
	t.Run("GET /v2/spaces rows carry the short form", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		fx.registerSpaceView(t, realSpaceEval, "APIv2 eval", "")
		fx.registerSpaceView(t, realSpaceTracker, "Project Tracker", "")
		want := []v2model.SpaceRow{
			{Id: "bugaxa", Name: "Project Tracker"},
			{Id: "hxwz2i", Name: "APIv2 eval"},
		}

		// when
		rows, total, _, err := fx.ListSpaces(context.Background(), 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows)
		assert.Equal(t, 2, total)
	})

	t.Run("colliding spaces are listed in full — never an ambiguous short form", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		fx.registerSpaceView(t, realSpacePersonal, "Personal", "")
		fx.registerSpaceView(t, twinPersonal, "Personal twin", "")

		// when
		rows, _, _, err := fx.ListSpaces(context.Background(), 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 2)
		for _, row := range rows {
			assert.Contains(t, []string{realSpacePersonal, twinPersonal}, row.Id)
		}
	})

	t.Run("the census precedes pagination — page 2 cannot claim page 1's tail", func(t *testing.T) {
		// given: the twins land on different pages
		fx := newV2FixtureBare(t)
		fx.registerSpaceView(t, realSpacePersonal, "Personal", "")
		fx.registerSpaceView(t, twinPersonal, "Personal twin", "")

		// when
		first, _, hasMore, err := fx.ListSpaces(context.Background(), 0, 1)

		// then
		require.NoError(t, err)
		require.True(t, hasMore)
		require.Len(t, first, 1)
		assert.Contains(t, []string{realSpacePersonal, twinPersonal}, first[0].Id,
			"a one-row page must not mint a tail the row it omitted also claims")
	})

	t.Run("GET-one serves the short form and accepts either spelling", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		fx.registerSpaceView(t, realSpaceEval, "APIv2 eval", "The eval space")
		want := v2model.Space{Id: "hxwz2i", Name: "APIv2 eval", Description: "The eval space"}

		// when: the store is addressed by the FULL id (the route middleware
		// has already resolved), and the served id is the short one
		got, err := fx.GetSpace(context.Background(), realSpaceEval)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
}

func TestWhoamiServesShortSpaceRefs(t *testing.T) {
	t.Run("the grant echo names spaces the way the list does", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		fx.registerSpaceView(t, realSpaceEval, "APIv2 eval", "")
		grant := &util.ApiGrant{Spaces: []string{realSpaceEval}, Perms: util.GrantPermsRead}

		// when
		names, refs, err := fx.resolveGrantedSpaceNames(util.CtxWithApiGrant(context.Background(), grant), grant)

		// then: keyed by the FULL id — a grant is keyed by space id and this
		// must not change that
		require.NoError(t, err)
		assert.Equal(t, map[string]string{realSpaceEval: "APIv2 eval"}, names)
		assert.Equal(t, map[string]string{realSpaceEval: "hxwz2i"}, refs)
	})

	t.Run("a granted space the caller cannot see keeps its full spelling", func(t *testing.T) {
		// given: granted, but no live space view — no census entry, no tail
		fx := newV2FixtureBare(t)
		grant := &util.ApiGrant{Spaces: []string{realSpaceEval}, Perms: util.GrantPermsRead}

		// when
		_, refs, err := fx.resolveGrantedSpaceNames(util.CtxWithApiGrant(context.Background(), grant), grant)

		// then
		require.NoError(t, err)
		assert.NotContains(t, refs, realSpaceEval)
	})
}
