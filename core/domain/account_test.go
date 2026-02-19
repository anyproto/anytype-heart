package domain

import (
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/stretchr/testify/require"
)

func TestSpace_deriveAccountMetadata(t *testing.T) {
	randKeys, err := accountdata.NewRandom()
	require.NoError(t, err)
	symKey, err := deriveAccountEncKey(randKeys.SignKey)
	require.NoError(t, err)
	symKeyProto, err := symKey.Marshall()
	require.NoError(t, err)
	metadata1, _, err := DeriveAccountMetadata(randKeys.SignKey)
	require.NoError(t, err)
	metadata2, _, err := DeriveAccountMetadata(randKeys.SignKey)
	require.NoError(t, err)
	require.Equal(t, metadata1, metadata2)

	require.Equal(t, symKeyProto, metadata1.GetIdentity().GetProfileSymKey())
}
