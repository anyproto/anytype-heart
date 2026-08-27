package notion

import (
	"context"
	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func tagProperty(id, name string) propertySchema {
	return propertySchema{Id: id, Type: "multi_select", Name: name}
}

func TestTagRedirectIsSpaceWide(t *testing.T) {
	t.Run("every database's Tags property redirects onto the one bundled relation", func(t *testing.T) {
		// given — two databases each carrying a "Tags" multi_select, which the
		// committed cassette also contains (ids Bfgr and yq%7B~). A shared
		// vocabulary across every type is the whole point of tags, and the one
		// bundled target kept for exactly that reason.
		store := newPropertiesStore()
		first := tagProperty("Bfgr", "Tags")
		second := tagProperty("yq%7B~", "Tags")
		store.noteName(first.Name)
		store.noteName(second.Name)

		// when
		firstDef, firstCreated := store.resolveRelation("db1", first)
		secondDef, secondCreated := store.resolveRelation("db2", second)

		// then
		require.NotNil(t, firstDef)
		require.NotNil(t, secondDef)
		assert.Equal(t, bundle.RelationKeyTag.String(), firstDef.key)
		assert.Equal(t, bundle.RelationKeyTag.String(), secondDef.key,
			"a later database's Tags must not mint a private relation with its own option pool")
		assert.False(t, firstCreated, "the bundled tag relation is never emitted as a new object")
		assert.False(t, secondCreated)
	})

	t.Run("an exact Tag property still wins over Tags", func(t *testing.T) {
		// given — v1's precedence rule: "Tags" only redirects when no property
		// named exactly "Tag" exists anywhere
		store := newPropertiesStore()
		store.noteName("Tag")
		store.noteName("Tags")

		// when
		tagsDef, _ := store.resolveRelation("db1", tagProperty("p1", "Tags"))
		tagDef, _ := store.resolveRelation("db2", tagProperty("p2", "Tag"))

		// then
		require.NotNil(t, tagsDef)
		require.NotNil(t, tagDef)
		assert.NotEqual(t, bundle.RelationKeyTag.String(), tagsDef.key,
			"Tags must not take the bundled relation when an exact Tag exists")
		assert.Equal(t, bundle.RelationKeyTag.String(), tagDef.key)
	})

	t.Run("same-named properties in different databases stay separate", func(t *testing.T) {
		// given — the c6e29db27 rule: a property belongs to its own database
		store := newPropertiesStore()
		first := propertySchema{Id: "catA", Type: "select", Name: "Category"}
		second := propertySchema{Id: "catB", Type: "select", Name: "Category"}

		// when
		firstDef, _ := store.resolveRelation("db1", first)
		secondDef, _ := store.resolveRelation("db2", second)

		// then
		require.NotNil(t, firstDef)
		require.NotNil(t, secondDef)
		assert.NotEqual(t, firstDef.key, secondDef.key)
	})
}

func TestPropertyIdIsScopedToItsDatabase(t *testing.T) {
	t.Run("the same property id in two databases is two relations", func(t *testing.T) {
		// given — Notion only guarantees property ids unique WITHIN a database.
		// A real workspace's teamspace templates use slug ids, so "Docs" and
		// "Meetings" both carry a relation property with id "project" — and
		// Notion says they differ: each has its own dual_property back
		// reference on the target database.
		store := newPropertiesStore()
		docs := propertySchema{Id: "project", Type: "select", Name: "Project"}
		meetings := propertySchema{Id: "project", Type: "select", Name: "Project"}

		// when
		docsDef, docsCreated := store.resolveRelation("docsDb", docs)
		meetingsDef, meetingsCreated := store.resolveRelation("meetingsDb", meetings)

		// then
		require.NotNil(t, docsDef)
		require.NotNil(t, meetingsDef)
		assert.True(t, docsCreated)
		assert.True(t, meetingsCreated, "the second database needs its own relation")
		assert.NotEqual(t, docsDef.key, meetingsDef.key,
			"two databases' properties collapsed onto one relation and one option pool")
	})

	t.Run("a database and its pages still share one relation", func(t *testing.T) {
		// given — the one case that MUST collapse: a page's property carries
		// the same id as its database's schema declared
		store := newPropertiesStore()
		schema := propertySchema{Id: "prio", Type: "select", Name: "Priority"}

		// when
		fromSchema, created := store.resolveRelation("db1", schema)
		fromPage, createdAgain := store.resolveRelation("db1", schema)

		// then
		require.NotNil(t, fromSchema)
		require.NotNil(t, fromPage)
		assert.True(t, created)
		assert.False(t, createdAgain, "the page must reuse its database's relation")
		assert.Equal(t, fromSchema.key, fromPage.key)
	})

	t.Run("a plan target is applied per database, not shadowed by the first", func(t *testing.T) {
		// given — Sanitize scopes the two plans apart; the store must not
		// short-circuit the second on a property-id hit and discard its plan
		store := newPropertiesStore()
		property := propertySchema{Id: "project", Type: "select", Name: "Project"}

		// when
		first, _ := store.resolvePlanTarget("docsDb", property,
			schemaplan.PropertyPlan{Key: "project@doc", Format: model.RelationFormat_status})
		second, _ := store.resolvePlanTarget("meetingsDb", property,
			schemaplan.PropertyPlan{Key: "project@meeting", Format: model.RelationFormat_status})

		// then
		require.NotNil(t, first)
		require.NotNil(t, second)
		assert.NotEqual(t, first.key, second.key, "the second database's plan entry was discarded")
	})
}

func TestNoteContainerCountsSharing(t *testing.T) {
	// The plan pre-registers every relation it minted, so "did this call
	// create it?" cannot tell a shared property from a lonely one. This can.
	store := newPropertiesStore()

	t.Run("the first container to arrive shares with nobody", func(t *testing.T) {
		assert.Equal(t, 0, store.noteContainer("aipropdesc", "db1"))
	})

	t.Run("the second one joins it", func(t *testing.T) {
		assert.Equal(t, 1, store.noteContainer("aipropdesc", "db2"))
		assert.Equal(t, 2, store.noteContainer("aipropdesc", "db3"))
	})

	t.Run("the same container asking twice is still one container", func(t *testing.T) {
		assert.Equal(t, 2, store.noteContainer("aipropdesc", "db3"))
	})

	t.Run("relations are counted apart", func(t *testing.T) {
		assert.Equal(t, 0, store.noteContainer("aipropother", "db1"))
	})
}

func TestPlannedPropertyNoteSaysWhatHappened(t *testing.T) {
	// given — two databases whose "Description" column the plan pointed at
	// one relation, and a third column the plan left alone by name
	converter := New(nil, nil, stubFactory{}, t.TempDir())
	sink := &recordingSink{}
	plan := schemaplan.PropertyPlan{Key: "summary", Name: "Description", Format: model.RelationFormat_longtext}
	property := propertySchema{Id: "p1", Type: "rich_text", Name: "Description"}

	notes := func() []string {
		var out []string
		for _, issue := range sink.issues {
			if issue.Code == importv2.IssuePropertyMapped {
				out = append(out, issue.Message+" — "+issue.Subject)
			}
		}
		return out
	}

	// when — the first database arrives
	_, err := converter.emitProperty(context.Background(), "db1", property, plan,
		container{key: "db1", name: "Templates", schema: true}, sink)
	require.NoError(t, err)

	// then — nothing happened yet worth telling anyone
	assert.Empty(t, notes(), "a column imported under its own name is not news")

	// when — a second database's column of the same kind arrives
	_, err = converter.emitProperty(context.Background(), "db2", property, plan,
		container{key: "db2", name: "Sprints", schema: true}, sink)
	require.NoError(t, err)

	// then — the join is the news: one property instead of two look-alikes
	require.Len(t, notes(), 1)
	assert.Contains(t, notes()[0], "same property as another database")
	assert.Contains(t, notes()[0], "Description")
}
