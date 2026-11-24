package api

// spacesstream_integration_test.go is the anti-drift test of the spaces
// stream (APIV2_SPACE_STREAM.md §7): the stream, the spaces list and the
// per-space gate answer "which spaces can this key see" from one store,
// through the real server, the real object-search adapter and the real
// subscription engine. Nothing here is fed by the test to both sides: a
// mocked live search would be.
//
// Expectations are derived from the stream's own output. The fixture carries
// every class the three could disagree on — a loading space, a failed one,
// one deleted on the network, a left one, a deleted and an archived view, two
// views of one target, a view of the tech space, legacy views with no status
// — on space ids in the production shape, two of which share a tail.

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const streamReplKey = ".28y6mgnwgodt7"

// Space ids in the production shape. Work and Twin share their last six
// characters, so the list serves both in full by default; every other one
// shortens — which is what makes "the stream serves full ids" observable.
const (
	spWork     = "bafyreiay4rdeleruyuy6x575hvhtifedmjq4g3ojpffvpbgmuackq3oake" + streamReplKey
	spTwin     = "bafyreiby4rdeleruyuy6x575hvhtifedmjq4g3ojpffvpbgmuackq3oake" + streamReplKey
	spLoading  = "bafyreia4znhhjvxek2iux7enfzilekj5vwlgddgogpfgnx5qnim7bugaxa" + streamReplKey
	spFailed   = "bafyreihwvsaekzzyb54o7um4hdpvpn5b2invn75lmijhhtghblvphxwz2i" + streamReplKey
	spRemote   = "bafyreie3qxj7466htdh75x5jxgh3qpa4e2t75oyplwffdbsjdpijrnjln4" + streamReplKey
	spLeft     = "bafyreic3qxj7466htdh75x5jxgh3qpa4e2t75oyplwffdbsjdpijrnjlna" + streamReplKey
	spLeaving  = "bafyreid3qxj7466htdh75x5jxgh3qpa4e2t75oyplwffdbsjdpijrnjlnb" + streamReplKey
	spDeleted  = "bafyreif3qxj7466htdh75x5jxgh3qpa4e2t75oyplwffdbsjdpijrnjlnc" + streamReplKey
	spArchived = "bafyreig3qxj7466htdh75x5jxgh3qpa4e2t75oyplwffdbsjdpijrnjlnd" + streamReplKey
	spDouble   = "bafyreih3qxj7466htdh75x5jxgh3qpa4e2t75oyplwffdbsjdpijrnjlne" + streamReplKey
	spCreated  = "bafyreii3qxj7466htdh75x5jxgh3qpa4e2t75oyplwffdbsjdpijrnjlnf" + streamReplKey
)

// streamFixtureSpaces is every space id the fixture names, the tech space
// included: the per-id gate check runs over all of them.
var streamFixtureSpaces = []string{
	spWork, spTwin, spLoading, spFailed, spRemote, spLeft, spLeaving, spDeleted, spArchived, spDouble, spCreated,
	objectstore.TestTechSpaceId,
}

func spaceViewObject(viewId, target, name string, extra objectstore.TestObject) objectstore.TestObject {
	view := objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(viewId),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
		bundle.RelationKeyTargetSpaceId:  domain.String(target),
		bundle.RelationKeyName:           domain.String(name),
	}
	for key, value := range extra {
		view[key] = value
	}
	return view
}

func viewStatuses(local, account model.SpaceStatus) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeySpaceLocalStatus:   domain.Int64(int64(local)),
		bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(account)),
	}
}

// spacesStreamOverEngine is the API service bound to a real port over the
// real store, subscription engine and adapter.
type spacesStreamOverEngine struct {
	*objectSearchFixture
	api     *apiService
	baseUrl string
	grants  map[string]*util.ApiGrant
}

func newSpacesStreamOverEngine(t *testing.T) *spacesStreamOverEngine {
	t.Helper()
	search := newObjectSearchFixture(t)
	const (
		ok      = model.SpaceStatus_Ok
		loading = model.SpaceStatus_Loading
		missing = model.SpaceStatus_Missing
		active  = model.SpaceStatus_SpaceActive
		deleted = model.SpaceStatus_SpaceDeleted
	)
	work := spaceViewObject("view-work", spWork, "Work", viewStatuses(ok, active))
	work[bundle.RelationKeyDescription] = domain.String("Team notes")
	work[bundle.RelationKeyIconImage] = domain.String("bafybeiiconwork")
	remote := spaceViewObject("view-remote", spRemote, "Deleted on the network", viewStatuses(missing, active))
	remote[bundle.RelationKeySpaceRemoteStatus] = domain.Int64(int64(model.SpaceStatus_RemoteDeleted))
	deletedView := spaceViewObject("view-deleted", spDeleted, "Deleted view", viewStatuses(ok, active))
	deletedView[bundle.RelationKeyIsDeleted] = domain.Bool(true)
	archivedView := spaceViewObject("view-archived", spArchived, "Archived view", viewStatuses(ok, active))
	archivedView[bundle.RelationKeyIsArchived] = domain.Bool(true)
	search.store.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{
		work,
		spaceViewObject("view-twin", spTwin, "Twin", nil), // a legacy view: no status at all
		spaceViewObject("view-loading", spLoading, "Loading", viewStatuses(loading, active)),
		spaceViewObject("view-failed", spFailed, "Failed", viewStatuses(missing, active)),
		remote,
		spaceViewObject("view-left", spLeft, "Left", viewStatuses(ok, deleted)),
		spaceViewObject("view-leaving", spLeaving, "Leaving", viewStatuses(ok, active)),
		deletedView,
		archivedView,
		// two views of one target, the dead one sorting first by view id
		spaceViewObject("view-double-1", spDouble, "Double, dead", viewStatuses(ok, deleted)),
		spaceViewObject("view-double-2", spDouble, "Double, live", viewStatuses(ok, active)),
		// a live view whose target is the tech space itself
		spaceViewObject("view-tech", objectstore.TestTechSpaceId, "Tech", viewStatuses(ok, active)),
	})

	fx := &spacesStreamOverEngine{
		objectSearchFixture: search,
		grants: map[string]*util.ApiGrant{
			"key-unscoped": nil,
			"key-all":      {AllSpaces: true, Perms: util.GrantPermsRead},
			// a list naming live, loading, dead and absent spaces, and the tech space
			"key-narrow": {Spaces: []string{spWork, spLoading, spFailed, spLeft, spDeleted, spCreated, objectstore.TestTechSpaceId}, Perms: util.GrantPermsRead},
		},
	}
	mw := mock_apicore.NewMockClientCommands(t)
	for key, grant := range fx.grants {
		response := &pb.RpcWalletCreateSessionResponse{
			Token:        "token-of-" + key,
			AppName:      "agent",
			AccountScope: model.AccountAuth_JsonAPI,
			Error:        &pb.RpcWalletCreateSessionResponseError{Code: pb.RpcWalletCreateSessionResponseError_NULL},
		}
		if grant != nil {
			response.Grant = &model.AccountAuthAppGrant{SpaceIds: grant.Spaces, AllSpaces: grant.AllSpaces, Perm: model.AccountAuthAppGrant_Read}
		}
		mw.EXPECT().WalletCreateSession(mock.Anything, &pb.RpcWalletCreateSessionRequest{
			Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{AppKey: key},
		}).Return(response).Maybe()
	}
	accounts := mock_apicore.NewMockAccountService(t)
	accounts.EXPECT().GetInfo(mock.Anything).Return(&model.AccountInfo{TechSpaceId: objectstore.TestTechSpaceId}, nil).Maybe()
	events := mock_apicore.NewMockEventService(t)
	events.EXPECT().Broadcast(mock.Anything).Return().Maybe()
	crossSpace := mock_apicore.NewMockCrossSpaceSubscriptionService(t)
	crossSpace.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{}, nil).Maybe()
	crossSpace.EXPECT().Unsubscribe(mock.Anything).Return(nil).Maybe()

	fx.api = &apiService{
		mw:                   mw,
		accountService:       accounts,
		eventService:         events,
		crossSpaceSubService: crossSpace,
		// the strict reader has no expectations: nothing here reads an object
		objectReader: mock_apicore.NewMockObjectReader(t),
		objectStore:  search.store,
		objectSearch: search.objectSearchAdapter,
	}
	status := fx.api.startServer("127.0.0.1:0")
	require.NotNil(t, status)
	require.True(t, status.Success, status.Error)
	fx.baseUrl = "http://" + status.ListenAddr
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = fx.api.Close(ctx)
	})
	return fx
}

// get answers one JSON request with the given key.
func (fx *spacesStreamOverEngine) get(t *testing.T, key, path string) (int, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fx.baseUrl+path, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, body
}

func (fx *spacesStreamOverEngine) list(t *testing.T, key, query string) v2model.ListSpacesResponse {
	t.Helper()
	status, body := fx.get(t, key, "/v2/spaces?limit=1000"+query)
	require.Equal(t, http.StatusOK, status, string(body))
	var list v2model.ListSpacesResponse
	require.NoError(t, json.Unmarshal(body, &list))
	return list
}

// liveSpacesStream is one open stream and the space set a client would hold
// from it.
type liveSpacesStream struct {
	t      *testing.T
	events chan v2model.SpaceStreamEvent
	held   map[string]v2model.SpaceRow
	flag   bool
	// removals records, per space id, the reason of every space_removed
	removals map[string][]string
}

func (fx *spacesStreamOverEngine) open(t *testing.T, key string) *liveSpacesStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fx.baseUrl+"/v2/spaces/stream", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		cancel()
		_ = resp.Body.Close()
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	stream := &liveSpacesStream{t: t, events: make(chan v2model.SpaceStreamEvent, 1024), held: map[string]v2model.SpaceRow{}, removals: map[string][]string{}}
	go func() {
		defer close(stream.events)
		scanner := bufio.NewScanner(resp.Body)
		var name, data string
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				if name != "" {
					var event v2model.SpaceStreamEvent
					if err := json.Unmarshal([]byte(data), &event); err != nil || event.Type != name {
						event = v2model.SpaceStreamEvent{Type: "malformed frame: " + name + " " + data}
					}
					stream.events <- event
				}
				name, data = "", ""
			case strings.HasPrefix(line, "event: "):
				name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return stream
}

// apply folds one event into the held set, the way a client would.
func (s *liveSpacesStream) apply(event v2model.SpaceStreamEvent) {
	s.t.Helper()
	switch event.Type {
	case v2model.SpaceStreamEventSpaceAdded:
		require.NotNil(s.t, event.Space)
		require.NotContains(s.t, s.held, event.Space.Id, "space_added for a space the client already holds")
		s.held[event.Space.Id] = *event.Space
	case v2model.SpaceStreamEventSpaceUpdated:
		require.NotNil(s.t, event.Space)
		require.Contains(s.t, s.held, event.Space.Id, "space_updated for a space the client does not hold")
		s.held[event.Space.Id] = *event.Space
	case v2model.SpaceStreamEventSpaceRemoved:
		s.removals[event.SpaceId] = append(s.removals[event.SpaceId], event.Reason)
		delete(s.held, event.SpaceId)
	case v2model.SpaceStreamEventSnapshotComplete, v2model.SpaceStreamEventNotGrantedUpdated:
		require.NotNil(s.t, event.HasNotGrantedSpaces)
		s.flag = *event.HasNotGrantedSpaces
	default:
		s.t.Fatalf("unexpected event %+v", event)
	}
}

// snapshot reads the opening snapshot and returns its rows in arrival order.
func (s *liveSpacesStream) snapshot() []v2model.SpaceRow {
	s.t.Helper()
	var rows []v2model.SpaceRow
	for {
		select {
		case event, open := <-s.events:
			require.True(s.t, open, "the stream ended inside its snapshot")
			s.apply(event)
			if event.Type == v2model.SpaceStreamEventSnapshotComplete {
				return rows
			}
			require.Equal(s.t, v2model.SpaceStreamEventSpaceAdded, event.Type, "only space_added precedes snapshot_complete")
			rows = append(rows, *event.Space)
		case <-time.After(10 * time.Second):
			s.t.Fatal("the snapshot did not complete")
		}
	}
}

// heldRows is the held set in the list's order.
func (s *liveSpacesStream) heldRows() []v2model.SpaceRow {
	rows := make([]v2model.SpaceRow, 0, len(s.held))
	for _, row := range s.held {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Id < rows[j].Id })
	return rows
}

// next returns the next event, failing at the deadline or if the stream ended.
func (s *liveSpacesStream) next() v2model.SpaceStreamEvent {
	s.t.Helper()
	select {
	case event, open := <-s.events:
		require.True(s.t, open, "the stream ended")
		return event
	case <-time.After(15 * time.Second):
		s.t.Fatal("no event arrived")
		return v2model.SpaceStreamEvent{}
	}
}

// applyThrough applies events up to and including the space_updated that
// names the space `name` — a rename the caller made AFTER every other
// change. The stream delivers changes in the order they were made, batch by
// batch, but INSIDE one batch its events are in full-id order, so the rename
// can precede events of the same batch whose ids sort after it. A caller
// that needs everything applied therefore renames twice and reads through
// the second: that one is a batch of its own.
func (s *liveSpacesStream) applyThrough(id, name string) {
	s.t.Helper()
	for {
		event := s.next()
		s.apply(event)
		if event.Type == v2model.SpaceStreamEventSpaceUpdated && event.Space.Id == id && event.Space.Name == name {
			return
		}
	}
}

func TestSpacesStreamAgreesWithListAndGate(t *testing.T) {
	for _, key := range []string{"key-unscoped", "key-all", "key-narrow"} {
		t.Run(key, func(t *testing.T) {
			// given
			fx := newSpacesStreamOverEngine(t)
			grant := fx.grants[key]

			// when
			stream := fx.open(t, key)
			snapshot := stream.snapshot()
			seen := map[string]bool{}
			for _, row := range snapshot {
				seen[row.Id] = true
			}
			full := fx.list(t, key, "&ids=full")
			compact := fx.list(t, key, "")

			// then: the snapshot IS the list — ids, names, descriptions, icons,
			// order — and so is the flag
			require.NotEmpty(t, snapshot, "a fixture in which this key sees nothing proves nothing")
			assert.Equal(t, full.Data, snapshot)
			assert.Equal(t, full.HasNotGrantedSpaces, stream.flag)
			assert.Equal(t, full.Total, len(snapshot))
			// the fixture's self-check: the list's default spelling differs
			// from the full one, so a stream serving short references would
			// have failed the comparison above
			assert.NotEqual(t, full.Data, compact.Data, "no id of this fixture shortens: the spelling axis is not tested")

			// and the per-space gate agrees with the stream on every id of the
			// fixture: served iff the stream shows it; when it is not shown,
			// refused as not granted exactly when the grant refuses it
			for _, id := range streamFixtureSpaces {
				status, body := fx.get(t, key, "/v2/spaces/"+id+"?ids=full")
				_, inStream := stream.held[id]
				refusedByGrant := util.SpaceGrantRefusal(grant, id, objectstore.TestTechSpaceId) != ""
				switch {
				case inStream:
					assert.Equal(t, http.StatusOK, status, "%s is in the stream: %s", id, body)
				case refusedByGrant:
					assert.Equal(t, http.StatusForbidden, status, "%s: %s", id, body)
					assert.Contains(t, string(body), v2model.CodeSpaceNotGranted, id)
				default:
					assert.Equal(t, http.StatusNotFound, status, "%s is granted and not in the stream: %s", id, body)
				}
			}

			// when: the live phase —
			const (
				ok      = model.SpaceStatus_Ok
				loading = model.SpaceStatus_Loading
				missing = model.SpaceStatus_Missing
				active  = model.SpaceStatus_SpaceActive
			)
			tech := objectstore.TestTechSpaceId
			// a failed load is retried and succeeds: Missing → Loading → Ok
			fx.store.AddObjects(t, tech, []objectstore.TestObject{spaceViewObject("view-failed", spFailed, "Failed", viewStatuses(loading, active))})
			fx.store.AddObjects(t, tech, []objectstore.TestObject{spaceViewObject("view-failed", spFailed, "Recovered", viewStatuses(ok, active))})
			// a space is left: Active → Deleted
			fx.store.AddObjects(t, tech, []objectstore.TestObject{spaceViewObject("view-leaving", spLeaving, "Leaving", viewStatuses(ok, model.SpaceStatus_SpaceDeleted))})
			// a view is deleted
			gone := spaceViewObject("view-twin", spTwin, "Twin", nil)
			gone[bundle.RelationKeyIsDeleted] = domain.Bool(true)
			fx.store.AddObjects(t, tech, []objectstore.TestObject{gone})
			// a space is created, still loading
			fx.store.AddObjects(t, tech, []objectstore.TestObject{spaceViewObject("view-created", spCreated, "Created", viewStatuses(loading, active))})
			// and one's load fails
			fx.store.AddObjects(t, tech, []objectstore.TestObject{spaceViewObject("view-loading", spLoading, "Loading", viewStatuses(missing, active))})

			// and, last, a space every key of this fixture sees is renamed,
			// twice: once the second rename is out, everything above has
			// been delivered — with a deadline, without a sleep, and without
			// stopping early on a held set that merely passes through the
			// expected one
			for _, name := range []string{"Work, renamed once", "Work, renamed"} {
				fx.store.AddObjects(t, tech, []objectstore.TestObject{spaceViewObject("view-work", spWork, name, viewStatuses(ok, active))})
				stream.applyThrough(spWork, name)
			}

			// then: what the client holds is the list again
			after := fx.list(t, key, "&ids=full")
			require.NotEqual(t, full.Data, after.Data, "the live phase must change what this key sees")
			assert.Equal(t, after.Data, stream.heldRows())
			assert.Equal(t, after.HasNotGrantedSpaces, stream.flag)

			// every space that left did so once, for the reason its change
			// has — and only the spaces this key held can leave
			wantRemovals := map[string][]string{}
			for id, reason := range map[string]string{
				spLeaving: v2model.SpaceRemovedGone,        // the account left it
				spTwin:    v2model.SpaceRemovedGone,        // its view was deleted: a removal with no details
				spLoading: v2model.SpaceRemovedUnavailable, // its load failed
			} {
				if _, held := seen[id]; held {
					wantRemovals[id] = []string{reason}
				}
			}
			require.NotEmpty(t, wantRemovals, "a key that loses nothing in the live phase tests no reason")
			assert.Equal(t, wantRemovals, stream.removals)

			// and nothing else is queued: one more rename, and the very next
			// event is that rename
			again := spaceViewObject("view-work", spWork, "Work, renamed again", viewStatuses(ok, active))
			fx.store.AddObjects(t, tech, []objectstore.TestObject{again})
			last := stream.next()
			require.Equal(t, v2model.SpaceStreamEventSpaceUpdated, last.Type, "an extra frame followed the live phase: %+v", last)
			require.NotNil(t, last.Space)
			assert.Equal(t, spWork, last.Space.Id)
			assert.Equal(t, "Work, renamed again", last.Space.Name)
		})
	}
}

// TestSpacesStreamEndsWhenTheServerIsReplaced is R6 end to end: a rebind
// shuts the old server down, and http.Server.Shutdown does not interrupt an
// open stream — the API service has to end it, or the stream would outlive
// the only server RevokeToken can reach.
func TestSpacesStreamEndsWhenTheServerIsReplaced(t *testing.T) {
	for name, replace := range map[string]func(t *testing.T, fx *spacesStreamOverEngine){
		// The stream's connection is still active when the shutdown grace
		// passes; the rebind must succeed and bind all the same.
		"rebind": func(t *testing.T, fx *spacesStreamOverEngine) {
			status, err := fx.api.ReassignAddress(context.Background(), "127.0.0.1:0")
			require.NoError(t, err)
			require.NotNil(t, status)
			require.True(t, status.Success, status.Error)
			assert.True(t, dialSucceeds(t, status.ListenAddr), "the new address must be reachable")
		},
		"stop": func(t *testing.T, fx *spacesStreamOverEngine) {
			require.NoError(t, fx.api.Close(context.Background()))
		},
	} {
		t.Run(name, func(t *testing.T) {
			// given
			fx := newSpacesStreamOverEngine(t)
			stream := fx.open(t, "key-all")
			stream.snapshot()

			// when
			replace(t, fx)

			// then: the body ends, with no closing event
			select {
			case event, open := <-stream.events:
				require.False(t, open, "an event followed where the stream had to end: %+v", event)
			case <-time.After(10 * time.Second):
				t.Fatal("the stream outlived the server it was accepted by")
			}
		})
	}
}
