package v2service

// liveness_test.go pins the one definition of "visible space"
// (APIV2_SPACE_STREAM.md §3): the classifier, the pure enumeration, and
// every surface that shares them.

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// statusView is a space view carrying exactly the given statuses.
func statusView(local, account, remote model.SpaceStatus) *domain.Details {
	view := domain.NewDetails()
	view.SetInt64(bundle.RelationKeySpaceLocalStatus, int64(local))
	view.SetInt64(bundle.RelationKeySpaceAccountStatus, int64(account))
	view.SetInt64(bundle.RelationKeySpaceRemoteStatus, int64(remote))
	return view
}

func TestSpaceViewState(t *testing.T) {
	t.Run("live is local Unknown, Loading or Ok and account Unknown or Active, over every status triple", func(t *testing.T) {
		// The oracle is the rule itself, in numbers, not the predicate under
		// test: local ∈ {0,1,2} ∧ account ∈ {0,8}. Any edit that moves a
		// triple in or out of live — Loading included — fails here.
		statuses := len(model.SpaceStatus_name)
		require.Equal(t, 11, statuses, "a new status needs a decision here")
		live := 0
		for local := 0; local < statuses; local++ {
			for account := 0; account < statuses; account++ {
				for remote := 0; remote < statuses; remote++ {
					// given
					view := statusView(model.SpaceStatus(local), model.SpaceStatus(account), model.SpaceStatus(remote))
					want := (local == 0 || local == 1 || local == 2) && (account == 0 || account == 8)

					// when
					state := spaceViewState(view)

					// then
					triple := fmt.Sprintf("local=%d account=%d remote=%d", local, account, remote)
					assert.Equal(t, want, state == spaceViewLive, triple)
					assert.Equal(t, want, isLiveSpaceView(view), triple)
					if want {
						live++
					}
					// what is not live is gone exactly when the account no
					// longer holds the space: account Deleted (7) or Removing
					// (10), or local Missing (3) with remote RemoteDeleted (6)
					wantGone := account == 7 || account == 10 || (local == 3 && remote == 6)
					if !want {
						assert.Equal(t, wantGone, state == spaceViewGone, triple)
						assert.Equal(t, !wantGone, state == spaceViewUnavailable, triple)
					}
				}
			}
		}
		assert.Equal(t, 3*2*statuses, live)
	})

	t.Run("a legacy view with no status at all is live", func(t *testing.T) {
		assert.Equal(t, spaceViewLive, spaceViewState(domain.NewDetails()))
	})

	t.Run("the states each writer leaves behind", func(t *testing.T) {
		const (
			unknown  = model.SpaceStatus_Unknown
			loading  = model.SpaceStatus_Loading
			ok       = model.SpaceStatus_Ok
			missing  = model.SpaceStatus_Missing
			active   = model.SpaceStatus_SpaceActive
			joining  = model.SpaceStatus_SpaceJoining
			deleted  = model.SpaceStatus_SpaceDeleted
			removing = model.SpaceStatus_SpaceRemoving
		)
		for _, tc := range []struct {
			name                   string
			local, account, remote model.SpaceStatus
			want                   spaceViewLiveness
		}{
			{"created, before its first load", unknown, unknown, unknown, spaceViewLive},
			{"loading", loading, active, unknown, spaceViewLive},
			{"loaded", ok, active, ok, spaceViewLive},
			{"deleted or left through the RPC", ok, deleted, ok, spaceViewGone},
			{"deleted, then offloaded", missing, deleted, unknown, spaceViewGone},
			{"leaving in progress", ok, removing, ok, spaceViewGone},
			{"the loader found the space deleted on the network", missing, active, model.SpaceStatus_RemoteDeleted, spaceViewGone},
			{"deletion pending on the network", missing, active, model.SpaceStatus_RemoteWaitingDeletion, spaceViewUnavailable},
			{"a load that failed for good", missing, active, unknown, spaceViewUnavailable},
			{"still joining", unknown, joining, unknown, spaceViewUnavailable},
			{"a local status no writer writes", model.SpaceStatus_Error, active, unknown, spaceViewUnavailable},
		} {
			assert.Equal(t, tc.want, spaceViewState(statusView(tc.local, tc.account, tc.remote)), tc.name)
		}
	})
}

// permutations calls visit with every ordering of views.
func permutations(views []*domain.Details, visit func([]*domain.Details)) {
	var walk func(int)
	walk = func(k int) {
		if k == len(views) {
			visit(append([]*domain.Details(nil), views...))
			return
		}
		for i := k; i < len(views); i++ {
			views[k], views[i] = views[i], views[k]
			walk(k + 1)
			views[k], views[i] = views[i], views[k]
		}
	}
	walk(0)
}

func TestVisibleSpaces(t *testing.T) {
	const tech = objectstore.TestTechSpaceId
	view := func(viewId, target, name string, statuses ...model.SpaceStatus) *domain.Details {
		d := domain.NewDetails()
		d.SetString(bundle.RelationKeyId, viewId)
		if target != "" {
			d.SetString(bundle.RelationKeyTargetSpaceId, target)
		}
		d.SetString(bundle.RelationKeyName, name)
		keys := []domain.RelationKey{bundle.RelationKeySpaceLocalStatus, bundle.RelationKeySpaceAccountStatus}
		for i, status := range statuses {
			d.SetInt64(keys[i], int64(status))
		}
		return d
	}
	// Every class the list and the stream could disagree on, in one set:
	//   A      a legacy view with no statuses
	//   B      two views of one target, the dead one sorting first by view id
	//   C      two LIVE views of one target: the first by view id names the row
	//   dead   a space whose load failed
	//   load   a space that is still loading
	//   tech   a live view whose target is the tech space
	//   and a view with no target at all
	views := func() []*domain.Details {
		withDetails := view("view-a", "spaceA", "A")
		withDetails.SetString(bundle.RelationKeyDescription, "about A")
		withDetails.SetString(bundle.RelationKeyIconImage, "iconA")
		return []*domain.Details{
			withDetails,
			view("view-b-1", "spaceB", "B dead", model.SpaceStatus_Ok, model.SpaceStatus_SpaceDeleted),
			view("view-b-2", "spaceB", "B live", model.SpaceStatus_Ok, model.SpaceStatus_SpaceActive),
			view("view-c-1", "spaceC", "C first", model.SpaceStatus_Ok, model.SpaceStatus_SpaceActive),
			view("view-c-2", "spaceC", "C second", model.SpaceStatus_Ok, model.SpaceStatus_SpaceActive),
			view("view-load", "spaceLoad", "Loading", model.SpaceStatus_Loading, model.SpaceStatus_SpaceActive),
			view("view-tech", tech, "Tech", model.SpaceStatus_Ok, model.SpaceStatus_SpaceActive),
			// the two that never produce a row
			view("view-dead", "spaceDead", "Dead", model.SpaceStatus_Missing, model.SpaceStatus_SpaceActive),
			view("view-none", "", "No target"),
		}
	}
	rowA := v2model.SpaceRow{Id: "spaceA", Name: "A", Description: "about A", IconImage: "iconA"}
	rowB := v2model.SpaceRow{Id: "spaceB", Name: "B live"}
	rowC := v2model.SpaceRow{Id: "spaceC", Name: "C first"}
	rowLoad := v2model.SpaceRow{Id: "spaceLoad", Name: "Loading"}
	rowTech := v2model.SpaceRow{Id: tech, Name: "Tech"}

	for _, tc := range []struct {
		name     string
		grant    *util.ApiGrant
		wantRows []v2model.SpaceRow
		wantFlag bool
	}{
		{
			name:     "an unscoped key sees every live space, the tech target included",
			grant:    nil,
			wantRows: []v2model.SpaceRow{rowA, rowB, rowC, rowLoad, rowTech},
		},
		{
			name:     "all spaces, read: every user space, never the tech space, and the tech space sets no flag",
			grant:    &util.ApiGrant{AllSpaces: true, Perms: util.GrantPermsRead},
			wantRows: []v2model.SpaceRow{rowA, rowB, rowC, rowLoad},
		},
		{
			name:     "all spaces, readwrite: the same set",
			grant:    &util.ApiGrant{AllSpaces: true, Perms: util.GrantPermsReadWrite},
			wantRows: []v2model.SpaceRow{rowA, rowB, rowC, rowLoad},
		},
		{
			name:     "a list naming a dead space and the tech space: the dead one stays out, the tech one is admitted",
			grant:    &util.ApiGrant{Spaces: []string{"spaceA", "spaceDead", tech}, Perms: util.GrantPermsRead},
			wantRows: []v2model.SpaceRow{rowA, rowTech},
			wantFlag: true,
		},
		{
			name:     "a list whose only non-granted spaces are dead or the tech space hides nothing",
			grant:    &util.ApiGrant{Spaces: []string{"spaceA", "spaceB", "spaceC", "spaceLoad"}, Perms: util.GrantPermsRead},
			wantRows: []v2model.SpaceRow{rowA, rowB, rowC, rowLoad},
		},
		{
			name:     "an explicit tech grant admits the tech target and hides the rest",
			grant:    &util.ApiGrant{Spaces: []string{tech}, Perms: util.GrantPermsRead},
			wantRows: []v2model.SpaceRow{rowTech},
			wantFlag: true,
		},
		{
			name:     "a grant naming no visible space sees nothing and says so",
			grant:    &util.ApiGrant{Spaces: []string{"spaceElsewhere"}, Perms: util.GrantPermsRead},
			wantRows: []v2model.SpaceRow{},
			wantFlag: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			orders := 0
			// every input order of the seven views that can win or lose a
			// row — the answer may depend on neither the store's order nor a
			// map's — with the two that never produce one at either end
			all := views()
			core, edge := all[:7], all[7:]
			require.Len(t, edge, 2)
			permutations(core, func(ordered []*domain.Details) {
				orders++
				if orders%2 == 0 {
					ordered = append(append([]*domain.Details{}, edge...), ordered...)
				} else {
					ordered = append(ordered, edge...)
				}
				// when
				rows, flag := visibleSpaces(ordered, tc.grant, tech)

				// then
				if !assert.ObjectsAreEqual(tc.wantRows, rows) || flag != tc.wantFlag {
					var order []string
					for _, v := range ordered {
						order = append(order, v.GetString(bundle.RelationKeyId))
					}
					require.Equal(t, tc.wantRows, rows, "input order %v", order)
					require.Equal(t, tc.wantFlag, flag, "input order %v", order)
				}
			})
			assert.Equal(t, 5040, orders)
		})
	}

	t.Run("the input is not reordered", func(t *testing.T) {
		// given
		in := views()
		first := in[0]

		// when
		visibleSpaces(in, nil, tech)

		// then
		assert.Same(t, first, in[0])
	})
}

// loadingFixture registers three spaces with shaped ids — one still
// loading, one loaded, one whose load failed — each holding one type and one
// object. Wherever the loading space is asserted to be served, the loaded
// one must be served the same way and the failed one refused: a surface
// that admits everything, or nothing, passes neither.
const (
	loadingSpaceId = realSpaceTracker
	loadedSpaceId  = realSpacePersonal
	failedSpaceId  = realSpaceEval
)

func newLoadingFixture(t *testing.T) *v2Fixture {
	fx := newV2FixtureBare(t)
	for _, sp := range []struct {
		id, name string
		local    model.SpaceStatus
	}{
		{loadingSpaceId, "Loading", model.SpaceStatus_Loading},
		{loadedSpaceId, "Loaded", model.SpaceStatus_Ok},
		{failedSpaceId, "Failed", model.SpaceStatus_Missing},
	} {
		fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                 domain.String("spaceView_" + sp.name),
			bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:      domain.String(sp.id),
			bundle.RelationKeyName:               domain.String(sp.name),
			bundle.RelationKeySpaceLocalStatus:   domain.Int64(int64(sp.local)),
			bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(model.SpaceStatus_SpaceActive)),
		}})
		fx.objectStore.AddObjects(t, sp.id, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("type-page-" + sp.name),
				bundle.RelationKeyName:           domain.String("Page"),
				bundle.RelationKeyUniqueKey:      domain.String("ot-page"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:               domain.String("note-" + sp.name),
				bundle.RelationKeyName:             domain.String("Note in " + sp.name),
				bundle.RelationKeyType:             domain.String("type-page-" + sp.name),
				bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyLastModifiedDate: domain.Int64(100),
			},
		})
	}
	return fx
}

func requireSpaceUnavailable(t *testing.T, err error) {
	t.Helper()
	apiErr := v2Err(t, err)
	assert.Equal(t, http.StatusNotFound, apiErr.Status)
	assert.Contains(t, apiErr.Message, "is not available")
}

func TestLoadingSpaceIsLive(t *testing.T) {
	full := CtxWithFullIds(context.Background())

	t.Run("the list serves the loading space beside the loaded one", func(t *testing.T) {
		// given
		fx := newLoadingFixture(t)
		want := []v2model.SpaceRow{{Id: loadingSpaceId, Name: "Loading"}, {Id: loadedSpaceId, Name: "Loaded"}}

		// when
		rows, total, _, _, err := fx.ListSpaces(full, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows)
		assert.Equal(t, 2, total)
	})

	t.Run("GET-one serves it", func(t *testing.T) {
		// given
		fx := newLoadingFixture(t)

		// when
		loading, loadingErr := fx.GetSpace(full, loadingSpaceId)
		loaded, loadedErr := fx.GetSpace(full, loadedSpaceId)
		_, failedErr := fx.GetSpace(full, failedSpaceId)

		// then
		require.NoError(t, loadingErr)
		require.NoError(t, loadedErr)
		assert.Equal(t, v2model.Space{Id: loadingSpaceId, Name: "Loading"}, loading)
		assert.Equal(t, v2model.Space{Id: loadedSpaceId, Name: "Loaded"}, loaded)
		requireSpaceUnavailable(t, failedErr)
	})

	t.Run("a nested list passes the space check", func(t *testing.T) {
		// given
		fx := newLoadingFixture(t)

		// when
		loading, _, _, loadingErr := fx.ListTypes(full, loadingSpaceId, 0, 25)
		loaded, _, _, loadedErr := fx.ListTypes(full, loadedSpaceId, 0, 25)
		_, _, _, failedErr := fx.ListTypes(full, failedSpaceId, 0, 25)

		// then
		require.NoError(t, loadingErr)
		require.NoError(t, loadedErr)
		assert.Equal(t, []v2model.TypeRow{{Key: "page", Name: "Page"}}, loading)
		assert.Equal(t, loaded, loading)
		requireSpaceUnavailable(t, failedErr)
	})

	t.Run("its short reference resolves", func(t *testing.T) {
		// given
		fx := newLoadingFixture(t)
		tail := func(id string) string { return spaceIdTail(id) }

		// when
		loading, loadingErr := fx.ResolveSpaceRef(context.Background(), tail(loadingSpaceId))
		loaded, loadedErr := fx.ResolveSpaceRef(context.Background(), tail(loadedSpaceId))
		failed, failedErr := fx.ResolveSpaceRef(context.Background(), tail(failedSpaceId))

		// then
		require.NoError(t, loadingErr)
		require.NoError(t, loadedErr)
		require.NoError(t, failedErr)
		assert.Equal(t, loadingSpaceId, loading)
		assert.Equal(t, loadedSpaceId, loaded)
		assert.Equal(t, tail(failedSpaceId), failed, "a space that is not visible does not resolve: the reference comes back unchanged")
	})

	t.Run("it is served by its short reference", func(t *testing.T) {
		// given
		fx := newLoadingFixture(t)

		// when
		got, err := fx.GetSpace(context.Background(), loadingSpaceId)

		// then
		require.NoError(t, err)
		assert.Equal(t, spaceIdTail(loadingSpaceId), got.Id)
		assert.NotEqual(t, loadingSpaceId, got.Id, "the fixture's ids must shorten, or the spelling axis is not tested")
	})

	t.Run("global search fans out to it", func(t *testing.T) {
		// given
		fx := newLoadingFixture(t)

		// when
		rows, total, _, _, err := fx.GlobalSearchObjects(full, v2model.SearchRequest{}, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.ElementsMatch(t, []string{"note-Loading", "note-Loaded"}, rowIds(rows))
	})
}

func TestStoreReadsWarnWhileLoading(t *testing.T) {
	full := CtxWithFullIds(context.Background())
	warning := func(spelling string) []v2model.Issue {
		return []v2model.Issue{{
			Path:    "space_id",
			Message: `space "` + spelling + `" is still loading on this device; results can be incomplete`,
			Hint:    "repeat the request once the space has loaded for the complete result",
		}}
	}

	t.Run("a loading space warns, a loaded one does not, a failed one is not a warning's business", func(t *testing.T) {
		// given
		fx := newLoadingFixture(t)

		// when / then
		assert.Equal(t, warning(loadingSpaceId), fx.LoadingSpaceWarnings(full, loadingSpaceId))
		assert.Equal(t, warning(spaceIdTail(loadingSpaceId)), fx.LoadingSpaceWarnings(context.Background(), loadingSpaceId),
			"the space is named the way this request is served it")
		assert.Empty(t, fx.LoadingSpaceWarnings(full, loadedSpaceId))
		assert.Empty(t, fx.LoadingSpaceWarnings(full, failedSpaceId))
		assert.Empty(t, fx.LoadingSpaceWarnings(full, "no-such-space"))
		assert.Empty(t, fx.LoadingSpaceWarnings(full, objectstore.TestTechSpaceId))
	})

	t.Run("a space search carries the warning for the loading space only", func(t *testing.T) {
		// given
		fx := newLoadingFixture(t)

		// when
		loadingRows, _, _, loadingWarnings, loadingErr := fx.SearchObjects(full, loadingSpaceId, v2model.SearchRequest{}, 0, 25)
		_, _, _, loadedWarnings, loadedErr := fx.SearchObjects(full, loadedSpaceId, v2model.SearchRequest{}, 0, 25)

		// then
		require.NoError(t, loadingErr)
		require.NoError(t, loadedErr)
		assert.Equal(t, []string{"note-Loading"}, rowIds(loadingRows), "what is indexed so far is served")
		assert.Equal(t, warning(loadingSpaceId), loadingWarnings)
		assert.Empty(t, loadedWarnings)
	})

	t.Run("a global search names each loading space it read", func(t *testing.T) {
		// given
		fx := newLoadingFixture(t)

		// when
		_, _, _, warnings, err := fx.GlobalSearchObjects(full, v2model.SearchRequest{}, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, warning(loadingSpaceId), warnings)
	})

	t.Run("a global search of loaded spaces only carries no such warning", func(t *testing.T) {
		// given: the key is granted the loaded space alone
		fx := newLoadingFixture(t)
		ctx := grantCtx(util.GrantPermsRead, loadedSpaceId)

		// when
		_, _, _, warnings, err := fx.GlobalSearchObjects(ctx, v2model.SearchRequest{}, 0, 25)

		// then: and a loading space the key cannot see is not disclosed
		require.NoError(t, err)
		assert.Empty(t, warnings)
	})
}

func TestJudgedSpaceView(t *testing.T) {
	view := func(viewId string, local, account model.SpaceStatus) *domain.Details {
		d := statusView(local, account, model.SpaceStatus_Unknown)
		d.SetString(bundle.RelationKeyId, viewId)
		return d
	}
	const (
		ok      = model.SpaceStatus_Ok
		active  = model.SpaceStatus_SpaceActive
		deleted = model.SpaceStatus_SpaceDeleted
	)
	for _, tc := range []struct {
		name  string
		views []*domain.Details
		want  string
	}{
		{"the only view", []*domain.Details{view("v1", ok, active)}, "v1"},
		{"a live view wins over a dead one that sorts first", []*domain.Details{view("v1", ok, deleted), view("v2", ok, active)}, "v2"},
		{"of two live views the first by view id", []*domain.Details{view("v1", ok, active), view("v2", ok, active)}, "v1"},
		{"of two dead views the first by view id", []*domain.Details{view("v1", ok, deleted), view("v2", ok, deleted)}, "v1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// every input order: the store's order must not decide
			permutations(tc.views, func(ordered []*domain.Details) {
				got := judgedSpaceView(ordered)
				require.NotNil(t, got)
				assert.Equal(t, tc.want, got.GetString(bundle.RelationKeyId))
			})
		})
	}

	t.Run("no view at all", func(t *testing.T) {
		assert.Nil(t, judgedSpaceView(nil))
	})
}

// TestDoubledSpaceViewIsJudgedAsTheListServesIt: the list and the spaces
// stream skip a dead view of a target when a live one follows it. GET-one
// and every nested route must judge the space by that same view, or a space
// the list just served answers 404.
func TestDoubledSpaceViewIsJudgedAsTheListServesIt(t *testing.T) {
	// given: two views of one target, the dead one sorting first and stored first
	fx := newV2FixtureBare(t)
	for _, v := range []struct {
		id      string
		name    string
		account model.SpaceStatus
	}{
		{"view-1", "Dead twin", model.SpaceStatus_SpaceDeleted},
		{"view-2", "Live twin", model.SpaceStatus_SpaceActive},
	} {
		fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                 domain.String(v.id),
			bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:      domain.String("doubled"),
			bundle.RelationKeyName:               domain.String(v.name),
			bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(v.account)),
		}})
	}
	want := v2model.Space{Id: "doubled", Name: "Live twin"}

	// when
	rows, _, _, _, listErr := fx.ListSpaces(context.Background(), 0, 25)
	got, getErr := fx.GetSpace(context.Background(), "doubled")
	_, _, _, nestedErr := fx.ListTypes(context.Background(), "doubled", 0, 25)

	// then
	require.NoError(t, listErr)
	assert.Equal(t, []v2model.SpaceRow{{Id: "doubled", Name: "Live twin"}}, rows)
	require.NoError(t, getErr)
	assert.Equal(t, want, got)
	require.NoError(t, nestedErr)
}

// TestLoadingWarningFollowsTheServedView: with two views of one target, the
// space is the view it is served by. A Loading view that does not name the
// row must not make a global search warn that the space is loading.
func TestLoadingWarningFollowsTheServedView(t *testing.T) {
	register := func(t *testing.T, firstLocal, secondLocal model.SpaceStatus) *v2Fixture {
		fx := newV2FixtureBare(t)
		for _, v := range []struct {
			id    string
			local model.SpaceStatus
		}{{"view-1", firstLocal}, {"view-2", secondLocal}} {
			fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:                 domain.String(v.id),
				bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
				bundle.RelationKeyTargetSpaceId:      domain.String("doubled"),
				bundle.RelationKeySpaceLocalStatus:   domain.Int64(int64(v.local)),
				bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(model.SpaceStatus_SpaceActive)),
			}})
		}
		fx.objectStore.AddObjects(t, "doubled", []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("note"),
			bundle.RelationKeyName:           domain.String("Note"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})
		return fx
	}

	t.Run("the served view is loaded: no warning, whatever the other view says", func(t *testing.T) {
		// given: the first view by id is Ok, the second Loading
		fx := register(t, model.SpaceStatus_Ok, model.SpaceStatus_Loading)

		// when
		rows, _, _, warnings, err := fx.GlobalSearchObjects(context.Background(), v2model.SearchRequest{}, 0, 25)

		// then
		require.NoError(t, err)
		require.Equal(t, []string{"note"}, rowIds(rows))
		assert.Empty(t, warnings)
		assert.Empty(t, fx.LoadingSpaceWarnings(context.Background(), "doubled"), "the single-space warning judges the same view")
	})

	t.Run("the served view is loading: the warning is there", func(t *testing.T) {
		// given: the other way round
		fx := register(t, model.SpaceStatus_Loading, model.SpaceStatus_Ok)

		// when
		_, _, _, warnings, err := fx.GlobalSearchObjects(context.Background(), v2model.SearchRequest{}, 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, "is still loading on this device")
		assert.Len(t, fx.LoadingSpaceWarnings(context.Background(), "doubled"), 1)
	})
}

// TestDryRunsOfALoadingSpaceSayTheIndexMayBeIncomplete: a dry run of a write
// that validates against the index does not wait for a loading space, so
// what it concludes from the index — a refusal or a result — carries the
// store-read warning. The loaded space beside it is the control: a warning
// attached unconditionally fails there.
func TestDryRunsOfALoadingSpaceSayTheIndexMayBeIncomplete(t *testing.T) {
	bg := context.Background()
	name := "Renamed"
	loadingIssue := func(spaceId string) v2model.Issue {
		return v2model.Issue{
			Path:    "space_id",
			Message: `space "` + spaceIdTail(spaceId) + `" is still loading on this device; results can be incomplete`,
			Hint:    "repeat the request once the space has loaded for the complete result",
		}
	}
	// both spaces hold the same index: one type ("page") and one property
	withProperty := func(t *testing.T) *v2Fixture {
		fx := newLoadingFixture(t)
		for _, spaceId := range []string{loadingSpaceId, loadedSpaceId} {
			fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:             domain.String("rel-severity"),
				bundle.RelationKeyRelationKey:    domain.String("severity"),
				bundle.RelationKeyName:           domain.String("Severity"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			}})
		}
		return fx
	}

	refusals := map[string]func(fx *v2Fixture, spaceId string) error{
		"delete an unknown type": func(fx *v2Fixture, spaceId string) error {
			_, err := fx.DeleteType(bg, spaceId, "no_such_type", true)
			return err
		},
		"update an unknown property": func(fx *v2Fixture, spaceId string) error {
			_, err := fx.UpdateProperty(bg, spaceId, "no_such_property", v2model.UpdatePropertyRequest{Name: &name}, true)
			return err
		},
		"delete an unknown property": func(fx *v2Fixture, spaceId string) error {
			_, err := fx.DeleteProperty(bg, spaceId, "no_such_property", true)
			return err
		},
		"create a property whose key is taken": func(fx *v2Fixture, spaceId string) error {
			_, err := fx.CreateProperty(bg, spaceId, v2model.CreatePropertyRequest{Key: "severity", Name: "Severity again", Format: "select"}, true)
			return err
		},
		"create a property whose name is taken": func(fx *v2Fixture, spaceId string) error {
			_, err := fx.CreateProperty(bg, spaceId, v2model.CreatePropertyRequest{Name: "Severity", Format: "select"}, true)
			return err
		},
		"create a collection of an unknown object": func(fx *v2Fixture, spaceId string) error {
			_, err := fx.CreateCollection(bg, spaceId, v2model.CreateCollectionRequest{Name: "Shelf", Items: []string{"no_such_object"}}, true)
			return err
		},
	}
	for label, call := range refusals {
		t.Run("refusal: "+label, func(t *testing.T) {
			// given
			fx := withProperty(t)

			// when
			loading := v2Err(t, call(fx, loadingSpaceId))
			loaded := v2Err(t, call(fx, loadedSpaceId))

			// then: the same refusal, and only the loading space's says the
			// index it was read from may be incomplete
			assert.Equal(t, loaded.Status, loading.Status)
			assert.Equal(t, loaded.Code, loading.Code)
			require.NotEmpty(t, loading.Issues)
			assert.Equal(t, loadingIssue(loadingSpaceId), loading.Issues[len(loading.Issues)-1])
			assert.Len(t, loading.Issues, len(loaded.Issues)+1)
			for _, issue := range loaded.Issues {
				assert.NotContains(t, issue.Message, "still loading", "a loaded space carries no such warning")
			}
		})
	}

	results := map[string]func(fx *v2Fixture, spaceId string) (*v2model.CreateResult, error){
		"delete type": func(fx *v2Fixture, spaceId string) (*v2model.CreateResult, error) {
			return fx.DeleteType(bg, spaceId, "page", true)
		},
		"update property": func(fx *v2Fixture, spaceId string) (*v2model.CreateResult, error) {
			return fx.UpdateProperty(bg, spaceId, "severity", v2model.UpdatePropertyRequest{Name: &name}, true)
		},
		"delete property": func(fx *v2Fixture, spaceId string) (*v2model.CreateResult, error) {
			return fx.DeleteProperty(bg, spaceId, "severity", true)
		},
		"create property": func(fx *v2Fixture, spaceId string) (*v2model.CreateResult, error) {
			return fx.CreateProperty(bg, spaceId, v2model.CreatePropertyRequest{Name: "Effort", Format: "number"}, true)
		},
	}
	for label, call := range results {
		t.Run("result: "+label, func(t *testing.T) {
			// given
			fx := withProperty(t)

			// when
			loading, loadingErr := call(fx, loadingSpaceId)
			loaded, loadedErr := call(fx, loadedSpaceId)

			// then
			require.NoError(t, loadingErr)
			require.NoError(t, loadedErr)
			require.True(t, loading.DryRun)
			require.True(t, loaded.DryRun)
			assert.Contains(t, loading.Warnings, loadingIssue(loadingSpaceId))
			assert.Len(t, loading.Warnings, len(loaded.Warnings)+1)
			for _, issue := range loaded.Warnings {
				assert.NotContains(t, issue.Message, "still loading")
			}
		})
	}

	t.Run("a refusal of the request itself carries no such warning, loading or not", func(t *testing.T) {
		// given
		fx := withProperty(t)

		// when: a dry run with a bad body
		_, err := fx.CreateProperty(bg, loadingSpaceId, v2model.CreatePropertyRequest{Name: "Effort", Format: "nope"}, true)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		for _, issue := range apiErr.Issues {
			assert.NotContains(t, issue.Message, "still loading")
		}
	})
}
