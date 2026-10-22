package spacecore

import (
	"testing"

	"github.com/anyproto/any-sync/commonspace/clientspaceproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func discoveryKeyFor(t *testing.T, spaceId string) []byte {
	t.Helper()
	key, err := clientspaceproto.DeriveDiscoveryKey(crypto.NewAES(), spaceId)
	require.NoError(t, err)
	return key
}

// roundTrip drives both sides of the handshake through the production
// functions: the caller builds request tokens, the responder intersects and
// proves, the caller matches the proofs back to its own space ids. A swapped
// peer-id pair on either side breaks the match and yields nothing.
func roundTrip(t *testing.T, nonce []byte, callerId, responderId string, caller, responder map[string][]byte) []string {
	t.Helper()
	callerIds, responderIds := spaceIdsOf(caller), spaceIdsOf(responder)

	request, err := requestTokens(callerIds, caller, nonce, callerId, responderId)
	require.NoError(t, err)

	_, proofs := respondTokens(responderIds, responder, request, nonce, callerId, responderId)

	return matchProofs(callerIds, caller, proofs, nonce, callerId, responderId)
}

// spaceIdsOf returns a stable id order; production passes AllSpaceIds, and the
// matched results must not depend on map iteration order.
func spaceIdsOf(keys map[string][]byte) []string {
	ids := make([]string, 0, len(keys))
	for _, id := range []string{"spaceA", "spaceB", "spaceC"} {
		if _, ok := keys[id]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func TestSpaceExchangeV2RoundTrip(t *testing.T) {
	const callerId, responderId = "callerPeer", "responderPeer"

	nonce, err := clientspaceproto.NewNonceV2()
	require.NoError(t, err)
	require.Len(t, nonce, clientspaceproto.NonceSizeV2)

	keyA := discoveryKeyFor(t, "spaceA")
	keyB := discoveryKeyFor(t, "spaceB")
	keyC := discoveryKeyFor(t, "spaceC")

	t.Run("only the shared space is learned", func(t *testing.T) {
		// given
		caller := map[string][]byte{"spaceA": keyA, "spaceB": keyB}
		responder := map[string][]byte{"spaceB": keyB, "spaceC": keyC}
		want := []string{"spaceB"}

		// when
		got := roundTrip(t, nonce, callerId, responderId, caller, responder)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("every space shared", func(t *testing.T) {
		// given
		both := map[string][]byte{"spaceA": keyA, "spaceB": keyB}
		want := []string{"spaceA", "spaceB"}

		// when
		got := roundTrip(t, nonce, callerId, responderId, both, both)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("no space in common", func(t *testing.T) {
		// given
		caller := map[string][]byte{"spaceA": keyA}
		responder := map[string][]byte{"spaceC": keyC}

		// when
		got := roundTrip(t, nonce, callerId, responderId, caller, responder)

		// then
		assert.Empty(t, got)
	})

	t.Run("responder proves only the intersection", func(t *testing.T) {
		// given
		caller := map[string][]byte{"spaceA": keyA}
		responder := map[string][]byte{"spaceA": keyA, "spaceB": keyB, "spaceC": keyC}

		// when
		request, err := requestTokens(spaceIdsOf(caller), caller, nonce, callerId, responderId)
		require.NoError(t, err)
		shared, proofs := respondTokens(spaceIdsOf(responder), responder, request, nonce, callerId, responderId)

		// then: the responder holds three spaces but answers for one
		assert.Equal(t, []string{"spaceA"}, shared)
		assert.Len(t, proofs, 1)
	})
}

func TestSpaceExchangeV2TokenBinding(t *testing.T) {
	const callerId, responderId = "callerPeer", "responderPeer"

	nonce, err := clientspaceproto.NewNonceV2()
	require.NoError(t, err)
	keyA := discoveryKeyFor(t, "spaceA")
	shared := map[string][]byte{"spaceA": keyA}

	t.Run("a proof minted for another peer pair does not match", func(t *testing.T) {
		// given: an eavesdropper replays proofs captured from a different connection
		request, err := requestTokens(spaceIdsOf(shared), shared, nonce, callerId, responderId)
		require.NoError(t, err)
		_, proofs := respondTokens(spaceIdsOf(shared), shared, request, nonce, callerId, responderId)

		// when: the same proofs are offered to a different caller
		got := matchProofs(spaceIdsOf(shared), shared, proofs, nonce, "otherCaller", responderId)

		// then
		assert.Empty(t, got)
	})

	t.Run("a proof minted under another nonce does not match", func(t *testing.T) {
		// given
		request, err := requestTokens(spaceIdsOf(shared), shared, nonce, callerId, responderId)
		require.NoError(t, err)
		_, proofs := respondTokens(spaceIdsOf(shared), shared, request, nonce, callerId, responderId)
		otherNonce, err := clientspaceproto.NewNonceV2()
		require.NoError(t, err)

		// when
		got := matchProofs(spaceIdsOf(shared), shared, proofs, otherNonce, callerId, responderId)

		// then
		assert.Empty(t, got)
	})

	t.Run("swapping the peer pair on one side breaks the handshake", func(t *testing.T) {
		// given: the responder mistakenly orders the pair (us, them)
		request, err := requestTokens(spaceIdsOf(shared), shared, nonce, callerId, responderId)
		require.NoError(t, err)

		// when
		swapped, proofs := respondTokens(spaceIdsOf(shared), shared, request, nonce, responderId, callerId)

		// then: the guard that the round-trip tests would otherwise not catch
		assert.Empty(t, swapped)
		assert.Empty(t, proofs)
	})

	t.Run("a space the caller lacks a key for is never advertised", func(t *testing.T) {
		// given: spaceB is held but its ACL is unreadable, so no discovery key
		caller := map[string][]byte{"spaceA": keyA}
		allIds := []string{"spaceA", "spaceB"}

		// when
		request, err := requestTokens(allIds, caller, nonce, callerId, responderId)
		require.NoError(t, err)

		// then: exactly one real token, the rest of the bucket is random padding
		assert.Len(t, request, 16)
		assert.True(t, clientspaceproto.ContainsTokenV2(request,
			clientspaceproto.RequestTokenV2(keyA, nonce, callerId, responderId)))
	})
}

func TestRequestTokensPadding(t *testing.T) {
	nonce, err := clientspaceproto.NewNonceV2()
	require.NoError(t, err)

	t.Run("no derivable keys still yields a full bucket", func(t *testing.T) {
		// given: nothing to advertise must look like something
		// when
		got, err := requestTokens([]string{"spaceA"}, nil, nonce, "caller", "responder")

		// then
		require.NoError(t, err)
		assert.Len(t, got, 16)
	})

	t.Run("tokens are sorted so real ones are not positionally identifiable", func(t *testing.T) {
		// given
		keys := map[string][]byte{"spaceA": discoveryKeyFor(t, "spaceA"), "spaceB": discoveryKeyFor(t, "spaceB")}

		// when
		got, err := requestTokens([]string{"spaceA", "spaceB"}, keys, nonce, "caller", "responder")

		// then
		require.NoError(t, err)
		require.Len(t, got, 16)
		assert.IsIncreasing(t, tokenStrings(got))
	})
}

func tokenStrings(tokens [][]byte) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, string(t))
	}
	return out
}
