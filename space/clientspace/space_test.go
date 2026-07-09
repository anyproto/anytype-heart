package clientspace

import (
	"context"
	"errors"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/headsync/headstorage"
	"github.com/anyproto/any-sync/commonspace/headsync/headstorage/mock_headstorage"
	"github.com/anyproto/any-sync/commonspace/mock_commonspace"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/anystorage/mock_anystorage"
)

func TestMissingMandatoryObjects(t *testing.T) {
	ctx := context.Background()
	derivedIDs := threads.DerivedSmartblockIds{
		Workspace: "ws",
		Home:      "home",
		Archive:   "archive",
		Widgets:   "widgets",
		Profile:   "profile",
	}

	// newSpaceFixture wires a space whose headstorage serves the given entries;
	// ids absent from both maps respond with anystore.ErrDocNotFound
	newSpaceFixture := func(t *testing.T, entries map[string]headstorage.HeadsEntry, errs map[string]error) *space {
		ctrl := gomock.NewController(t)
		headStorage := mock_headstorage.NewMockHeadStorage(ctrl)
		headStorage.EXPECT().GetEntry(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
			func(_ context.Context, id string) (headstorage.HeadsEntry, error) {
				if err, ok := errs[id]; ok {
					return headstorage.HeadsEntry{}, err
				}
				entry, ok := entries[id]
				if !ok {
					return headstorage.HeadsEntry{}, anystore.ErrDocNotFound
				}
				return entry, nil
			})
		storage := mock_anystorage.NewMockClientSpaceStorage(t)
		storage.EXPECT().HeadStorage().Return(headStorage).Maybe()
		commonSpace := mock_commonspace.NewMockSpace(ctrl)
		commonSpace.EXPECT().Storage().AnyTimes().Return(storage)
		commonSpace.EXPECT().Id().AnyTimes().Return("space1")
		return &space{
			common:     commonSpace,
			derivedIDs: derivedIDs,
		}
	}

	presentEntry := func(id string) headstorage.HeadsEntry {
		return headstorage.HeadsEntry{Id: id, Heads: []string{"head-" + id}, DeletedStatus: headstorage.DeletedStatusNotDeleted}
	}

	t.Run("nothing missing when all trees are present", func(t *testing.T) {
		// given
		entries := map[string]headstorage.HeadsEntry{}
		for _, id := range derivedIDs.IDs() {
			entries[id] = presentEntry(id)
		}
		fx := newSpaceFixture(t, entries, nil)

		// when
		missing := fx.missingMandatoryObjects(ctx)

		// then
		assert.Empty(t, missing)
	})

	t.Run("absent, deleted and deletion-queued trees are missing", func(t *testing.T) {
		// given
		entries := map[string]headstorage.HeadsEntry{
			"ws":      presentEntry("ws"),
			"profile": presentEntry("profile"),
			"widgets": {Id: "widgets", DeletedStatus: headstorage.DeletedStatusDeleted},
			"archive": {Id: "archive", DeletedStatus: headstorage.DeletedStatusQueued},
			// "home" has no entry at all
		}
		fx := newSpaceFixture(t, entries, nil)
		want := []string{"home", "widgets", "archive"}

		// when
		missing := fx.missingMandatoryObjects(ctx)

		// then
		assert.ElementsMatch(t, want, missing)
	})

	t.Run("probe error reports the id as missing", func(t *testing.T) {
		// given
		entries := map[string]headstorage.HeadsEntry{}
		for _, id := range derivedIDs.IDs() {
			entries[id] = presentEntry(id)
		}
		fx := newSpaceFixture(t, entries, map[string]error{"ws": errors.New("db is broken")})
		want := []string{"ws"}

		// when
		missing := fx.missingMandatoryObjects(ctx)

		// then
		assert.ElementsMatch(t, want, missing)
	})
}
