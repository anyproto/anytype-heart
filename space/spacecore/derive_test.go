package spacecore

import (
	"bytes"
	"testing"

	"github.com/anyproto/any-sync/commonspace/spacepayloads"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDerivedSpaceIdStability pins the derived space id for fixed keys.
//
// Derived ids address the space header's content, so switching header builders
// silently repoints every account's personal and tech space at a new, empty
// one. commonspace.DeriveId moved to the v1 builder in any-sync v0.13; heart
// derives through StoragePayloadForSpaceDerive to stay on v0. Golden values
// were produced under any-sync v0.12.16 and must not change with a bump.
func TestDerivedSpaceIdStability(t *testing.T) {
	newDerivePayload := func(t *testing.T, spaceType string) spacepayloads.SpaceDerivePayload {
		t.Helper()
		seed := make([]byte, 32)
		for i := range seed {
			seed[i] = byte(i)
		}
		signKey, _, err := crypto.GenerateEd25519Key(bytes.NewReader(seed))
		require.NoError(t, err)
		masterKey, _, err := crypto.GenerateEd25519Key(bytes.NewReader(seed))
		require.NoError(t, err)
		return spacepayloads.SpaceDerivePayload{
			SigningKey: signKey,
			MasterKey:  masterKey,
			SpaceType:  spaceType,
		}
	}

	for _, tc := range []struct {
		name      string
		spaceType string
		want      string
	}{
		{
			name:      "regular space",
			spaceType: "anytype.space",
			want:      "bafyreigyheufx2jtbg3lxenkia5dsx3go52uukhnhg6ijc74mcuv2mfb7q.34cvme2f4id4n",
		},
		{
			name:      "tech space",
			spaceType: "anytype.techspace",
			want:      "bafyreifzhhjkfzxpex4wdk76k6hzkz37hiyplk2ri2zuzkpiesuq7phse4.34cvme2f4id4n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			payload := newDerivePayload(t, tc.spaceType)

			// when
			got, err := spacepayloads.StoragePayloadForSpaceDerive(payload)

			// then
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.SpaceHeaderWithId.Id)

			// the v1 builder commonspace.DeriveId now uses derives a different
			// id — the regression this test guards against
			v1, err := spacepayloads.StoragePayloadForSpaceDeriveV1(payload)
			require.NoError(t, err)
			assert.NotEqual(t, tc.want, v1.SpaceHeaderWithId.Id)
		})
	}
}
