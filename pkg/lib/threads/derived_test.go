package threads

import (
	"math/rand"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/textileio/go-threads/core/thread"
)

func TestPatchSmartBlockType(t *testing.T) {
	origIdTid, err := ThreadCreateID(thread.AccessControlled, smartblock.SmartBlockTypeProfilePage)
	require.NoError(t, err)
	origId := origIdTid.String()
	newId, err := PatchSmartBlockType(origId, smartblock.SmartBlockTypeBundledTemplate)
	require.NoError(t, err)
	newSbt, err := smartblock.SmartBlockTypeFromID(newId)
	require.NoError(t, err)
	assert.Equal(t, smartblock.SmartBlockTypeBundledTemplate, newSbt)
}

func TestPatchSmartBlockType_Account(t *testing.T) {
	accountKey := make([]byte, 32)
	rand.Read(accountKey)

	id, err := threadDeriveId(threadDerivedIndexAccountOld, accountKey)
	require.NoError(t, err)
	sbt, err := smartblock.SmartBlockTypeFromThreadID(id)
	require.NoError(t, err)

	require.Equal(t, smartblock.SmartBlockType(0x0), sbt)

	id, err = threadDeriveId(threadDerivedIndexAccount, accountKey)
	require.NoError(t, err)
	sbt, err = smartblock.SmartBlockTypeFromThreadID(id)
	require.NoError(t, err)

	require.Equal(t, smartblock.SmartBlockTypeWorkspaceV2, sbt)
}
