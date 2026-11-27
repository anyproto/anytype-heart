package smartblock

// Creation-provenance stamping tests (APIV2_OBJECT_DELETE.md §11.4/§15).
// The assertions run at the PushChangeParams level — what the source is
// actually asked to write — via the recording sourceStub, so a green Apply
// alone cannot pass them.

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree/mock_objecttree"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// initAs mirrors the creation-path Init exactly as ObjectFactory.InitObject
// performs it (factory.go): the InitContext carries the request ctx and the
// IsNewObject flag, Init prepares initCtx.State, and the caller then applies
// that state. Keeping the ctx and the flag as parameters is what lets the
// tests below distinguish the creation path from an existing-object open —
// the distinction the stamp gates on.
func (fx *fixture) initAs(t *testing.T, ctx context.Context, isNewObject bool) *InitContext {
	// the IsNewObject init branch runs the sub-object links migration, which
	// consults the space kind
	fx.space.EXPECT().IsPersonal().Return(false).Maybe()
	doc := state.NewDoc(fx.source.id, map[string]simple.Block{
		fx.source.id: simple.New(&model.Block{Id: fx.source.id}),
	})
	fx.source.doc = doc
	initCtx := &InitContext{
		Ctx:         ctx,
		IsNewObject: isNewObject,
		SpaceID:     "space1",
		Source:      fx.source,
		Doc:         doc,
	}
	require.NoError(t, fx.Init(initCtx))
	return initCtx
}

// applyWithNewBlock adds one block to s and applies it the way the creation
// path does (no events, no history — factory.go's flag set minus the ones
// the fixture cannot satisfy), so every apply in these tests produces a
// pushable change set.
func (fx *fixture) applyWithNewBlock(t *testing.T, s *state.State, blockId string) {
	s.Add(simple.New(&model.Block{Id: blockId}))
	require.NoError(t, s.InsertTo(fx.source.id, model.Block_Inner, blockId))
	require.NoError(t, fx.Apply(s, NoHistory, NoEvent, NoRestrictions, KeepInternalFlags))
}

// lastModifiedOf reads the last-modified tuple a state holds.
func lastModifiedOf(s *state.State) state.LastModified {
	local := s.LocalDetails()
	return state.LastModified{
		Timestamp:       local.GetInt64(bundle.RelationKeyLastModifiedDate),
		By:              local.GetString(bundle.RelationKeyLastModifiedBy),
		IntegrationName: local.GetString(bundle.RelationKeyLastModifiedVia),
	}
}

func TestSmartBlock_IntegrationNameStamping(t *testing.T) {
	ctxWithName := domain.CtxWithIntegrationName(context.Background(), "Claude Desktop")

	t.Run("creating apply stamps, the next apply does not — the per-apply leak guard", func(t *testing.T) {
		// given: a NEW object whose init ctx carries the API session raw app name —
		// the §8a creation shape shared by v1 and v2
		fx := newFixture("root", t)
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil)
		initCtx := fx.initAs(t, ctxWithName, true)

		// when: the creating Apply (what factory.InitObject runs), then a
		// later local edit on the very same open object
		initCtx.State.SetChangeType(domain.ChangeTypeObjectInit)
		fx.applyWithNewBlock(t, initCtx.State, "created-block")
		fx.applyWithNewBlock(t, fx.NewState(), "edited-later-block")

		// then: exactly the creating change carries the name. The first
		// assertion fails if the Init fill or the Apply plumbing is reverted;
		// the SECOND fails if the value is ever persisted on the doc/smartblock
		// instead of the per-apply state — the leak that would misattribute
		// every subsequent local edit and silently widen the DELETE gate.
		require.Len(t, fx.source.pushed, 2)
		assert.Equal(t, "Claude Desktop", fx.source.pushed[0].IntegrationName)
		assert.Equal(t, "", fx.source.pushed[1].IntegrationName)

		// F6: the creating Apply CONSUMES the stamp. initCtx.State became the
		// doc after the apply — if Apply only read the field without clearing
		// it, the doc would keep carrying the name and re-applying the same
		// state object would push it twice (a caller-count invariant, not a
		// code one; executed by the review). A fixture that never looks at
		// the post-apply state cannot catch this — this assertion fails if
		// the clear-at-capture line is dropped.
		assert.Equal(t, "", initCtx.State.IntegrationName(),
			"the stamp must be consumed by the apply that pushed it")
	})

	t.Run("existing object opened under an API ctx is not stamped", func(t *testing.T) {
		// given: the SAME ctx, but IsNewObject false — an existing object
		// loaded during an API request (a PATCH, a migration-producing open).
		// This fixture can fail: an implementation that stamps from the ctx on
		// every apply — instead of gating on creation — pushes the name here.
		fx := newFixture("root", t)
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil)
		fx.initAs(t, ctxWithName, false)

		// when
		fx.applyWithNewBlock(t, fx.NewState(), "block")

		// then
		require.Len(t, fx.source.pushed, 1)
		assert.Equal(t, "", fx.source.pushed[0].IntegrationName)
	})

	t.Run("creation without an API ctx is not stamped", func(t *testing.T) {
		// given: the UI/import/indexer shape — a new object, no carrier on the
		// ctx. Fails if the stamp ever stops being derived from the carrier
		// (e.g. a hardcoded or session-global value).
		fx := newFixture("root", t)
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil)
		initCtx := fx.initAs(t, context.Background(), true)

		// when
		initCtx.State.SetChangeType(domain.ChangeTypeObjectInit)
		fx.applyWithNewBlock(t, initCtx.State, "block")

		// then
		require.Len(t, fx.source.pushed, 1)
		assert.Equal(t, "", fx.source.pushed[0].IntegrationName)
	})
}

// Edit attribution (docs/ApiChangeAttribution.md): an edit made for a
// local-API request pushes a stamped change and shows the app in
// lastModifiedVia; the next edit made in an app pushes an unstamped one and
// clears it. Asserted on what the source is asked to write.
func TestSmartBlock_EditAttribution(t *testing.T) {
	apiSession := session.NewContext(session.WithIntegrationName("Raycast"))
	apiCtx := domain.CtxWithIntegrationName(context.Background(), "Raycast")

	open := func(t *testing.T) *fixture {
		fx := newFixture("root", t)
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil)
		fx.initAs(t, context.Background(), false)
		return fx
	}
	via := func(fx *fixture) string {
		return fx.LocalDetails().GetString(bundle.RelationKeyLastModifiedVia)
	}

	for _, tc := range []struct {
		name     string
		newState func(fx *fixture) *state.State
	}{
		// red if Apply stops asking the state's session context
		{"session context of the request", func(fx *fixture) *state.State { return fx.NewStateCtx(apiSession) }},
		// red if StampFromCtx or the explicit stamp stops reaching the push
		{"explicit stamp from the request context", func(fx *fixture) *state.State { return fx.NewState().StampFromCtx(apiCtx) }},
		// red if NewStateCtx stops turning the attribution-only context into a stamp
		{"attribution-only context", func(fx *fixture) *state.State { return fx.NewStateCtx(session.AttributionOnly(apiCtx)) }},
	} {
		t.Run("an API edit is stamped, the next app edit is not: "+tc.name, func(t *testing.T) {
			// given
			fx := open(t)

			// when: an API edit, then a desktop-style edit of the same open
			// object in the same process — NewStateCtx with a session that
			// carries no app, which is what a gRPC call gets
			fx.applyWithNewBlock(t, tc.newState(fx), "api-block")
			viaAfterApiEdit := via(fx)
			fx.applyWithNewBlock(t, fx.NewStateCtx(session.NewContext(session.WithSession("token"))), "app-block")

			// then
			require.Len(t, fx.source.pushed, 2)
			assert.Equal(t, "Raycast", fx.source.pushed[0].IntegrationName)
			// red if the name is kept anywhere that outlives the apply
			assert.Equal(t, "", fx.source.pushed[1].IntegrationName)
			assert.Equal(t, "Raycast", viaAfterApiEdit)
			// red if lastModifiedVia is only ever set, never removed
			assert.False(t, fx.LocalDetails().Has(bundle.RelationKeyLastModifiedVia))
		})
	}

	t.Run("lastModifiedVia never enters the pushed change or the synced details", func(t *testing.T) {
		// given
		fx := open(t)

		// when
		fx.applyWithNewBlock(t, fx.NewStateCtx(apiSession), "api-block")

		// then: red if the detail is ever written as a synced detail — it
		// would cost a change (or ride the next one) on every edit
		require.Len(t, fx.source.pushed, 1)
		for _, content := range fx.source.pushed[0].Changes {
			if set := content.GetDetailsSet(); set != nil {
				assert.NotEqual(t, bundle.RelationKeyLastModifiedVia.String(), set.Key)
			}
		}
		assert.False(t, fx.Details().Has(bundle.RelationKeyLastModifiedVia))
	})

	t.Run("a non-user change is stamped but does not move lastModifiedVia", func(t *testing.T) {
		// given: an object whose last user change was made in an app
		fx := open(t)
		s := fx.NewStateCtx(apiSession)
		s.SetChangeType(domain.ChangeTypeLayoutSync)

		// when
		fx.applyWithNewBlock(t, s, "sync-block")

		// then: the change says who wrote it; lastModifiedVia follows
		// lastModifiedBy, which only user changes move. Red if the stamp is
		// gated on the change type, or lastModifiedVia is not.
		require.Len(t, fx.source.pushed, 1)
		assert.Equal(t, "Raycast", fx.source.pushed[0].IntegrationName)
		assert.False(t, fx.LocalDetails().Has(bundle.RelationKeyLastModifiedVia))
	})

	t.Run("a hook learns the apply's attribution", func(t *testing.T) {
		// given
		fx := open(t)
		var got []string
		fx.AddHook(func(info ApplyInfo) error {
			got = append(got, info.IntegrationName)
			return nil
		}, HookAfterApply)

		// when
		fx.applyWithNewBlock(t, fx.NewStateCtx(apiSession), "api-block")
		fx.applyWithNewBlock(t, fx.NewState(), "app-block")

		// then: red if ApplyInfo.IntegrationName is not filled — the type →
		// objects layout sync would push unattributed changes for an API edit
		want := []string{"Raycast", ""}
		assert.Equal(t, want, got)
	})

	// create runs a new object's Init and creating apply on a creation
	// state prepared by write, and returns the created object.
	create := func(t *testing.T, write func(creation *state.State)) *fixture {
		fx := newFixture("root", t)
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
		fx.space.EXPECT().IsPersonal().Return(false).Maybe()
		doc := state.NewDoc(fx.source.id, map[string]simple.Block{
			fx.source.id: simple.New(&model.Block{Id: fx.source.id}),
		})
		fx.source.doc = doc
		creation := doc.NewState()
		write(creation)
		initCtx := &InitContext{
			Ctx:         domain.CtxWithIntegrationName(context.Background(), "Raycast"),
			IsNewObject: true,
			SpaceID:     "space1",
			Source:      fx.source,
			Doc:         doc,
			State:       creation,
		}
		require.NoError(t, fx.Init(initCtx))
		initCtx.State.SetChangeType(domain.ChangeTypeObjectInit)
		fx.applyWithNewBlock(t, initCtx.State, "created-block")
		return fx
	}

	t.Run("a creation state built from copied details does not keep their app", func(t *testing.T) {
		// given: the details of an existing object, last edited by Alice
		// through Linear, copied whole into a new object's creation state
		// (a duplicate; a create request naming both keys is the same shape)
		copied := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:            domain.String("Copy"),
			bundle.RelationKeyLastModifiedBy:  domain.String("alice"),
			bundle.RelationKeyLastModifiedVia: domain.String("Linear"),
		})

		// the details are written in the order their map is visited in,
		// which is random: repeat until both orders have certainly been met
		for i := 0; i < 32; i++ {
			// when
			fx := create(t, func(creation *state.State) { creation.SetDetails(copied.Copy()) })

			// then: red if the bulk write lets the copied app in after the
			// copied author — the new object would show another object's app
			require.False(t, fx.LocalDetails().Has(bundle.RelationKeyLastModifiedVia), "iteration %d", i)
			require.Len(t, fx.source.pushed, 1)
			assert.Equal(t, "Raycast", fx.source.pushed[0].IntegrationName)
		}
	})

	t.Run("a creation state cannot plant lastModifiedVia through a whole local-details write", func(t *testing.T) {
		// given: the one writer the generic setters do not guard — a state's
		// local details replaced as a whole — with an app name, and with a
		// value that is not a name at all
		for name, planted := range map[string]domain.Value{
			"a name":           domain.String("Planted"),
			"not a name":       domain.Bool(true),
			"an empty value":   domain.String(""),
			"an invalid value": domain.Invalid(),
		} {
			// when: Init, then the creating apply
			fx := create(t, func(creation *state.State) {
				creation.SetLocalDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyLastModifiedVia: planted,
				}))
			})

			// then: red if Apply keeps a value no derivation put there — and,
			// for the value that is no name, red if the check compares the
			// key's text only: a boolean reads as "", like the document's
			// absent key, and would be let through; for the invalid value,
			// red if presence is read off the value instead of the map. Creation does not set it
			// either: it follows lastModifiedBy, which only user changes move
			// — the creating change itself carries the app.
			assert.False(t, fx.LocalDetails().Has(bundle.RelationKeyLastModifiedVia), name)
			require.Len(t, fx.source.pushed, 1, name)
			assert.Equal(t, "Raycast", fx.source.pushed[0].IntegrationName, name)
		}
	})

	// initWith opens an object whose state, as built from the tree, holds
	// treeTuple (nil: the replayed part of the tree has no user change),
	// over an index row holding indexTuple.
	initWith := func(t *testing.T, treeTuple *state.LastModified, indexTuple state.LastModified) *fixture {
		fx := newFixture("root", t)
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
		fx.space.EXPECT().IsPersonal().Return(false).Maybe()
		row := []objectstore.TestObject{{
			bundle.RelationKeyId:               domain.String("root"),
			bundle.RelationKeyLastModifiedDate: domain.Int64(indexTuple.Timestamp),
			bundle.RelationKeyLastModifiedBy:   domain.String(indexTuple.By),
			bundle.RelationKeyLastOpenedDate:   domain.Int64(42),
		}}
		if indexTuple.IntegrationName != "" {
			// a row whose latest change was made in an app has NO such key
			// — not an empty one
			row[0][bundle.RelationKeyLastModifiedVia] = domain.String(indexTuple.IntegrationName)
		}
		fx.objectStore.AddObjects(t, testSpaceId, row)
		doc := state.NewDoc("root", map[string]simple.Block{"root": simple.New(&model.Block{Id: "root"})}).(*state.State)
		// an existing object: its document is past the header change
		doc.SetChangeId("some-change")
		if treeTuple != nil {
			doc.SetLastModified(*treeTuple)
		}
		fx.source.doc = doc
		require.NoError(t, fx.Init(&InitContext{Ctx: context.Background(), SpaceID: "space1", Source: fx.source, Doc: doc}))
		return fx
	}
	tupleOf := func(fx *fixture) state.LastModified {
		return lastModifiedOf(fx.Doc.(*state.State))
	}

	t.Run("reopen: the app comes from the row the author comes from", func(t *testing.T) {
		// On open the index row's author and date override what the tree
		// build produced — long-standing behavior. The app follows the
		// author: from the same row, present or absent.
		for _, tc := range []struct {
			name  string
			tree  state.LastModified
			index state.LastModified
			want  state.LastModified
		}{
			{
				// red if the app is NOT restored with the author
				name:  "the row has an app, the tree's change has none",
				tree:  state.LastModified{Timestamp: 200, By: "bob"},
				index: state.LastModified{Timestamp: 100, By: "alice", IntegrationName: "Raycast"},
				want:  state.LastModified{Timestamp: 100, By: "alice", IntegrationName: "Raycast"},
			},
			{
				// red if injection only ever ADDS the key: the tree's app
				// would stay next to the index's author — "Alice via Linear"
				// when Bob made the Linear edit
				name:  "the row has no app, the tree's change has one",
				tree:  state.LastModified{Timestamp: 200, By: "bob", IntegrationName: "Linear"},
				index: state.LastModified{Timestamp: 100, By: "alice"},
				want:  state.LastModified{Timestamp: 100, By: "alice"},
			},
		} {
			tree := tc.tree
			fx := initWith(t, &tree, tc.index)
			assert.Equal(t, tc.want, tupleOf(fx), tc.name)
			// absent means absent: no key at all, not an empty value
			assert.Equal(t, tc.want.IntegrationName != "", fx.LocalDetails().Has(bundle.RelationKeyLastModifiedVia), tc.name)
		}
	})

	t.Run("reopen: no index row, the tree's pair stands", func(t *testing.T) {
		// given: an object this device never indexed
		fx := newFixture("root", t)
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
		doc := state.NewDoc("root", map[string]simple.Block{"root": simple.New(&model.Block{Id: "root"})}).(*state.State)
		doc.SetChangeId("some-change")
		doc.SetLastModified(state.LastModified{Timestamp: 200, By: "bob", IntegrationName: "Linear"})
		fx.source.doc = doc

		// when
		require.NoError(t, fx.Init(&InitContext{Ctx: context.Background(), SpaceID: "space1", Source: fx.source, Doc: doc}))

		// then: red if a row without an author clears the app
		want := state.LastModified{Timestamp: 200, By: "bob", IntegrationName: "Linear"}
		assert.Equal(t, want, tupleOf(fx))
	})

	t.Run("reopen with a pending local update keeps the restored app", func(t *testing.T) {
		// given: a closed page made a favorite — the flag waits in the
		// pending local details, which keeps the index row out of the
		// DOCUMENT on open (only the edit state gets it)
		fx := newFixture("root", t)
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
		fx.eventSender.EXPECT().SendToSession(mock.Anything, mock.Anything).Maybe()
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:               domain.String("root"),
			bundle.RelationKeyLastModifiedDate: domain.Int64(100),
			bundle.RelationKeyLastModifiedBy:   domain.String("alice"),
			bundle.RelationKeyLastModifiedVia:  domain.String("Raycast"),
		}})
		require.NoError(t, fx.store.UpdatePendingLocalDetails("root", func(pending *domain.Details) (*domain.Details, error) {
			pending.SetBool(bundle.RelationKeyIsFavorite, true)
			return pending, nil
		}))
		doc := state.NewDoc("root", map[string]simple.Block{"root": simple.New(&model.Block{Id: "root"})}).(*state.State)
		doc.SetChangeId("some-change")
		// what the tree build produced: Bob's edit through Linear
		doc.SetLastModified(state.LastModified{Timestamp: 200, By: "bob", IntegrationName: "Linear"})
		fx.source.doc = doc
		initCtx := &InitContext{Ctx: context.Background(), SpaceID: "space1", Source: fx.source, Doc: doc}
		require.NoError(t, fx.Init(initCtx))

		// then, BEFORE any apply — this is what stays if the init apply
		// never merges the edit state (a before-apply hook failing): each
		// state holds a whole pair. Red if the app is restored into the
		// document while the author is restored into the edit state only
		// (the document would say "Bob via Raycast").
		inDocument := lastModifiedOf(doc)
		inEditState := lastModifiedOf(initCtx.State)
		assert.Equal(t, state.LastModified{Timestamp: 200, By: "bob", IntegrationName: "Linear"}, inDocument)
		assert.Equal(t, state.LastModified{Timestamp: 100, By: "alice", IntegrationName: "Raycast"}, inEditState)

		// when: the init apply, as the factory runs it
		initCtx.State.SetChangeType(domain.ChangeTypeObjectInit)
		require.NoError(t, fx.Apply(initCtx.State, InitApplyFlags(false)...))

		// then: red if Apply treats the restored app as written by content
		// and resets it to the document's — the page would be indexed with
		// the row's author and the tree's app
		want := state.LastModified{Timestamp: 100, By: "alice", IntegrationName: "Raycast"}
		assert.Equal(t, want, tupleOf(fx))
		assert.True(t, fx.LocalDetails().GetBool(bundle.RelationKeyIsFavorite), "the pending update itself must have been applied")
	})

	t.Run("a store-backed object drops an indexed app on open", func(t *testing.T) {
		// given: a chat whose index row holds an app (an earlier build of
		// this branch wrote it)
		fx := newFixture("root", t)
		fx.source.sbType = coresb.SmartBlockTypeChatDerivedObject
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:               domain.String("root"),
			bundle.RelationKeyLastModifiedDate: domain.Int64(100),
			bundle.RelationKeyLastModifiedBy:   domain.String("alice"),
			bundle.RelationKeyLastModifiedVia:  domain.String("Raycast"),
		}})
		doc := state.NewDoc("root", map[string]simple.Block{"root": simple.New(&model.Block{Id: "root"})}).(*state.State)
		fx.source.doc = doc

		// when
		require.NoError(t, fx.Init(&InitContext{Ctx: context.Background(), SpaceID: "space1", Source: fx.source, Doc: doc}))

		// then: red if index hydration restores the app for store-backed
		// objects — nothing their replay derives could confirm or clear it
		assert.False(t, fx.LocalDetails().Has(bundle.RelationKeyLastModifiedVia))
		assert.Equal(t, "alice", fx.LocalDetails().GetString(bundle.RelationKeyLastModifiedBy))
	})

	t.Run("a user change sets the author, the date and the app in one place, before the push", func(t *testing.T) {
		// given: last edited by Alice in an app; the push will fail
		alice := state.LastModified{Timestamp: 100, By: "alice"}
		fx := initWith(t, &alice, alice)
		fx.currentParticipantId = "me"
		fx.source.pushErr = fmt.Errorf("storage is full")
		s := fx.NewStateCtx(apiSession)
		s.Add(simple.New(&model.Block{Id: "api-block"}))
		require.NoError(t, s.InsertTo("root", model.Block_Inner, "api-block"))

		// when
		err := fx.Apply(s, NoHistory, NoEvent, NoRestrictions, KeepInternalFlags)

		// then: the whole tuple is on the pushed state when the push is
		// called — the author and date where they have always been set,
		// right before the push, and the app with them, from the same
		// change. Red if any member is set after the push (or not at all:
		// the date would still be 100), or the app comes from elsewhere.
		require.Error(t, err)
		require.Len(t, fx.source.attempts, 1)
		require.Len(t, fx.source.tuplesAtPush, 1)
		want := state.LastModified{
			Timestamp:       fx.source.attempts[0].Time.Unix(),
			By:              "me",
			IntegrationName: "Raycast",
		}
		require.NotEqual(t, alice.Timestamp, want.Timestamp)
		assert.Equal(t, want, fx.source.tuplesAtPush[0])
		// and, as on develop, a failed push leaves it in the document
		assert.Equal(t, want, tupleOf(fx))
	})

	t.Run("a bare write of the author does not keep the previous author's app", func(t *testing.T) {
		// lastModifiedBy has always been writable (a details RPC, a block
		// bound to it). Whoever writes it, the app next to it belonged to
		// the previous author's change.
		attributed := state.LastModified{Timestamp: 100, By: "alice", IntegrationName: "Raycast"}
		for name, write := range map[string]func(s *state.State){
			"set":    func(s *state.State) { s.SetDetail(bundle.RelationKeyLastModifiedBy, domain.String("bob")) },
			"remove": func(s *state.State) { s.RemoveLocalDetail(bundle.RelationKeyLastModifiedBy) },
		} {
			fx := initWith(t, &attributed, attributed)

			// when: a property-only edit — nothing is pushed, so nothing
			// re-derives the pair afterwards
			s := fx.NewState()
			write(s)
			require.NoError(t, fx.Apply(s, NoHistory, NoEvent, NoRestrictions, KeepInternalFlags), name)

			// then: red if the app survives — "Bob via Raycast" for an edit
			// Alice made through Raycast
			assert.Empty(t, fx.source.pushed, name)
			assert.False(t, fx.LocalDetails().Has(bundle.RelationKeyLastModifiedVia), name)
		}
	})

	t.Run("a block bound to lastModifiedVia shows nothing, and the app reaches no stored content", func(t *testing.T) {
		// given: last edited through an app whose name is punctuation only
		// (an anonymized debug export scrambles letters, not punctuation);
		// a desktop edit adds a paragraph and a block bound to the detail
		attributed := state.LastModified{Timestamp: 100, By: "alice", IntegrationName: "!!!"}
		fx := initWith(t, &attributed, attributed)
		s := fx.NewState()
		s.Add(simple.New(&model.Block{Id: "paragraph", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "hello"}}}))
		s.Add(simple.New(&model.Block{
			Id:      "bound",
			Fields:  &types.Struct{Fields: map[string]*types.Value{"_detailsKey": pbtypes.StringList([]string{bundle.RelationKeyLastModifiedVia.String()})}},
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{}},
		}))
		require.NoError(t, s.InsertTo("root", model.Block_Inner, "paragraph", "bound"))

		// when
		require.NoError(t, fx.Apply(s, NoHistory, NoEvent, NoRestrictions, KeepInternalFlags))

		// then: develop's binding, as for every local or derived property:
		// the block is initialized from the synced details alone, so it
		// shows nothing. Red if the binding is filled with the app: the
		// block-create change would store the name as block text, and the
		// snippet would index it.
		require.Len(t, fx.source.pushed, 1)
		created := map[string]string{}
		for _, content := range fx.source.pushed[0].Changes {
			if create := content.GetBlockCreate(); create != nil {
				for _, b := range create.Blocks {
					created[b.Id] = b.GetText().GetText()
				}
			}
		}
		assert.Equal(t, map[string]string{"paragraph": "hello", "bound": ""}, created)
		assert.Equal(t, "hello", fx.LocalDetails().GetString(bundle.RelationKeySnippet))
		assert.Equal(t, "", fx.Pick("bound").Model().GetText().GetText())
	})

	t.Run("a block bound to lastModifiedVia cannot write it", func(t *testing.T) {
		// given: last edited by Alice through Raycast, and a text block bound
		// to the detail, as a client can create one
		attributed := state.LastModified{Timestamp: 100, By: "alice", IntegrationName: "Raycast"}
		fx := initWith(t, &attributed, attributed)
		bound := func(text string) simple.Block {
			return simple.New(&model.Block{
				Id:      "bound",
				Fields:  &types.Struct{Fields: map[string]*types.Value{"_detailsKey": pbtypes.StringList([]string{bundle.RelationKeyLastModifiedVia.String()})}},
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: text}},
			})
		}
		s := fx.NewState()
		s.Add(bound("Raycast"))
		require.NoError(t, s.InsertTo("root", model.Block_Inner, "bound"))
		require.NoError(t, fx.Apply(s, NoHistory, NoEvent, NoRestrictions, KeepInternalFlags))

		// when: the block's text is edited
		s = fx.NewState()
		s.Get("bound").Model().GetText().Text = "Forged"
		require.NoError(t, fx.Apply(s, NoHistory, NoEvent, NoRestrictions, KeepInternalFlags))

		// then: red if the block-to-detail projection writes the key — it
		// runs inside the state apply, after Apply's own reset. (The edit is
		// a user change, so the app is now none: it must not be "Forged".)
		assert.NotEqual(t, "Forged", fx.LocalDetails().GetString(bundle.RelationKeyLastModifiedVia))
	})

	t.Run("a details write cannot set or clear the app", func(t *testing.T) {
		// given: last edited by Alice through Raycast
		attributed := state.LastModified{Timestamp: 100, By: "alice", IntegrationName: "Raycast"}
		for name, write := range map[string]func(s *state.State){
			"set":   func(s *state.State) { s.SetDetail(bundle.RelationKeyLastModifiedVia, domain.String("Forged")) },
			"unset": func(s *state.State) { s.RemoveDetail(bundle.RelationKeyLastModifiedVia) },
			// the generic setters skip the key; a whole local-details write
			// does not, and only Apply's check stands in its way
			"whole local details": func(s *state.State) {
				local := s.LocalDetails().Copy()
				local.Set(bundle.RelationKeyLastModifiedVia, domain.String("Forged"))
				s.SetLocalDetails(local)
			},
		} {
			fx := initWith(t, &attributed, attributed)

			// when: a property-only edit that also writes the key — no
			// change is pushed, so nothing re-derives the tuple afterwards
			s := fx.NewState()
			write(s)
			require.NoError(t, fx.Apply(s, NoHistory, NoEvent, NoRestrictions, KeepInternalFlags), name)

			// then: red if Apply merges the written value — the object would
			// show a name its signed history contradicts
			assert.Empty(t, fx.source.pushed, name)
			assert.Equal(t, attributed, tupleOf(fx), name)
		}
	})

	t.Run("a store-backed object never shows the app, its change is stamped all the same", func(t *testing.T) {
		// given: a chat — its details live in a store whose replay does not
		// derive the tuple
		fx := newFixture("root", t)
		fx.source.sbType = coresb.SmartBlockTypeChatDerivedObject
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
		fx.initAs(t, context.Background(), false)

		// when: an API rename-like edit
		fx.applyWithNewBlock(t, fx.NewStateCtx(apiSession), "api-block")

		// then: red if the app is surfaced on store-backed objects — it
		// would be there until the chat is reopened and on no other device
		require.Len(t, fx.source.pushed, 1)
		assert.Equal(t, "Raycast", fx.source.pushed[0].IntegrationName)
		assert.False(t, fx.LocalDetails().Has(bundle.RelationKeyLastModifiedVia))
	})
}

// The DELETE ownership rule reads the FIRST non-root change of a tree
// (core/api/objectprovenanceadapter.go). With edits stamped too, that rule
// keeps its meaning only if an edit can never BE that change. This is the
// structural half of the proof (docs/ApiChangeAttribution.md "DELETE
// ownership is unchanged"): a child of the root can be pushed only by an
// apply on a root-only tree, and only the creating apply is allowed one.
func TestSmartBlock_AnEditIsNeverATreesFirstChange(t *testing.T) {
	apiSession := session.NewContext(session.WithIntegrationName("Raycast"))
	rootOnlyTree := func(t *testing.T) *mock_objecttree.MockObjectTree {
		tree := mock_objecttree.NewMockObjectTree(gomock.NewController(t))
		tree.EXPECT().Id().Return("root").AnyTimes()
		tree.EXPECT().Heads().Return([]string{"root"}).AnyTimes()
		return tree
	}

	t.Run("an API edit of a root-only tree is refused and pushes nothing", func(t *testing.T) {
		// given: an object whose tree holds its root and no content change
		// yet — created elsewhere and only partly synced, or left so by a
		// crash between the tree's creation and its creating apply
		fx := newFixture("root", t)
		fx.initAs(t, context.Background(), false)
		fx.ObjectTree = rootOnlyTree(t)
		s := fx.NewStateCtx(apiSession)
		s.Add(simple.New(&model.Block{Id: "api-block"}))
		require.NoError(t, s.InsertTo("root", model.Block_Inner, "api-block"))

		// when
		err := fx.Apply(s, NoHistory, NoEvent, NoRestrictions, KeepInternalFlags)

		// then: red if the empty-tree guard is removed or stops covering
		// edits — an app could then write the first content change of an
		// object it did not create and pass the permanent-delete check
		require.ErrorIs(t, err, ErrApplyOnEmptyTreeDisallowed)
		assert.Empty(t, fx.source.pushed)
	})

	t.Run("only the creating apply may write a root-only tree, with the creator's name", func(t *testing.T) {
		// given: the creation shape of ObjectFactory.InitObject
		fx := newFixture("root", t)
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil)
		initCtx := fx.initAs(t, domain.CtxWithIntegrationName(context.Background(), "Claude Desktop"), true)
		fx.ObjectTree = rootOnlyTree(t)
		initCtx.State.SetChangeType(domain.ChangeTypeObjectInit)
		initCtx.State.Add(simple.New(&model.Block{Id: "created-block"}))
		require.NoError(t, initCtx.State.InsertTo("root", model.Block_Inner, "created-block"))

		// when
		err := fx.Apply(initCtx.State, NoHistory, NoEvent, NoRestrictions, KeepInternalFlags, AllowApplyWithEmptyTree)

		// then
		require.NoError(t, err)
		require.Len(t, fx.source.pushed, 1)
		assert.Equal(t, "Claude Desktop", fx.source.pushed[0].IntegrationName)
	})

	// The two subtests above pass the flag by hand. These drive the init
	// apply with the flag set ObjectFactory.InitObject itself uses
	// (InitApplyFlags is that set; the factory adds nothing to it), on the
	// same two trees, and look at the actual init push.
	initPush := func(t *testing.T, isNewObject bool) (*fixture, error) {
		fx := newFixture("root", t)
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
		initCtx := fx.initAs(t, domain.CtxWithIntegrationName(context.Background(), "Raycast"), isNewObject)
		fx.ObjectTree = rootOnlyTree(t)
		// what a migration or a creation state contributes
		initCtx.State.Add(simple.New(&model.Block{Id: "init-block"}))
		require.NoError(t, initCtx.State.InsertTo("root", model.Block_Inner, "init-block"))
		initCtx.State.SetChangeType(domain.ChangeTypeObjectInit)
		return fx, fx.Apply(initCtx.State, InitApplyFlags(isNewObject)...)
	}

	t.Run("the init of an EXISTING root-only tree, loaded by an API request, pushes nothing", func(t *testing.T) {
		// when: an object whose tree holds only its root is loaded for the
		// first time by a request that carries an app name
		fx, err := initPush(t, false)

		// then: red if the existing-object flag set ever gains
		// AllowApplyWithEmptyTree — a load-time migration, run under an API
		// request's context, would become the tree's first change
		require.ErrorIs(t, err, ErrApplyOnEmptyTreeDisallowed)
		assert.Empty(t, fx.source.pushed)
	})

	t.Run("the init of a NEW object pushes the tree's first change, named after its creator", func(t *testing.T) {
		// when
		fx, err := initPush(t, true)

		// then: red if the new-object flag set loses the flag (nothing could
		// be created), or the creating change is pushed unstamped
		require.NoError(t, err)
		require.Len(t, fx.source.pushed, 1)
		assert.Equal(t, "Raycast", fx.source.pushed[0].IntegrationName)
		assert.Equal(t, domain.ChangeTypeObjectInit, fx.source.pushed[0].ChangeType)
	})
}

// Restoring a version compares the version's state with the live document.
// lastModifiedVia is a required internal relation that did not exist when a
// page last changed before the upgrade: the live document has its relation
// link (every load adds the required links), a version built from a tree
// that stored no link for it does not. This establishes that this
// difference pushes nothing (the removal of a local or derived link is
// filtered out of the change); a restore whose content differs pushes, as on
// develop.
func TestSmartBlock_ResetToAPreUpgradeVersion(t *testing.T) {
	// given: a page as its tree stores it — every required internal
	// relation link of its day that is not local or derived, plus the tuple
	// the build derives (BuildState sets it with its links): last edited by
	// Alice in an app
	alice := state.LastModified{Timestamp: 100, By: "alice"}
	stored := func() *state.State {
		st := titledDoc()
		for _, key := range bundle.RequiredInternalRelations {
			if !slices.Contains(bundle.LocalAndDerivedRelationKeys, key) {
				st.AddBundledRelationLinks(key)
			}
		}
		st.AddBundledRelationLinks(bundle.RelationKeyName)
		st.SetLastModified(alice)
		return st
	}
	fx := newFixture(lifecycleId, t)
	withTaskTypeLayout(t, fx, model.ObjectType_basic)
	fx.space.EXPECT().IsPersonal().Return(false).Maybe()
	fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
	fx.currentParticipantId = "me"
	// loaded today: the source adds every required link on read, Init too
	loaded := stored()
	loaded.AddBundledRelationLinks(bundle.RequiredInternalRelations...)
	require.True(t, loaded.PickRelationLinks().Has(bundle.RelationKeyLastModifiedVia.String()))
	initCtx := initLoaded(t, fx, loaded)
	require.NoError(t, fx.Apply(initCtx.State, InitApplyFlags(false)...))
	pushedBefore := len(fx.source.attempts)

	// when: the current version is restored, as history builds it
	historical := stored()
	require.False(t, historical.PickRelationLinks().Has(bundle.RelationKeyLastModifiedVia.String()))
	require.NoError(t, fx.ResetToVersion(historical))

	// then: nothing is pushed — apart from the missing link, the version is
	// the document — and the page's last editor is still Alice. Red if the
	// missing link of the derived key is taken for a removal to sync: the
	// restore would push a change and make the restoring participant the
	// last editor, now.
	assert.Len(t, fx.source.attempts, pushedBefore)
	assert.Equal(t, alice, lastModifiedOf(fx.Doc.(*state.State)))
}
