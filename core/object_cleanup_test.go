package core

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestCleanupKeys_EmptyRequest_UsesDefaultsPlusForced(t *testing.T) {
	keys := cleanupKeys(nil)

	assert.Subset(t, keys, defaultCleanupKeys)
	assert.Subset(t, keys, forcedCleanupKeys)
}

func TestCleanupKeys_CallerKeys_ForcedAlwaysIncluded(t *testing.T) {
	keys := cleanupKeys([]string{"name"})

	assert.Contains(t, keys, domain.RelationKey("name"))
	assert.Contains(t, keys, bundle.RelationKeyId)
	assert.Contains(t, keys, bundle.RelationKeyCreatedInContext)
	assert.Contains(t, keys, bundle.RelationKeyResolvedLayout)
	// caller keys do not pull in the default set
	assert.NotContains(t, keys, bundle.RelationKeySnippet)
}

func TestCleanupKeys_Deduplicates(t *testing.T) {
	// resolvedLayout is both a caller key and a forced key
	keys := cleanupKeys([]string{"resolvedLayout", "resolvedLayout"})

	count := 0
	for _, k := range keys {
		if k == bundle.RelationKeyResolvedLayout {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

// cleanupKeys must not mutate the package-level defaultCleanupKeys slice. append() on a slice with
// spare capacity would write through to the backing array and corrupt it for every later call.
func TestCleanupKeys_DoesNotMutateDefaults(t *testing.T) {
	before := append([]domain.RelationKey{}, defaultCleanupKeys...)

	cleanupKeys(nil)
	cleanupKeys(nil)

	assert.Equal(t, before, defaultCleanupKeys)
}
