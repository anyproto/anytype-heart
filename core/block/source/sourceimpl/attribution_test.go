package sourceimpl

// Attribution tests at the source level (docs/ApiChangeAttribution.md):
// lastModifiedVia as BuildState derives it from the tree, and the store
// change stamp on the wire and on its way to a handler. Each test names the
// implementation change that turns it red.

import (
	"context"
	"testing"

	anystore "github.com/anyproto/any-store/v2"
	"github.com/anyproto/any-store/v2/anyenc"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// viaTree is the slice of a readable object tree BuildState consumes: a
// header that says "page", a root id, and the changes in tree order.
type viaTree struct {
	objecttree.ReadableObjectTree
	id      string
	rootId  string // the change the tree starts from: id itself, or a snapshot
	changes []*objecttree.Change
}

func (v viaTree) Id() string { return v.id }

func (v viaTree) Root() *objecttree.Change { return &objecttree.Change{Id: v.rootId} }

func (v viaTree) Header() *treechangeproto.RawTreeChangeWithId {
	payload, _ := (&model.ObjectChangePayload{SmartBlockType: model.SmartBlockType_Page}).Marshal()
	root, _ := (&treechangeproto.RootChange{ChangePayload: payload, ChangeType: spacedomain.ChangeType}).MarshalVT()
	raw, _ := (&treechangeproto.RawTreeChange{Payload: root}).MarshalVT()
	return &treechangeproto.RawTreeChangeWithId{Id: v.id, RawChange: raw}
}

func (v viaTree) IterateFrom(id string, _ objecttree.ChangeConvertFunc, iterate objecttree.ChangeIterateFunc) error {
	started := false
	for _, change := range v.changes {
		if change.Id == id {
			started = true
		}
		if started && !iterate(change) {
			return nil
		}
	}
	return nil
}

func TestBuildState_LastModifiedVia(t *testing.T) {
	const (
		spaceId  = "space1"
		objectId = "object1"
	)
	_, author, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)

	root := &objecttree.Change{Id: objectId, Identity: author}
	// edit is one user change setting the object's name
	edit := func(id string, ts int64, integrationName string) *objecttree.Change {
		return &objecttree.Change{Id: id, Timestamp: ts, Identity: author, Model: &pb.Change{
			Timestamp:       ts,
			IntegrationName: integrationName,
			Content: []*pb.ChangeContent{{Value: &pb.ChangeContentValueOfDetailsSet{
				DetailsSet: &pb.ChangeDetailsSet{Key: bundle.RelationKeyName.String(), Value: pbtypes.String(id)},
			}}},
		}}
	}
	build := func(t *testing.T, changes ...*objecttree.Change) *state.State {
		st, _, _, err := BuildState(spaceId, nil, viaTree{id: objectId, rootId: objectId, changes: append([]*objecttree.Change{root}, changes...)}, true)
		require.NoError(t, err)
		return st
	}
	via := func(st *state.State) (string, bool) {
		value := st.LocalDetails().Get(bundle.RelationKeyLastModifiedVia)
		return value.String(), value.Ok()
	}

	t.Run("the latest change was made through the API", func(t *testing.T) {
		// when
		got, _ := via(build(t, edit("c1", 100, ""), edit("c2", 200, "Raycast")))

		// then: red if BuildState does not derive the detail at all
		assert.Equal(t, "Raycast", got)
	})

	t.Run("an app edit after an API edit clears it", func(t *testing.T) {
		// when
		_, has := via(build(t, edit("c1", 100, "Raycast"), edit("c2", 200, "")))

		// then: red if the detail is only ever set
		assert.False(t, has)
	})

	t.Run("the app is read off the very change the author is read off", func(t *testing.T) {
		// given: two user changes by different members in the same second,
		// one made through the API. WHICH of two same-second changes counts
		// as the latest is long-standing behavior (the first met) and not
		// this feature's business; that the author and the app come from the
		// SAME one is.
		_, other, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		for _, tc := range []struct {
			name           string
			first, second  string // integration names, in tree order
			wantVia        string
			wantViaPresent bool
		}{
			{"the API change is the one picked", "Raycast", "", "Raycast", true},
			{"the app change is the one picked", "", "Raycast", "", false},
		} {
			first := edit("c1", 100, tc.first)
			second := edit("c2", 100, tc.second)
			second.Identity = other

			// when
			st := build(t, first, second)

			// then: red if the app is picked by any rule other than "the
			// change the author came from" — e.g. the last non-empty name,
			// or a different tie-break
			gotVia, has := via(st)
			assert.Equal(t, domain.NewParticipantId(spaceId, author.Account()), st.LocalDetails().GetString(bundle.RelationKeyLastModifiedBy), tc.name)
			assert.Equal(t, tc.wantViaPresent, has, tc.name)
			assert.Equal(t, tc.wantVia, gotVia, tc.name)
		}
	})

	t.Run("a non-user change does not move it", func(t *testing.T) {
		// given: a layout sync written for an API request, after an app edit
		sync := edit("c2", 200, "Raycast")
		sync.Model.(*pb.Change).ChangeType = domain.ChangeTypeLayoutSync.Raw()

		// when
		_, has := via(build(t, edit("c1", 100, ""), sync))

		// then: red if lastModifiedVia stops following lastModifiedBy
		assert.False(t, has)
	})

	t.Run("incoming changes from another device, with and without a name", func(t *testing.T) {
		// given: the open document, last edited in an app
		doc := build(t, edit("c1", 100, ""))
		_, _, err := state.ApplyState(spaceId, doc, false)
		require.NoError(t, err)
		incoming := func(t *testing.T, doc *state.State, changes ...*objecttree.Change) *state.State {
			st, _, _, err := BuildState(spaceId, doc, viaTree{id: objectId, rootId: objectId, changes: changes}, false)
			require.NoError(t, err)
			_, _, err = state.ApplyState(spaceId, st, false)
			require.NoError(t, err)
			return doc
		}

		// when: an API edit arrives; then, on the document that now holds
		// "via Raycast", an app edit
		afterApiEdit, _ := via(incoming(t, doc, edit("c1", 100, ""), edit("c2", 200, "Raycast")))
		seeded, _ := via(doc)
		require.Equal(t, "Raycast", seeded, "the clear below must start from an attributed document")
		_, hasAfterAppEdit := via(incoming(t, doc, edit("c2", 200, "Raycast"), edit("c3", 300, "")))

		// then: red if the incremental build (treeSource.Update) does not
		// maintain the detail, in either direction
		assert.Equal(t, "Raycast", afterApiEdit)
		assert.False(t, hasAfterAppEdit)
		// and the author it sits next to is that change's
		assert.Equal(t, domain.NewParticipantId(spaceId, author.Account()), doc.LocalDetails().GetString(bundle.RelationKeyLastModifiedBy))
	})

	t.Run("a snapshot cannot carry it", func(t *testing.T) {
		// given: the tree starts at a snapshot whose details claim an app
		// (a peer wrote them, or an old build did), followed by an app edit
		snapshot := &objecttree.Change{Id: "snap", Timestamp: 100, Identity: author, Model: &pb.Change{
			Timestamp: 100,
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Blocks: []*model.Block{{Id: objectId}},
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyLastModifiedVia.String(): pbtypes.String("Forged"),
				}},
			}},
		}}

		// when
		st, _, _, err := BuildState(spaceId, nil, viaTree{id: objectId, rootId: "snap", changes: []*objecttree.Change{snapshot, edit("c2", 200, "")}}, true)
		require.NoError(t, err)

		// then: red if the relation stops being derived (snapshot details
		// are cut of derived keys) or the clear is skipped. CombinedDetails:
		// the forged value must be in neither the synced nor the local half.
		_, has := via(st)
		assert.False(t, has)
		assert.False(t, st.CombinedDetails().Has(bundle.RelationKeyLastModifiedVia))
	})

	t.Run("a build with no user change sets no app next to the author it sets", func(t *testing.T) {
		// given: a layout sync wrote the snapshot the replay starts from, and
		// the snapshot claims an app in its details
		snapshot := &objecttree.Change{Id: "snap", Timestamp: 300, Identity: author, Model: &pb.Change{
			Timestamp:       300,
			ChangeType:      domain.ChangeTypeLayoutSync.Raw(),
			IntegrationName: "Raycast",
			Snapshot:        &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{Blocks: []*model.Block{{Id: objectId}}}},
		}}

		// when
		st, _, _, err := BuildState(spaceId, nil, viaTree{id: objectId, rootId: "snap", changes: []*objecttree.Change{snapshot}}, true)
		require.NoError(t, err)

		// then: the build writes an author here as it always has (from no
		// change at all); the app written with it comes from that same
		// nothing. Red if the app is taken from a change the author was not.
		_, has := via(st)
		assert.False(t, has)
	})

}

// A store payload is read as pb.Change by version history and the debug tree
// reader. The stamp shares pb.Change's field number so that read still works
// and still finds it (the rule is written in changes.proto).
func TestStoreChange_ReadsAsChange(t *testing.T) {
	t.Run("a stamped store change decodes as pb.Change with the same name", func(t *testing.T) {
		// given: a store change with real content in field 1
		builder := &storestate.Builder{}
		arena := &anyenc.Arena{}
		doc := arena.NewObject()
		doc.Set("text", arena.NewString("hello"))
		require.NoError(t, builder.Create("chats", "message1", doc))
		data, dataType, err := MarshalStoreChange(&pb.StoreChange{ChangeSet: builder.ChangeSet, IntegrationName: "Raycast"})
		require.NoError(t, err)

		// when: both decoders history and the debug reader use
		asChange, err := UnmarshalChange(&objecttree.Change{DataType: dataType}, data)
		require.NoError(t, err)
		asChangeNoSnapshot, err := unmarshalChange(&objecttree.Change{DataType: dataType}, data, false)
		require.NoError(t, err)

		// then: red if StoreChange.integrationName moves off field 10, or
		// takes a pb.Change number of another type
		assert.Equal(t, "Raycast", asChange.(*pb.Change).IntegrationName)
		assert.Equal(t, "Raycast", asChangeNoSnapshot.(*pb.Change).IntegrationName)
	})
}

// recordingHandler records the attribution each create arrives with.
type recordingHandler struct {
	storestate.DefaultHandler
	created map[string]string
}

func (h *recordingHandler) BeforeCreate(_ context.Context, ch storestate.ChangeOp) error {
	h.created[ch.Change.Id] = ch.Change.IntegrationName
	return nil
}

type storeAccountStub struct{ accountService }

func (storeAccountStub) AccountID() string { return "account1" }

func TestPushStoreChange_IntegrationName(t *testing.T) {
	apiCtx := domain.CtxWithIntegrationName(ctx, "Raycast")
	newStore := func(t *testing.T) (*store, *storeFx, *recordingHandler) {
		fx := newRealTreeStoreFx(t)
		handler := &recordingHandler{DefaultHandler: storestate.DefaultHandler{Name: "messages"}, created: map[string]string{}}
		st, err := storestate.New(ctx, "attribution_test", fx.db, handler)
		require.NoError(t, err)
		fx.state = st
		return &store{
			treeSource:   &treeSource{ObjectTree: fx.realTree, accountService: storeAccountStub{}},
			store:        st,
			onUpdateHook: func() {},
		}, fx, handler
	}
	push := func(t *testing.T, s *store, pushCtx context.Context, documentId string) string {
		builder := &storestate.Builder{}
		arena := &anyenc.Arena{}
		doc := arena.NewObject()
		doc.Set("text", arena.NewString("hello"))
		require.NoError(t, builder.Create("messages", documentId, doc))
		changeId, err := s.PushStoreChange(pushCtx, source.PushStoreChangeParams{Changes: builder.ChangeSet, State: s.store})
		require.NoError(t, err)
		return changeId
	}
	// pushed reads the payloads back from the tree, as a peer would
	pushed := func(t *testing.T, tree objecttree.ObjectTree) map[string]string {
		names := map[string]string{}
		require.NoError(t, tree.IterateRoot(UnmarshalStoreChange, func(change *objecttree.Change) bool {
			if storeChange, ok := change.Model.(*pb.StoreChange); ok {
				names[change.Id] = storeChange.IntegrationName
			}
			return true
		}))
		return names
	}

	t.Run("a push for an API request is stamped, a push for anything else is not", func(t *testing.T) {
		// given
		s, fx, handler := newStore(t)

		// when
		apiChange := push(t, s, apiCtx, "m1")
		appChange := push(t, s, ctx, "m2")

		// then: the signed payload — red if PushStoreChange stops reading the
		// name off its context, or reads it from anywhere that outlives one
		want := map[string]string{apiChange: "Raycast", appChange: ""}
		assert.Equal(t, want, pushed(t, fx.realTree))
		// and what the local apply handed the handler — red if the validator
		// callback builds its change set without the attribution
		assert.Equal(t, want, handler.created)
	})

	t.Run("a replayed change reaches the handler with the name its payload carries", func(t *testing.T) {
		// given: a tree holding one API change and one app change
		s, fx, _ := newStore(t)
		apiChange := push(t, s, apiCtx, "m1")
		appChange := push(t, s, ctx, "m2")

		// when: another device (a fresh store) replays the tree
		db, err := anystore.Open(ctx, t.TempDir()+"/replay.db", nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		replayHandler := &recordingHandler{DefaultHandler: storestate.DefaultHandler{Name: "messages"}, created: map[string]string{}}
		replayState, err := storestate.New(ctx, "attribution_replay", db, replayHandler)
		require.NoError(t, err)
		tx, err := replayState.NewTx(ctx)
		require.NoError(t, err)
		require.NoError(t, (&storeApply{tx: tx, ot: fx.realTree}).Apply(ctx))
		require.NoError(t, tx.Commit())

		// then: red if storeApply.applyChange drops the payload's attribution
		want := map[string]string{apiChange: "Raycast", appChange: ""}
		assert.Equal(t, want, replayHandler.created)
	})
}

// cancelAfterAdd is an object tree that cancels a context right after the
// change has been added: the moment between the store apply and its commit.
type cancelAfterAdd struct {
	objecttree.ObjectTree
	cancel context.CancelFunc
}

func (c cancelAfterAdd) AddContentWithValidator(ctx context.Context, content objecttree.SignableChangeContent, validate func(change objecttree.StorageChange) error) (objecttree.AddResult, error) {
	res, err := c.ObjectTree.AddContentWithValidator(ctx, content, validate)
	c.cancel()
	return res, err
}

func TestPushStoreChange_ReportsAFailedCommit(t *testing.T) {
	// given: a store whose transaction cannot commit — its context ends
	// after the change is in the tree
	fx := newRealTreeStoreFx(t)
	pushCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	updated := false
	s := &store{
		treeSource:   &treeSource{ObjectTree: cancelAfterAdd{ObjectTree: fx.realTree, cancel: cancel}, accountService: storeAccountStub{}},
		store:        fx.state,
		onUpdateHook: func() { updated = true },
	}
	builder := &storestate.Builder{}
	arena := &anyenc.Arena{}
	doc := arena.NewObject()
	doc.Set("text", arena.NewString("hello"))
	require.NoError(t, builder.Create("default", "doc1", doc))

	// when
	changeId, err := s.PushStoreChange(pushCtx, source.PushStoreChangeParams{Changes: builder.ChangeSet, State: s.store})

	// then: red if the commit error is overwritten by the lookup that
	// follows it — the caller would be told the push succeeded while the
	// store holds nothing of it
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit store tx")
	assert.Empty(t, changeId)
	assert.False(t, updated, "a failed commit must not announce an update")
}
