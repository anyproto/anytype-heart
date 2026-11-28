package detailservice

// The detail-service writes that hold a request context but build their
// state deep inside an editor (docs/ApiChangeAttribution.md, carries 2 and
// 3). smarttest records what the real smartblock.Apply would stamp on the
// pushed change.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/blockcollection"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/objectgc"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

// workspaceEditor is the workspace object as the detail service meets it:
// the real details editor (basic) over a smartblock.
type workspaceEditor struct {
	smartblock.SmartBlock
	basic.DetailsSettable
}

// gcWithOrphans is a GC that finds the same orphaned files under every
// archived object.
type gcWithOrphans struct {
	*fileGCStub
	files []string
}

func (g gcWithOrphans) CheckObjectsOnObjectArchived(string, string, bool) (objectgc.OrphanCandidates, error) {
	return objectgc.OrphanCandidates{Files: g.files}, nil
}

type homeCollection struct {
	smartblock.SmartBlock
	blockcollection.Collection
}

func TestApiWritesAreAttributed(t *testing.T) {
	appCtx := context.Background()
	apiCtx := domain.CtxWithIntegrationName(appCtx, "Raycast")
	objects := []objectstore.TestObject{
		{bundle.RelationKeyId: domain.String("obj1"), bundle.RelationKeySpaceId: domain.String(spaceId)},
		{bundle.RelationKeyId: domain.String("obj2"), bundle.RelationKeySpaceId: domain.String(spaceId)},
	}
	linksRoot := func(id string) *smarttest.SmartTest {
		sb := smarttest.New(id)
		sb.AddBlock(simple.New(&model.Block{Id: id, ChildrenIds: []string{}}))
		return sb
	}

	t.Run("PATCH space: each rename is pushed with its own request's attribution", func(t *testing.T) {
		// given: the workspace object behind the real details editor
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("rel-name"),
			bundle.RelationKeySpaceId:        domain.String(spaceId),
			bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyName.String()),
			bundle.RelationKeyUniqueKey:      domain.String("rel-name"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
		}})
		ws := smarttest.New("workspace")
		ws.SetSpaceId(spaceId)
		workspace := workspaceEditor{SmartBlock: ws, DetailsSettable: basic.NewBasic(ws, fx.store.SpaceIndex(spaceId), converter.NewLayoutConverter(), nil)}
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Workspace: "workspace"})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).Return(workspace, nil)
		rename := func(name string) *domain.Details {
			return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyName: domain.String(name)})
		}

		// when: two DISTINCT renames, so both are real changes — the API's,
		// then an app's
		require.NoError(t, fx.SetSpaceInfo(apiCtx, spaceId, rename("Renamed by the API")))
		require.NoError(t, fx.SetSpaceInfo(appCtx, spaceId, rename("Renamed in the app")))

		// then: what each apply would stamp on its pushed change. Red if
		// SetSpaceInfo goes back to passing nil for an API request (first
		// value), or the name outlives the request (second).
		want := []string{"Renamed by the API", "Renamed in the app"}
		assert.Equal(t, want[1], ws.Details().GetString(bundle.RelationKeyName))
		assert.Equal(t, []string{"Raycast", ""}, ws.Results.IntegrationNames)
		// both renames broadcast their events: neither was collected into a
		// session context nobody reads. Red if SetSpaceInfo passes a real
		// session context instead of the attribution-only one.
		assert.Len(t, ws.Results.Events, 2)
	})

	t.Run("favorite through the API: the Home collection change is stamped", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, objects)
		home := linksRoot("home")
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Home: "home"})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			require.Equal(t, "home", objectId)
			// the Home object is this collection over a smartblock
			return homeCollection{SmartBlock: home, Collection: blockcollection.NewCollection(home, fx.store.SpaceIndex(spaceId))}, nil
		})

		// when
		require.NoError(t, fx.SetListIsFavorite(apiCtx, []string{"obj1"}, true))
		require.NoError(t, fx.SetListIsFavorite(appCtx, []string{"obj2"}, true))

		// then: red if the request context stops at SetListIsFavorite (it
		// was discarded by the RPC method before), or outlives the request
		want := []string{"Raycast", ""}
		assert.Equal(t, want, home.Results.IntegrationNames)
	})

	t.Run("archive and restore through the API: the Bin collection changes are stamped", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, objects)
		bin := linksRoot("bin")
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Archive: "bin"})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			if objectId == "bin" {
				return editor.NewArchive(bin, fx.store.SpaceIndex(spaceId)), nil
			}
			return smarttest.New(objectId), nil
		})

		// when: DELETE then restore through the API, then an app archives
		require.NoError(t, fx.SetIsArchived(nil, apiCtx, "obj1", true, false))
		require.NoError(t, fx.SetIsArchived(nil, apiCtx, "obj1", false, false))
		require.NoError(t, fx.SetIsArchived(nil, appCtx, "obj2", true, false))

		// then: red if the Bin collection builds its state without the
		// request — an API archive would leave no attributed change at all,
		// since the archived object itself gets none
		want := []string{"Raycast", "Raycast", ""}
		assert.Equal(t, want, bin.Results.IntegrationNames)
	})

	t.Run("an archive names the app for the objects it asked for, not for the GC's cascade", func(t *testing.T) {
		// given: archiving obj1 makes the GC archive an orphaned attachment
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, append(objects, objectstore.TestObject{
			bundle.RelationKeyId: domain.String("file1"), bundle.RelationKeySpaceId: domain.String(spaceId),
		}))
		fx.Service.(*service).objectGC = gcWithOrphans{fileGCStub: &fileGCStub{}, files: []string{"file1"}}
		bin := linksRoot("bin")
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Archive: "bin"})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			if objectId == "bin" {
				return editor.NewArchive(bin, fx.store.SpaceIndex(spaceId)), nil
			}
			return smarttest.New(objectId), nil
		})

		// when
		require.NoError(t, fx.SetIsArchived(nil, apiCtx, "obj1", true, false))

		// then: two Bin changes — the requested object's, stamped; the
		// cascaded file's, not. Red if the cascade runs on the request
		// context: the app would be named for a file it never mentioned.
		want := []string{"Raycast", ""}
		assert.Equal(t, want, bin.Results.IntegrationNames)
	})
}
