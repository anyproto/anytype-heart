package compose

// usedkeys_test.go pins the byte-level used-key census against the chain
// the codec itself runs (§3): legend first, bundled table second, verbatim
// last. It is the shared implementation behind
// cmd/internal/anyblockbatch.UsedPropertyKeys, whose own drift-pin test
// (TestLintResolvesPropertyTermsLikeTheCodec) covers the codec agreement;
// this one covers the slots.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// How this can fail: read `recommended*` off the document root instead of
// `type_settings.property_definitions` (post-v0.32 declarations vanish from
// the census and the dictionary silently shrinks); stop skipping id/type
// (two envelope facts join every dictionary); resolve an `internal_key`
// through the slug ladder (a stored id that happens to fold onto a bundled
// key gets rewritten).
func TestUsedPropertyKeysFromBytes(t *testing.T) {
	doc := []byte(`{
		"version": 1,
		"id": "bafyx",
		"type": "task",
		"property_internal_keys": {"aroma": "6a32d4856761631534b22f85"},
		"properties": {
			"aroma": "smoky",
			"due_date": "2026-01-01",
			"custom_verbatim": 3
		},
		"type_settings": {
			"property_definitions": [
				{"property": "due_date"},
				{"internal_key": "64f2d485676163153aaaaaaa", "name": "Team"},
				{"name": "name-only entries state no identity"}
			]
		}
	}`)

	used, err := UsedPropertyKeysFromBytes(doc)
	require.NoError(t, err)

	assert.True(t, used["6a32d4856761631534b22f85"], "the legend resolves the spelling (chain step 1)")
	assert.True(t, used["dueDate"], "the bundled table resolves the slug (chain step 2)")
	assert.True(t, used["custom_verbatim"], "an unresolvable spelling passes through verbatim (chain step 4)")
	assert.True(t, used["64f2d485676163153aaaaaaa"], "a stated internal_key is its own address")
	assert.False(t, used["id"], "envelope facts are not property references")
	assert.False(t, used["type"], "envelope facts are not property references")
	assert.Len(t, used, 4)

	_, err = UsedPropertyKeysFromBytes([]byte("not json"))
	assert.Error(t, err)
}
