package spacecore

import (
	"bytes"
	"context"
	"testing"

	"github.com/anyproto/any-sync/commonspace/spacepayloads"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/space/spacedomain"
)

// TestDerivedSpaceIdStability pins the ids DeriveID returns for fixed keys.
//
// Derived ids address the space header's content, so switching header builders
// silently repoints every account's personal and tech space at a new, empty
// one. commonspace.DeriveId moved to the v1 builder in any-sync v0.13; heart
// derives through StoragePayloadForSpaceDerive to stay on v0. Golden values
// were produced under any-sync v0.12.16 and must not change with a bump.
//
// This drives service.DeriveID rather than the any-sync builder directly, so
// it fails if heart's own choice of builder changes, not only if upstream's
// v0 builder does.
func TestDerivedSpaceIdStability(t *testing.T) {
	// given
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	accountKey, _, err := crypto.GenerateEd25519Key(bytes.NewReader(seed))
	require.NoError(t, err)
	masterKey, _, err := crypto.GenerateEd25519Key(bytes.NewReader(seed))
	require.NoError(t, err)
	w := mock_wallet.NewMockWallet(t)
	w.EXPECT().GetAccountPrivkey().Return(accountKey).Maybe()
	w.EXPECT().GetMasterKey().Return(masterKey).Maybe()
	s := &service{wallet: w}

	for _, tc := range []struct {
		name      string
		spaceType spacedomain.SpaceType
		want      string
	}{
		{
			name:      "personal space",
			spaceType: spacedomain.SpaceTypeRegular,
			want:      "bafyreigyheufx2jtbg3lxenkia5dsx3go52uukhnhg6ijc74mcuv2mfb7q.34cvme2f4id4n",
		},
		{
			name:      "tech space",
			spaceType: spacedomain.SpaceTypeTech,
			want:      "bafyreifzhhjkfzxpex4wdk76k6hzkz37hiyplk2ri2zuzkpiesuq7phse4.34cvme2f4id4n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// when
			got, err := s.DeriveID(context.Background(), tc.spaceType)

			// then
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestDerivedSpaceIdUsesV0Builder states the invariant behind the goldens: the
// v1 builder any-sync now defaults to derives a different id from the same
// keys, which is the regression the goldens above exist to catch.
func TestDerivedSpaceIdUsesV0Builder(t *testing.T) {
	// given
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	signKey, _, err := crypto.GenerateEd25519Key(bytes.NewReader(seed))
	require.NoError(t, err)
	payload := spacepayloads.SpaceDerivePayload{
		SigningKey: signKey,
		MasterKey:  signKey,
		SpaceType:  string(spacedomain.SpaceTypeRegular),
	}

	// when
	v0, err := spacepayloads.StoragePayloadForSpaceDerive(payload)
	require.NoError(t, err)
	v1, err := spacepayloads.StoragePayloadForSpaceDeriveV1(payload)
	require.NoError(t, err)

	// then
	assert.NotEqual(t, v0.SpaceHeaderWithId.Id, v1.SpaceHeaderWithId.Id)
}
