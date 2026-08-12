package spacecore

import (
	"github.com/anyproto/any-sync/commonspace/clientspaceproto"
)

// The three steps of the SpaceExchangeV2 handshake, shared by the outbound
// (peer.go) and inbound (rpchandler.go) sides so both derive their tokens
// through the same code. Every token is bound to the ordered (caller,
// responder) peer pair, so the two sides must pass the pair in the same order
// regardless of which one they are.

// requestTokens builds the caller's membership token per space it can derive a
// discovery key for, padded to a bucket size and sorted so the list length
// reveals nothing about how many spaces the caller actually holds.
func requestTokens(spaceIds []string, keys map[string][]byte, nonce []byte, callerPeerId, responderPeerId string) ([][]byte, error) {
	tokens := make([][]byte, 0, len(keys))
	for _, spaceId := range spaceIds {
		if key, ok := keys[spaceId]; ok {
			tokens = append(tokens, clientspaceproto.RequestTokenV2(key, nonce, callerPeerId, responderPeerId))
		}
	}
	return clientspaceproto.PadTokensV2(tokens)
}

// respondTokens is the responder side: recompute the caller's expected token
// for every space we hold a key for, and answer a membership proof for the
// intersection only. Returns the intersecting space ids alongside the proofs.
func respondTokens(spaceIds []string, keys map[string][]byte, received [][]byte, nonce []byte, callerPeerId, responderPeerId string) (shared []string, proofs [][]byte) {
	set := tokenSet(received)
	for _, spaceId := range spaceIds {
		key, ok := keys[spaceId]
		if !ok {
			continue
		}
		if _, ok = set[string(clientspaceproto.RequestTokenV2(key, nonce, callerPeerId, responderPeerId))]; !ok {
			continue
		}
		shared = append(shared, spaceId)
		proofs = append(proofs, clientspaceproto.ResponseTokenV2(key, nonce, callerPeerId, responderPeerId))
	}
	return shared, proofs
}

// matchProofs is the caller side of the response: map the responder's proofs
// back to our own space ids. A proof we cannot reproduce belongs to a space we
// do not hold, so it is ignored.
func matchProofs(spaceIds []string, keys map[string][]byte, proofs [][]byte, nonce []byte, callerPeerId, responderPeerId string) (shared []string) {
	set := tokenSet(proofs)
	for _, spaceId := range spaceIds {
		key, ok := keys[spaceId]
		if !ok {
			continue
		}
		if _, ok = set[string(clientspaceproto.ResponseTokenV2(key, nonce, callerPeerId, responderPeerId))]; ok {
			shared = append(shared, spaceId)
		}
	}
	return shared
}

func tokenSet(tokens [][]byte) map[string]struct{} {
	set := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		set[string(token)] = struct{}{}
	}
	return set
}
