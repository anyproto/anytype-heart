package state

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The integration name must be strictly per-apply (APIV2_OBJECT_DELETE.md
// §11.4): if any of these propagation paths ever starts carrying it, every
// later local edit inherits the creation stamp and the DELETE ownership gate
// silently widens. Each subtest names the exact leak it would catch.
func TestState_IntegrationNameIsNotPropagated(t *testing.T) {
	t.Run("set and get on one state", func(t *testing.T) {
		s := NewDoc("root", nil).(*State)
		s.SetIntegrationName("Claude Desktop")
		assert.Equal(t, "Claude Desktop", s.IntegrationName())
	})

	t.Run("NewState does not inherit", func(t *testing.T) {
		// the leak: every derived edit state would carry the creation stamp
		parent := NewDoc("root", nil).(*State)
		parent.SetIntegrationName("Claude Desktop")
		assert.Equal(t, "", parent.NewState().IntegrationName())
	})

	t.Run("Copy does not inherit", func(t *testing.T) {
		// the leak: template/snapshot copies would carry the stamp
		s := NewDoc("root", nil).(*State)
		s.SetIntegrationName("Claude Desktop")
		assert.Equal(t, "", s.Copy().IntegrationName())
	})

	t.Run("apply does not write it onto the parent doc", func(t *testing.T) {
		// the leak the spec calls out: if apply persisted the key on the doc
		// state, the NEXT apply's push would read it back and every subsequent
		// local change would be misattributed
		doc := NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root"}),
		}).(*State)
		s := doc.NewState()
		s.SetIntegrationName("Claude Desktop")
		s.Add(simple.New(&model.Block{Id: "child"}))
		require.NoError(t, s.InsertTo("root", model.Block_Inner, "child"))

		_, _, err := ApplyState("space1", s, false)
		require.NoError(t, err)

		assert.Equal(t, "", doc.IntegrationName())
		assert.Equal(t, "", doc.NewState().IntegrationName())
	})
}

// TakeIntegrationName is the one question smartblock.Apply asks an applied
// state (docs/ApiChangeAttribution.md "The carry"). Each subtest states the
// implementation change that turns it red.
func TestState_TakeIntegrationName(t *testing.T) {
	apiSession := func(name string) session.Context {
		return session.NewContext(session.WithIntegrationName(name))
	}
	newDoc := func() *State {
		return NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root"}),
		}).(*State)
	}

	t.Run("a state created for an API request answers with the request's app", func(t *testing.T) {
		// given
		s := newDoc().NewStateCtx(apiSession("Raycast"))
		want := "Raycast"

		// when
		got := s.TakeIntegrationName()

		// then: red if TakeIntegrationName stops reading the session context
		// — the carry most API edit routes depend on
		assert.Equal(t, want, got)
	})

	t.Run("a state created for an app request answers with nothing", func(t *testing.T) {
		// given: the gRPC shape — a session context that carries no app name
		s := newDoc().NewStateCtx(session.NewContext(session.WithSession("token")))

		// when
		got := s.TakeIntegrationName()

		// then: red if any non-empty default is ever returned
		assert.Equal(t, "", got)
	})

	t.Run("the explicit stamp wins over the session and is consumed", func(t *testing.T) {
		// given
		s := newDoc().NewStateCtx(apiSession("Raycast"))
		s.SetIntegrationName("Claude Desktop")

		// when
		first := s.TakeIntegrationName()

		// then: red if the explicit stamp is not consumed (a second apply of
		// the same state object would then push it again)
		assert.Equal(t, "Claude Desktop", first)
		assert.Equal(t, "", s.IntegrationName())
	})

	t.Run("StampFromCtx stamps from an API request context and from nothing else", func(t *testing.T) {
		// given
		apiCtx := domain.CtxWithIntegrationName(context.Background(), "Raycast")

		// when
		stamped := newDoc().NewState().StampFromCtx(apiCtx).TakeIntegrationName()
		plain := newDoc().NewState().StampFromCtx(context.Background()).TakeIntegrationName()

		// then: red if StampFromCtx stops reading the request context, or
		// starts stamping a context no API request made
		assert.Equal(t, "Raycast", stamped)
		assert.Equal(t, "", plain)
	})

	t.Run("an attribution-only context stamps the state and gives it no session", func(t *testing.T) {
		// given
		apiCtx := domain.CtxWithIntegrationName(context.Background(), "Raycast")

		// when
		s := newDoc().NewStateCtx(session.AttributionOnly(apiCtx))

		// then: red if NewStateCtx keeps the attribution-only context as the
		// state's session context — Apply would then hand the apply's events
		// to a context that drops them, instead of broadcasting them
		assert.Nil(t, s.Context())
		assert.Equal(t, "Raycast", s.TakeIntegrationName())
	})

	t.Run("no API request, no attribution-only context", func(t *testing.T) {
		// then: red if AttributionOnly ever returns a non-nil context for a
		// request that carries no app name — every such caller passed nil
		// before and must still pass nil
		assert.Nil(t, session.AttributionOnly(context.Background()))
	})

	t.Run("a layered edit state answers for the layer that holds the request", func(t *testing.T) {
		// given: a child of the edit state that was created for the request
		request := newDoc().NewStateCtx(apiSession("Raycast"))
		layered := request.NewState()

		// when
		got := layered.TakeIntegrationName()

		// then: red if only the applied state itself is consulted
		assert.Equal(t, "Raycast", got)
	})

	t.Run("the document is never consulted", func(t *testing.T) {
		// given: a document that (wrongly) holds an earlier request's session
		// context and explicit stamp — the shape a leak would produce
		doc := newDoc()
		doc.SetContext(apiSession("Raycast"))
		doc.SetIntegrationName("Raycast")

		// when: the next edit, made by an app
		got := doc.NewState().TakeIntegrationName()

		// then: red if the parent walk includes the parentless state — a
		// desktop edit after an API edit would then be attributed to the app
		assert.Equal(t, "", got)
	})
}

// lastModifiedVia is set or REMOVED, never left to linger.
func TestState_SetLastModifiedVia(t *testing.T) {
	t.Run("a name sets the detail, an empty name removes it", func(t *testing.T) {
		// given
		doc := NewDoc("root", nil).(*State)

		// when
		doc.SetLastModifiedVia("Raycast")
		set := doc.LocalDetails().GetString(bundle.RelationKeyLastModifiedVia)
		doc.SetLastModifiedVia("")

		// then: red if the empty branch stores "" or does nothing — an app
		// edit after an API edit would keep showing the app
		assert.Equal(t, "Raycast", set)
		assert.False(t, doc.LocalDetails().Has(bundle.RelationKeyLastModifiedVia))
		// derived: it must never reach the synced details (and so a change)
		assert.False(t, doc.Details().Has(bundle.RelationKeyLastModifiedVia))
	})
}

func TestState_RemoveLocalDetail(t *testing.T) {
	newDocWith := func(details map[domain.RelationKey]domain.Value) *State {
		doc := NewDoc("root", nil).(*State)
		doc.SetLocalDetails(domain.NewDetailsFromMap(details))
		return doc
	}

	t.Run("removing an absent key allocates nothing", func(t *testing.T) {
		// given: the every-edit case — an app edit removes lastModifiedVia
		// from an object that has none — on a parent with a realistic number
		// of local details
		details := map[domain.RelationKey]domain.Value{}
		for _, key := range bundle.LocalAndDerivedRelationKeys {
			if key != bundle.RelationKeyLastModifiedVia {
				details[key] = domain.String("value of " + key.String())
			}
		}
		doc := newDocWith(details)
		s := doc.NewState()

		// when
		allocs := testing.AllocsPerRun(100, func() {
			s.RemoveLocalDetail(bundle.RelationKeyLastModifiedVia)
		})

		// then: red if the local details are copied before the key is looked
		// for — one map copy per unattributed edit
		assert.Zero(t, allocs)
		assert.Nil(t, s.localDetails)
	})

	t.Run("removing a present key removes it from the child only", func(t *testing.T) {
		// given
		doc := newDocWith(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:            domain.String("n"),
			bundle.RelationKeyLastModifiedVia: domain.String("Raycast"),
		})
		s := doc.NewState()

		// when
		removed := s.RemoveLocalDetail(bundle.RelationKeyLastModifiedVia)

		// then: red if the presence check short-circuits a real removal, or
		// the parent's map is edited in place
		assert.True(t, removed)
		assert.False(t, s.LocalDetails().Has(bundle.RelationKeyLastModifiedVia))
		assert.Equal(t, "n", s.LocalDetails().GetString(bundle.RelationKeyName))
		assert.True(t, doc.LocalDetails().Has(bundle.RelationKeyLastModifiedVia))
	})
}
