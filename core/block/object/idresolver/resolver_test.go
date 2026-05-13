package idresolver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

// Personal widgets ids must resolve without a bindId entry — the resolver
// short-circuits the lookup so a clean cache can still open the virtual
// object on first load.
func TestResolveSpaceID_personalWidgetsShortCircuit(t *testing.T) {
	r := newTestResolver(t)
	const spaceId = "bafya4spacepayload.yt7efooa"

	got, err := r.ResolveSpaceID(domain.NewPersonalWidgetsId(spaceId))

	require.NoError(t, err)
	assert.Equal(t, spaceId, got)
}

func TestResolveSpaceID_malformedPersonalWidgetsId(t *testing.T) {
	r := newTestResolver(t)

	_, err := r.ResolveSpaceID("_personalWidgets_")

	assert.Error(t, err)
}

func newTestResolver(t *testing.T) *resolver {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &resolver{
		componentCtx:       ctx,
		componentCtxCancel: cancel,
		objectStore:        objectstore.NewStoreFixture(t),
	}
}
