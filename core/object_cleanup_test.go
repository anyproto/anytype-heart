package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// An empty spaceId is a caller bug, not a backend failure: the proto declares BAD_INPUT for it, and
// the handler must reject it before reaching the store (SpaceIndex("") yields an invalid store whose
// error would otherwise surface as UNKNOWN_ERROR).
func TestObjectCleanupSuggestions_EmptySpaceId_BadInput(t *testing.T) {
	mw := &Middleware{}

	resp := mw.ObjectCleanupSuggestions(context.Background(), &pb.RpcObjectCleanupSuggestionsRequest{SpaceId: ""})

	assert.Equal(t, pb.RpcObjectCleanupSuggestionsResponseError_BAD_INPUT, resp.Error.Code)
	assert.Empty(t, resp.Items)
}

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
