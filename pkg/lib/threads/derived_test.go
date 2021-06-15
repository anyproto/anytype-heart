package threads

import (
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
