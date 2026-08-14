package identity

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

// Rehydration is the pass-3 restart's identity seed (DM spec §8.1, the
// 08-13 §6.2 rehydration minus converter concerns): a resumed run MINTS
// NOTHING — every id it uses was recorded by a previous incarnation.

func rehydratedService(t *testing.T, entries []RehydratedEntry, files []RehydratedFile) *Service {
	store := objectstore.NewStoreFixture(t)
	space := mock_clientspace.NewMockSpace(t) // no payload expectations: resume mints nothing
	return NewService(space, store.SpaceIndex(spaceId), false, time.Unix(1700000000, 0),
		WithRehydrated(entries, files))
}

func TestRehydratedEntries(t *testing.T) {
	t.Run("a non-terminal minted claim assigns its recorded id and payload without minting", func(t *testing.T) {
		// given
		service := rehydratedService(t, []RehydratedEntry{{
			SourceKey:    "page-1",
			ObjectId:     "obj-1",
			PayloadRoot:  []byte("raw-root"),
			PayloadHeads: []string{"obj-1"},
		}}, nil)

		// when
		assignment, err := service.Assign("page-1")

		// then: the recorded payload is reconstructed whole — the id is the
		// hash of these bytes, so nothing may be re-minted
		require.NoError(t, err)
		assert.Equal(t, "obj-1", assignment.Id)
		assert.False(t, assignment.IsExisting)
		require.NotNil(t, assignment.Payload.RootRawChange)
		assert.Equal(t, "obj-1", assignment.Payload.RootRawChange.Id)
		assert.Equal(t, []byte("raw-root"), assignment.Payload.RootRawChange.GetRawChange())
		assert.Equal(t, []string{"obj-1"}, assignment.Payload.Heads)
	})

	t.Run("a matched claim assigns as existing", func(t *testing.T) {
		// given
		service := rehydratedService(t, []RehydratedEntry{{
			SourceKey: "page-2", ObjectId: "obj-2", Matched: true,
		}}, nil)

		// when
		assignment, err := service.Assign("page-2")

		// then
		require.NoError(t, err)
		assert.Equal(t, "obj-2", assignment.Id)
		assert.True(t, assignment.IsExisting)
	})

	t.Run("a terminal claim counts as arrived and stays resolvable", func(t *testing.T) {
		// given: the object persisted in a previous incarnation — the replay
		// skips it, so Assign never runs, but references must still resolve
		// and reconciliation must not flag it
		service := rehydratedService(t, []RehydratedEntry{{
			SourceKey: "page-3", ObjectId: "obj-3", Terminal: true,
		}}, nil)

		// when / then
		id, ok := service.Resolve("page-3")
		assert.True(t, ok)
		assert.Equal(t, "obj-3", id)
		assert.Empty(t, service.UnassignedClaims(),
			"a terminal rehydrated claim arrived in a previous incarnation")
	})

	t.Run("a non-terminal claim is unassigned until the replay delivers it", func(t *testing.T) {
		// given
		service := rehydratedService(t, []RehydratedEntry{{
			SourceKey: "page-4", ObjectId: "obj-4", PayloadRoot: []byte("r"),
		}}, nil)

		// then: reconciliation sees it pending; assigning settles it
		assert.Equal(t, []string{"page-4"}, service.UnassignedClaims())
		_, err := service.Assign("page-4")
		require.NoError(t, err)
		assert.Empty(t, service.UnassignedClaims())
	})

	t.Run("a claim over a rehydrated key is still a duplicate", func(t *testing.T) {
		// given — pins the shape of the pass-3-only restart: no claim path
		// re-claims a rehydrated key (pass 1 does not re-run; late claims are
		// dropped from rehydration and re-claim FRESH keys), so the duplicate
		// guard stays armed.
		service := rehydratedService(t, []RehydratedEntry{{
			SourceKey: "page-5", ObjectId: "obj-5", PayloadRoot: []byte("r"),
		}}, nil)

		// when
		err := service.Claim(context.Background(), importv2.IdentityClaim{
			SourceKey: "page-5", SbType: coresb.SmartBlockTypePage,
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate source key")
	})
}

func TestRehydratedFiles(t *testing.T) {
	t.Run("a completed upload rehydrates as an already-resolved future", func(t *testing.T) {
		// given — 08-13 §6.2: futures for done file rows are rehydrated
		// already-resolved; they cannot deadlock and references to them
		// resolve without any re-upload
		service := rehydratedService(t, nil, []RehydratedFile{{
			SourceKey: "img.png", ObjectId: "file-1",
		}})

		// when
		id, err := service.ResolveFile(context.Background(), "img.png")

		// then
		require.NoError(t, err)
		assert.Equal(t, "file-1", id)

		// and: re-registration by the replayed stream is a no-op, and a
		// late CompleteFile cannot change the resolved id
		service.RegisterFile("img.png")
		service.CompleteFile("img.png", "other-id", nil)
		id, err = service.ResolveFile(context.Background(), "img.png")
		require.NoError(t, err)
		assert.Equal(t, "file-1", id)
	})
}
