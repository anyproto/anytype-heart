package spacecore

import (
	"bytes"
	"errors"
	"testing"

	"github.com/anyproto/any-sync/commonspace/clientspaceproto"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/mock_storage"
)

// newKeyedService builds the slice of service the exchange needs, for an
// account whose keys derive from seed, over a storage mock.
func newKeyedService(t *testing.T, seed byte, store *mock_storage.MockClientStorage) *service {
	t.Helper()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = seed + byte(i)
	}
	accountKey, _, err := crypto.GenerateEd25519Key(bytes.NewReader(raw))
	require.NoError(t, err)
	masterKey, _, err := crypto.GenerateEd25519Key(bytes.NewReader(raw))
	require.NoError(t, err)
	peerKey, _, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)
	w := mock_wallet.NewMockWallet(t)
	w.EXPECT().GetAccountPrivkey().Return(accountKey).Maybe()
	w.EXPECT().GetMasterKey().Return(masterKey).Maybe()
	keys := accountdata.New(peerKey, accountKey)
	return &service{
		wallet:               w,
		accountKeys:          keys,
		spaceStorageProvider: store,
		discoveryKeys:        newDiscoveryKeySource(store, keys),
	}
}

func TestTechSpaceExchangeInfo(t *testing.T) {
	t.Run("the tech space id and discovery key follow from the account keys alone", func(t *testing.T) {
		// given
		a := newKeyedService(t, 1, mock_storage.NewMockClientStorage(t))
		b := newKeyedService(t, 1, mock_storage.NewMockClientStorage(t))
		other := newKeyedService(t, 9, mock_storage.NewMockClientStorage(t))

		// when
		infoA, infoB, infoOther := a.techSpaceExchangeInfo(), b.techSpaceExchangeInfo(), other.techSpaceExchangeInfo()

		// then
		require.NotEmpty(t, infoA.id)
		require.Len(t, infoA.key, 32)
		assert.Equal(t, infoA, infoB, "deterministic: no storage was consulted")
		assert.NotEqual(t, infoA.id, infoOther.id)
		assert.NotEqual(t, infoA.key, infoOther.key)
		// and the key source is seeded, so token building finds it without disk
		assert.Equal(t, map[string][]byte{infoA.id: infoA.key}, a.discoveryKeys.DiscoveryKeys(t.Context(), []string{infoA.id}))
	})
}

func TestExchangeRequestIds(t *testing.T) {
	t.Run("a cold device asks about the derived tech space", func(t *testing.T) {
		// given
		store := mock_storage.NewMockClientStorage(t)
		store.EXPECT().AllSpaceIds().Return(nil, nil)
		s := newKeyedService(t, 1, store)

		// when
		got, err := s.exchangeRequestIds()

		// then
		require.NoError(t, err)
		want := []string{s.techSpaceExchangeInfo().id}
		assert.Equal(t, want, got)
	})

	t.Run("once the tech space is on disk it is not listed twice", func(t *testing.T) {
		// given
		store := mock_storage.NewMockClientStorage(t)
		s := newKeyedService(t, 1, store)
		techId := s.techSpaceExchangeInfo().id
		store.EXPECT().AllSpaceIds().Return([]string{"spaceA", techId}, nil)

		// when
		got, err := s.exchangeRequestIds()

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"spaceA", techId}, got)
	})

	t.Run("a storage error is returned", func(t *testing.T) {
		// given
		store := mock_storage.NewMockClientStorage(t)
		store.EXPECT().AllSpaceIds().Return(nil, errors.New("datadir"))
		s := newKeyedService(t, 1, store)

		// when
		_, err := s.exchangeRequestIds()

		// then
		require.Error(t, err)
	})
}

func TestColdDeviceExchangesOnTechSpace(t *testing.T) {
	const callerId, responderId = "coldPeer", "warmPeer"
	nonce, err := clientspaceproto.NewNonceV2()
	require.NoError(t, err)

	t.Run("a cold caller learns that a warm responder holds its account", func(t *testing.T) {
		// given: the caller has no storage, only the seeded tech key; the
		// responder holds the tech space and one more
		cold := newKeyedService(t, 1, func() *mock_storage.MockClientStorage {
			st := mock_storage.NewMockClientStorage(t)
			st.EXPECT().AllSpaceIds().Return(nil, nil)
			return st
		}())
		callerIds, err := cold.exchangeRequestIds()
		require.NoError(t, err)
		tech := cold.techSpaceExchangeInfo()
		callerKeys := cold.discoveryKeys.DiscoveryKeys(t.Context(), callerIds)
		responderIds := []string{tech.id, "spaceB"}
		responderKeys := map[string][]byte{tech.id: tech.key, "spaceB": discoveryKeyFor(t, "spaceB")}

		// when
		request, err := requestTokens(callerIds, callerKeys, nonce, callerId, responderId)
		require.NoError(t, err)
		sharedByResponder, proofs := respondTokens(responderIds, responderKeys, request, nonce, callerId, responderId)
		got := matchProofs(callerIds, callerKeys, proofs, nonce, callerId, responderId)

		// then
		assert.Equal(t, []string{tech.id}, got)
		assert.Equal(t, []string{tech.id}, sharedByResponder)
	})

	t.Run("a cold responder does not claim a space it does not hold", func(t *testing.T) {
		// given: both devices are cold; the responder answers from disk only
		cold := newKeyedService(t, 1, func() *mock_storage.MockClientStorage {
			st := mock_storage.NewMockClientStorage(t)
			st.EXPECT().AllSpaceIds().Return(nil, nil)
			return st
		}())
		callerIds, err := cold.exchangeRequestIds()
		require.NoError(t, err)
		callerKeys := cold.discoveryKeys.DiscoveryKeys(t.Context(), callerIds)
		var responderIds []string // the inbound side stays on AllSpaceIds
		responderKeys := map[string][]byte{}

		// when
		request, err := requestTokens(callerIds, callerKeys, nonce, callerId, responderId)
		require.NoError(t, err)
		_, proofs := respondTokens(responderIds, responderKeys, request, nonce, callerId, responderId)
		got := matchProofs(callerIds, callerKeys, proofs, nonce, callerId, responderId)

		// then
		assert.Empty(t, got)
	})
}
