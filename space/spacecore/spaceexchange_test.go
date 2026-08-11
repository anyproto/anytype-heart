package spacecore

import (
	"testing"

	"github.com/anyproto/any-sync/commonspace/clientspaceproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// discoveryKeyFor stands in for a space's derived discovery key: the exchange
// only ever treats it as opaque key material.
func discoveryKeyFor(t *testing.T, spaceId string) []byte {
	t.Helper()
	key, err := clientspaceproto.DeriveDiscoveryKey(crypto.NewAES(), spaceId)
	require.NoError(t, err)
	return key
}

// exchangeTokens mirrors the two sides of the SpaceExchangeV2 handshake over
// the token helpers, so the intersection logic can be checked without a peer.
func exchangeTokens(t *testing.T, nonce []byte, callerId, responderId string, callerKeys, responderKeys map[string][]byte) []string {
	t.Helper()

	// caller: one request token per space it holds a key for, padded and sorted
	var request [][]byte
	for _, key := range callerKeys {
		request = append(request, clientspaceproto.RequestTokenV2(key, nonce, callerId, responderId))
	}
	request, err := clientspaceproto.PadTokensV2(request)
	require.NoError(t, err)

	// responder: answer proofs for the intersection only
	received := make(map[string]struct{}, len(request))
	for _, token := range request {
		received[string(token)] = struct{}{}
	}
	var response [][]byte
	for _, key := range responderKeys {
		if _, ok := received[string(clientspaceproto.RequestTokenV2(key, nonce, callerId, responderId))]; ok {
			response = append(response, clientspaceproto.ResponseTokenV2(key, nonce, callerId, responderId))
		}
	}

	// caller: match proofs back to its own space ids
	proofs := make(map[string]struct{}, len(response))
	for _, token := range response {
		proofs[string(token)] = struct{}{}
	}
	var shared []string
	for spaceId, key := range callerKeys {
		if _, ok := proofs[string(clientspaceproto.ResponseTokenV2(key, nonce, callerId, responderId))]; ok {
			shared = append(shared, spaceId)
		}
	}
	return shared
}

func TestSpaceExchangeV2Tokens(t *testing.T) {
	const callerId, responderId = "callerPeer", "responderPeer"

	nonce, err := clientspaceproto.NewNonceV2()
	require.NoError(t, err)
	require.Len(t, nonce, clientspaceproto.NonceSizeV2)

	sharedKey := discoveryKeyFor(t, "spaceShared")
	callerOnly := discoveryKeyFor(t, "spaceCallerOnly")
	responderOnly := discoveryKeyFor(t, "spaceResponderOnly")

	t.Run("only the intersection is learned", func(t *testing.T) {
		// given
		callerKeys := map[string][]byte{"spaceShared": sharedKey, "spaceCallerOnly": callerOnly}
		responderKeys := map[string][]byte{"spaceShared": sharedKey, "spaceResponderOnly": responderOnly}
		want := []string{"spaceShared"}

		// when
		got := exchangeTokens(t, nonce, callerId, responderId, callerKeys, responderKeys)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("no common space yields nothing", func(t *testing.T) {
		// given
		callerKeys := map[string][]byte{"spaceCallerOnly": callerOnly}
		responderKeys := map[string][]byte{"spaceResponderOnly": responderOnly}

		// when
		got := exchangeTokens(t, nonce, callerId, responderId, callerKeys, responderKeys)

		// then
		assert.Empty(t, got)
	})

	t.Run("tokens are bound to the peer pair", func(t *testing.T) {
		// given
		keys := map[string][]byte{"spaceShared": sharedKey}

		// when: the responder computes against a different caller id
		got := exchangeTokens(t, nonce, callerId, responderId, keys, keys)
		other := exchangeTokens(t, nonce, "otherCaller", responderId, keys, keys)

		// then: each pair matches itself, but their tokens differ
		assert.Equal(t, []string{"spaceShared"}, got)
		assert.Equal(t, []string{"spaceShared"}, other)
		assert.NotEqual(t,
			clientspaceproto.RequestTokenV2(sharedKey, nonce, callerId, responderId),
			clientspaceproto.RequestTokenV2(sharedKey, nonce, "otherCaller", responderId))
	})

	t.Run("tokens are bound to the nonce", func(t *testing.T) {
		// given
		otherNonce, err := clientspaceproto.NewNonceV2()
		require.NoError(t, err)

		// when / then
		assert.NotEqual(t,
			clientspaceproto.RequestTokenV2(sharedKey, nonce, callerId, responderId),
			clientspaceproto.RequestTokenV2(sharedKey, otherNonce, callerId, responderId))
	})

	t.Run("request is padded to a bucket, hiding the space count", func(t *testing.T) {
		// given
		tokens := [][]byte{clientspaceproto.RequestTokenV2(sharedKey, nonce, callerId, responderId)}

		// when
		padded, err := clientspaceproto.PadTokensV2(tokens)

		// then
		require.NoError(t, err)
		assert.Len(t, padded, 16)
		assert.True(t, clientspaceproto.ContainsTokenV2(padded, tokens[0]))
	})
}
