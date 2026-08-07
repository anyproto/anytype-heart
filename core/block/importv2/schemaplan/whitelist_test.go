// External test package: the fixture-suite tests import planfixture, which
// itself imports schemaplan — an internal test package would be a cycle.
package schemaplan_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan/planfixture"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// fixture is the shared test harness: schemas in, kinds in, sanitized plan
// out, with the §4.4 structural claim (zero drops) asserted on every run.
type fixture struct {
	t       *testing.T
	schemas []schemaplan.ContainerSchema
}

func newFixture(t *testing.T, schemas []schemaplan.ContainerSchema) *fixture {
	return &fixture{t: t, schemas: schemas}
}

// complete runs CompleteKinds and asserts the raw plan is returned unharmed.
func (fx *fixture) complete(kinds []schemaplan.KindPlan) schemaplan.Plan {
	return schemaplan.CompleteKinds(kinds, fx.schemas)
}

// completeSanitized runs CompleteKinds then Sanitize, requiring zero dropped
// entries — the structural claim of §4.4, asserted, not assumed.
func (fx *fixture) completeSanitized(kinds []schemaplan.KindPlan) schemaplan.Plan {
	fx.t.Helper()
	plan := fx.complete(kinds)
	var issues []importv2.Issue
	clean := schemaplan.Sanitize(plan, fx.schemas, func(issue importv2.Issue) { issues = append(issues, issue) })
	require.Empty(fx.t, issues, "CompleteKinds output must sanitize with zero drops")
	return clean
}

func selectProperty(id, name string, options ...string) schemaplan.PropertySchema {
	return schemaplan.PropertySchema{Id: id, Name: name, Format: model.RelationFormat_status, Options: options}
}

func property(id, name string, format model.RelationFormat) schemaplan.PropertySchema {
	return schemaplan.PropertySchema{Id: id, Name: name, Format: format}
}

func TestCompleteKindsBundledRules(t *testing.T) {
	t.Run("email format rule catches decorated names", func(t *testing.T) {
		// given — the real workspace's "Email 📧 " with a trailing space; a
		// name table would miss it, the format rule cannot
		fx := newFixture(t, []schemaplan.ContainerSchema{{
			Id: "c1", Name: "Speakers",
			Properties: []schemaplan.PropertySchema{
				property("p1", "Email 📧 ", model.RelationFormat_email),
				property("p2", "Talk Title", model.RelationFormat_longtext),
			},
		}})
		kinds := []schemaplan.KindPlan{{Name: "Speaker", ContainerIds: []string{"c1"}}}

		// when
		plan := fx.completeSanitized(kinds)

		// then
		assert.Equal(t, bundle.RelationKeyEmail, plan.Containers["c1"].Properties["p1"].Key)
	})

	t.Run("two email properties mean no mapping", func(t *testing.T) {
		// given — ambiguity degrades to no mapping, never a sanitizer drop
		fx := newFixture(t, []schemaplan.ContainerSchema{{
			Id: "c1", Name: "Contacts",
			Properties: []schemaplan.PropertySchema{
				property("p1", "Email", model.RelationFormat_email),
				property("p2", "Work Email", model.RelationFormat_email),
			},
		}})
		kinds := []schemaplan.KindPlan{{Name: "Contact", ContainerIds: []string{"c1"}}}

		// when
		plan := fx.completeSanitized(kinds)

		// then — both stay kind-local, neither is the bundled email
		for _, id := range []string{"p1", "p2"} {
			key := plan.Containers["c1"].Properties[id].Key
			assert.False(t, bundle.HasRelation(key), "property %s must not reach a bundled relation", id)
		}
	})

	t.Run("sole phone property maps to phone", func(t *testing.T) {
		// given
		fx := newFixture(t, []schemaplan.ContainerSchema{{
			Id: "c1", Name: "Vendors",
			Properties: []schemaplan.PropertySchema{
				property("p1", "Mobile", model.RelationFormat_phone),
			},
		}})
		kinds := []schemaplan.KindPlan{{Name: "Vendor", ContainerIds: []string{"c1"}}}

		// when
		plan := fx.completeSanitized(kinds)

		// then
		assert.Equal(t, bundle.RelationKeyPhone, plan.Containers["c1"].Properties["p1"].Key)
	})

	t.Run("due token rule matches whole words only", func(t *testing.T) {
		// given — "Bid Due Date" carries the token; the negative list encodes
		// the measured workspace's real non-due date names plus the LLM's
		// deliberately-lost semantic guesses
		negatives := []string{
			"Overdue", "Created Date", "Creation Date", "Reported Date",
			"Requested Date", "Start Date", "Timeline", "Last edited time",
			"Created on", "Created time", "Publish Date", "Launch Date", "Do Date",
		}
		properties := []schemaplan.PropertySchema{
			property("pDue", "Bid Due Date", model.RelationFormat_date),
		}
		for i, name := range negatives {
			properties = append(properties, property("pNeg"+string(rune('a'+i)), name, model.RelationFormat_date))
		}
		fx := newFixture(t, []schemaplan.ContainerSchema{{Id: "c1", Name: "Bids", Properties: properties}})
		kinds := []schemaplan.KindPlan{{Name: "Bid", ContainerIds: []string{"c1"}}}

		// when
		plan := fx.completeSanitized(kinds)

		// then — the sole token match maps, every negative stays kind-local
		container := plan.Containers["c1"]
		assert.Equal(t, bundle.RelationKeyDueDate, container.Properties["pDue"].Key)
		for id, entry := range container.Properties {
			if id == "pDue" {
				continue
			}
			assert.False(t, bundle.HasRelation(entry.Key), "property %s must not reach a bundled relation", id)
		}
	})

	t.Run("two due-token dates mean no mapping", func(t *testing.T) {
		// given
		fx := newFixture(t, []schemaplan.ContainerSchema{{
			Id: "c1", Name: "Tasks Area",
			Properties: []schemaplan.PropertySchema{
				property("p1", "Due Date", model.RelationFormat_date),
				property("p2", "Deadline", model.RelationFormat_date),
			},
		}})
		kinds := []schemaplan.KindPlan{{Name: "Task Item", ContainerIds: []string{"c1"}}}

		// when
		plan := fx.completeSanitized(kinds)

		// then
		for _, id := range []string{"p1", "p2"} {
			assert.False(t, bundle.HasRelation(plan.Containers["c1"].Properties[id].Key))
		}
	})

	t.Run("done matches none of the measured checkbox names", func(t *testing.T) {
		// given — the real workspace's 15 checkbox names (deduped): every one
		// is a state flag, none reads as completion
		measured := []string{
			"Featured", "Pin To Dashboard?", "Important", "Urgent", "Favourite?",
			"Action", "Capture", "Home", "Master", "Plan", "Track", "Launched",
		}
		var properties []schemaplan.PropertySchema
		for i, name := range measured {
			properties = append(properties, property("p"+string(rune('a'+i)), name, model.RelationFormat_checkbox))
		}
		fx := newFixture(t, []schemaplan.ContainerSchema{{Id: "c1", Name: "Launch Tracker", Properties: properties}})
		kinds := []schemaplan.KindPlan{{Name: "Launch", ContainerIds: []string{"c1"}}}

		// when
		plan := fx.completeSanitized(kinds)

		// then
		for id, entry := range plan.Containers["c1"].Properties {
			assert.False(t, bundle.HasRelation(entry.Key), "checkbox %s must not become done", id)
		}
	})

	t.Run("completion checkbox names map to done", func(t *testing.T) {
		// given — "Resolved?" and "Got It?" are the two evidence-supported
		// additions to the completion set
		for _, name := range []string{"Done?", "Resolved?", "Got It?"} {
			fx := newFixture(t, []schemaplan.ContainerSchema{{
				Id: "c1", Name: "List",
				Properties: []schemaplan.PropertySchema{
					property("p1", name, model.RelationFormat_checkbox),
				},
			}})
			kinds := []schemaplan.KindPlan{{Name: "Item", ContainerIds: []string{"c1"}}}

			// when
			plan := fx.completeSanitized(kinds)

			// then
			assert.Equal(t, bundle.RelationKeyDone, plan.Containers["c1"].Properties["p1"].Key, name)
		}
	})

	t.Run("genre stays per-kind instead of joining a space-wide pool", func(t *testing.T) {
		// given — a bookshelf and a record collection, each with a "Genre".
		// genre is deliberately NOT an allowed bundled target: its option pool
		// would be space-wide, pouring Shoegaze in beside Memoir.
		fx := newFixture(t, []schemaplan.ContainerSchema{
			{Id: "c1", Name: "Reading List", Properties: []schemaplan.PropertySchema{
				{Id: "p1", Name: "Genre", Format: model.RelationFormat_tag,
					Options: []string{"Fantasy", "Memoir"}},
			}},
			{Id: "c2", Name: "Vinyl Shelf", Properties: []schemaplan.PropertySchema{
				{Id: "q1", Name: "Genre", Format: model.RelationFormat_tag,
					Options: []string{"Shoegaze", "Ambient"}},
			}},
		})
		kinds := []schemaplan.KindPlan{
			{Name: "Book", ContainerIds: []string{"c1"}},
			{Name: "Record", ContainerIds: []string{"c2"}},
		}

		// when
		plan := fx.completeSanitized(kinds)

		// then
		book := plan.Containers["c1"].Properties["p1"].Key
		record := plan.Containers["c2"].Properties["q1"].Key
		assert.NotEqual(t, bundle.RelationKeyGenre, book, "must not join the space-wide pool")
		assert.NotEqual(t, book, record, "each media kind keeps its own genre vocabulary")
	})

	t.Run("one kind's members still share their genre relation", func(t *testing.T) {
		// given — the counterpart: three family reading lists of one kind read
		// the same vocabulary, so per-kind scoping still merges them
		fx := newFixture(t, []schemaplan.ContainerSchema{
			{Id: "c1", Name: "Mia's Books", Properties: []schemaplan.PropertySchema{
				{Id: "p1", Name: "Genre", Format: model.RelationFormat_tag,
					Options: []string{"Fantasy", "Memoir"}},
			}},
			{Id: "c2", Name: "Leo's Books", Properties: []schemaplan.PropertySchema{
				{Id: "q1", Name: "Genre", Format: model.RelationFormat_tag,
					Options: []string{"Fantasy", "Sci-fi"}},
			}},
		})
		kinds := []schemaplan.KindPlan{{Name: "Book", ContainerIds: []string{"c1", "c2"}}}

		// when
		plan := fx.completeSanitized(kinds)

		// then
		assert.Equal(t, plan.Containers["c1"].Properties["p1"].Key,
			plan.Containers["c2"].Properties["q1"].Key)
	})

	t.Run("tag-shaped properties produce no plan entry", func(t *testing.T) {
		// given — the shipped notion tag redirect owns these; a plan entry
		// would shadow its global Tags-only-when-no-Tag-exists latch
		fx := newFixture(t, []schemaplan.ContainerSchema{{
			Id: "c1", Name: "Calendar",
			Properties: []schemaplan.PropertySchema{
				{Id: "p1", Name: "Tags", Format: model.RelationFormat_tag, Options: []string{"Work", "Home"}},
				{Id: "p2", Name: "Tag", Format: model.RelationFormat_status, Options: []string{"A", "B"}},
				property("p3", "Where", model.RelationFormat_longtext),
			},
		}})
		kinds := []schemaplan.KindPlan{{Name: "Event", ContainerIds: []string{"c1"}}}

		// when
		plan := fx.completeSanitized(kinds)

		// then
		container := plan.Containers["c1"]
		assert.NotContains(t, container.Properties, "p1")
		assert.NotContains(t, container.Properties, "p2")
		assert.Contains(t, container.Properties, "p3")
	})
}

func TestCompleteKindsSharing(t *testing.T) {
	duplicated := func() []schemaplan.ContainerSchema {
		return []schemaplan.ContainerSchema{
			{Id: "c1", Name: "Premium Templates", Properties: []schemaplan.PropertySchema{
				selectProperty("p1", "Priority", "High", "Medium", "Low"),
				property("p2", "Notes", model.RelationFormat_longtext),
			}},
			{Id: "c2", Name: "Premium Templates 2", Properties: []schemaplan.PropertySchema{
				selectProperty("q1", "Priority", "High", "Medium", "Low"),
				property("q2", "Notes", model.RelationFormat_longtext),
			}},
		}
	}

	t.Run("duplicated databases share one relation per identical property", func(t *testing.T) {
		// given
		fx := newFixture(t, duplicated())
		kinds := []schemaplan.KindPlan{{Name: "Template", ContainerIds: []string{"c1", "c2"}}}

		// when
		clean := fx.completeSanitized(kinds)

		// then — byte-identical (name, format) derives one shared key
		require.Contains(t, clean.Containers, "c1")
		require.Contains(t, clean.Containers, "c2")
		assert.Equal(t, clean.Containers["c1"].Properties["p1"].Key, clean.Containers["c2"].Properties["q1"].Key)
		assert.Equal(t, clean.Containers["c1"].Properties["p2"].Key, clean.Containers["c2"].Properties["q2"].Key)
	})

	t.Run("different spelling or format stays separate", func(t *testing.T) {
		// given
		fx := newFixture(t, []schemaplan.ContainerSchema{
			{Id: "c1", Name: "A", Properties: []schemaplan.PropertySchema{
				property("p1", "Owner", model.RelationFormat_longtext),
				property("p2", "Qty", model.RelationFormat_longtext),
			}},
			{Id: "c2", Name: "B", Properties: []schemaplan.PropertySchema{
				property("q1", "owner", model.RelationFormat_longtext), // spelling differs
				property("q2", "Qty", model.RelationFormat_number),     // format differs
			}},
		})
		kinds := []schemaplan.KindPlan{{Name: "Thing", ContainerIds: []string{"c1", "c2"}}}

		// when
		clean := fx.completeSanitized(kinds)

		// then
		assert.NotEqual(t, clean.Containers["c1"].Properties["p1"].Key, clean.Containers["c2"].Properties["q1"].Key)
		assert.NotEqual(t, clean.Containers["c1"].Properties["p2"].Key, clean.Containers["c2"].Properties["q2"].Key)
	})

	t.Run("two kinds' same-named selects never share", func(t *testing.T) {
		// given — same (name, format, options) but different kinds: scoping by
		// type key keeps them apart (the four-databases-one-Category defense)
		fx := newFixture(t, []schemaplan.ContainerSchema{
			{Id: "c1", Name: "Recipes", Properties: []schemaplan.PropertySchema{
				selectProperty("p1", "Category", "Breakfast", "Dinner"),
			}},
			{Id: "c2", Name: "Launches", Properties: []schemaplan.PropertySchema{
				selectProperty("q1", "Category", "Breakfast", "Dinner"),
			}},
		})
		kinds := []schemaplan.KindPlan{
			{Name: "Recipe Card", ContainerIds: []string{"c1"}},
			{Name: "Launch", ContainerIds: []string{"c2"}},
		}

		// when
		clean := fx.completeSanitized(kinds)

		// then
		assert.NotEqual(t, clean.Containers["c1"].Properties["p1"].Key, clean.Containers["c2"].Properties["q1"].Key)
	})

	t.Run("option vocabulary guard vetoes disagreeing selects within a kind", func(t *testing.T) {
		// given — same name and format, zero option overlap: sharing would
		// merge two vocabularies into one dropdown
		fx := newFixture(t, []schemaplan.ContainerSchema{
			{Id: "c1", Name: "Content Planner", Properties: []schemaplan.PropertySchema{
				selectProperty("p1", "Status", "Drafted", "Idea", "Posted"),
				property("p2", "Notes", model.RelationFormat_longtext),
			}},
			{Id: "c2", Name: "Notebooks", Properties: []schemaplan.PropertySchema{
				selectProperty("q1", "Status", "Done", "In progress", "Inbox"),
				property("q2", "Notes", model.RelationFormat_longtext),
			}},
		})
		kinds := []schemaplan.KindPlan{{Name: "Note Doc", ContainerIds: []string{"c1", "c2"}}}

		// when
		clean := fx.completeSanitized(kinds)

		// then — the vetoed Status splits, the agreeing Notes still shares
		assert.NotEqual(t, clean.Containers["c1"].Properties["p1"].Key, clean.Containers["c2"].Properties["q1"].Key)
		assert.Equal(t, clean.Containers["c1"].Properties["p2"].Key, clean.Containers["c2"].Properties["q2"].Key)
	})

	t.Run("option vocabulary guard shares agreeing selects", func(t *testing.T) {
		// given — overlap 2/3 clears the half-the-smaller-set bar
		fx := newFixture(t, []schemaplan.ContainerSchema{
			{Id: "c1", Name: "Sprint", Properties: []schemaplan.PropertySchema{
				selectProperty("p1", "Priority", "High", "Medium", "Low"),
			}},
			{Id: "c2", Name: "Backlog Items", Properties: []schemaplan.PropertySchema{
				selectProperty("q1", "Priority", "High", "Medium", "Urgent"),
			}},
		})
		kinds := []schemaplan.KindPlan{{Name: "Work Item", ContainerIds: []string{"c1", "c2"}}}

		// when
		clean := fx.completeSanitized(kinds)

		// then
		assert.Equal(t, clean.Containers["c1"].Properties["p1"].Key, clean.Containers["c2"].Properties["q1"].Key)
	})
}

func TestCompleteKindsCoverageGate(t *testing.T) {
	t.Run("bloated merge is split into single-container kinds", func(t *testing.T) {
		// given — the motivating failure: a tiny tracker merged into a
		// 20-relation type it fills three fields of
		small := schemaplan.ContainerSchema{Id: "c1", Name: "Tasks", Properties: []schemaplan.PropertySchema{
			property("p1", "Name", model.RelationFormat_longtext),
			property("p2", "Done?", model.RelationFormat_checkbox),
			property("p3", "Due Date", model.RelationFormat_date),
		}}
		var bigProperties []schemaplan.PropertySchema
		bigProperties = append(bigProperties, small.Properties...)
		for i := 0; i < 17; i++ {
			bigProperties = append(bigProperties,
				property("q"+string(rune('a'+i)), "Field "+string(rune('A'+i)), model.RelationFormat_longtext))
		}
		big := schemaplan.ContainerSchema{Id: "c2", Name: "Sprint Planner", Properties: bigProperties}
		fx := newFixture(t, []schemaplan.ContainerSchema{small, big})
		kinds := []schemaplan.KindPlan{{
			Name: "Task Entry", IconName: "checkbox", Layout: model.ObjectType_todo,
			ContainerIds: []string{"c1", "c2"},
		}}

		// when
		plan := fx.complete(kinds)
		clean := fx.completeSanitized(kinds)

		// then — split wholesale: each member gets its own minted kind, named
		// from its container title, keeping the model's icon and layout
		require.Len(t, plan.NewTypes, 2)
		names := []string{plan.NewTypes[0].Name, plan.NewTypes[1].Name}
		assert.ElementsMatch(t, []string{"Tasks", "Sprint Planner"}, names)
		for _, def := range plan.NewTypes {
			assert.Equal(t, "checkbox", def.IconName)
			assert.Equal(t, model.ObjectType_todo, def.Layout)
		}
		assert.NotEqual(t, clean.Containers["c1"].TypeKey, clean.Containers["c2"].TypeKey)
	})

	t.Run("sound merge with partial coverage survives", func(t *testing.T) {
		// given — the real Tasks + Tasks & Features pair: coverage 0.60/1.00
		shared := []schemaplan.PropertySchema{
			property("p1", "Name", model.RelationFormat_longtext),
			property("p2", "Due Date", model.RelationFormat_date),
			property("p3", "Done?", model.RelationFormat_checkbox),
		}
		extra := []schemaplan.PropertySchema{
			property("p4", "Effort", model.RelationFormat_number),
			property("p5", "Spec", model.RelationFormat_url),
		}
		fx := newFixture(t, []schemaplan.ContainerSchema{
			{Id: "c1", Name: "Tasks", Properties: shared},
			{Id: "c2", Name: "Tasks & Features", Properties: append(append([]schemaplan.PropertySchema{}, shared...), extra...)},
		})
		kinds := []schemaplan.KindPlan{{Name: "Task Entry", ContainerIds: []string{"c1", "c2"}}}

		// when
		clean := fx.completeSanitized(kinds)

		// then
		require.Len(t, clean.NewTypes, 1)
		assert.Equal(t, clean.Containers["c1"].TypeKey, clean.Containers["c2"].TypeKey)
	})
}

func TestCompleteKindsTypeDerivation(t *testing.T) {
	t.Run("slug collision gets a deterministic suffix", func(t *testing.T) {
		// given — two kind names normalizing to one slug
		fx := newFixture(t, []schemaplan.ContainerSchema{
			{Id: "c1", Name: "A", Properties: []schemaplan.PropertySchema{property("p1", "X", model.RelationFormat_longtext)}},
			{Id: "c2", Name: "B", Properties: []schemaplan.PropertySchema{property("q1", "Y", model.RelationFormat_longtext)}},
		})
		kinds := []schemaplan.KindPlan{
			{Name: "Launch Task", ContainerIds: []string{"c1"}},
			{Name: "launch task!", ContainerIds: []string{"c2"}},
		}

		// when
		plan := fx.complete(kinds)
		fx.completeSanitized(kinds)

		// then
		require.Len(t, plan.NewTypes, 2)
		assert.Equal(t, domain.TypeKey("launch-task"), plan.NewTypes[0].Key)
		assert.Equal(t, domain.TypeKey("launch-task-2"), plan.NewTypes[1].Key)
	})

	t.Run("kind slugging onto a bundled type key is disambiguated before sanitize", func(t *testing.T) {
		// given — "Task" would slug to the bundled key `task`. The collision is
		// avoided here rather than left to sanitizeNewTypes' re-key, because
		// that rename table is applied to every container plan's TypeKey —
		// including the bundled verdicts unassigned containers inherit from
		// typesuggest — which would pull naive-typed `task` containers onto
		// this minted type behind the coverage gate's back.
		fx := newFixture(t, []schemaplan.ContainerSchema{
			{Id: "c1", Name: "Chores", Properties: []schemaplan.PropertySchema{property("p1", "X", model.RelationFormat_longtext)}},
		})
		kinds := []schemaplan.KindPlan{{Name: "Task", ContainerIds: []string{"c1"}}}

		// when
		clean := fx.completeSanitized(kinds)

		// then
		require.Len(t, clean.NewTypes, 1)
		assert.Equal(t, domain.TypeKey("kind-task"), clean.NewTypes[0].Key)
		assert.Equal(t, domain.TypeKey("kind-task"), clean.Containers["c1"].TypeKey)
	})

	t.Run("unassigned container gets the typesuggest verdict and bundled-only mappings", func(t *testing.T) {
		// given — the historical-defect regression test for the fallback path:
		// naive verdicts are bundled type keys, and kind-local keys there would
		// let two unrelated `task` databases share every same-named select
		fx := newFixture(t, []schemaplan.ContainerSchema{{
			Id: "c1", Name: "Tasks",
			Properties: []schemaplan.PropertySchema{
				property("p1", "Due Date", model.RelationFormat_date),
				selectProperty("p2", "Category", "Home", "Work"),
			},
		}})

		// when
		plan := fx.completeSanitized(nil)

		// then
		container := plan.Containers["c1"]
		assert.Equal(t, bundle.TypeKeyTask, container.TypeKey)
		assert.Equal(t, "container name", container.Reason)
		assert.Equal(t, bundle.RelationKeyDueDate, container.Properties["p1"].Key)
		assert.NotContains(t, container.Properties, "p2", "no kind-local keys without a minted kind")
		assert.Empty(t, plan.NewTypes)
	})

	t.Run("featured names resolve by exact trimmed match and cap at four", func(t *testing.T) {
		// given — five names: one decorated source name matched after trim,
		// one miss dropped silently, and the fifth cut by the cap
		fx := newFixture(t, []schemaplan.ContainerSchema{{
			Id: "c1", Name: "Vendors",
			Properties: []schemaplan.PropertySchema{
				property("p1", "Email 📧 ", model.RelationFormat_email),
				property("p2", "Cost", model.RelationFormat_number),
				property("p3", "Notes", model.RelationFormat_longtext),
				property("p4", "City", model.RelationFormat_longtext),
				property("p5", "Country", model.RelationFormat_longtext),
			},
		}})
		kinds := []schemaplan.KindPlan{{
			Name:          "Vendor",
			ContainerIds:  []string{"c1"},
			FeaturedNames: []string{"Email 📧", "No Such Property", "Cost", "Notes", "City"},
		}}

		// when
		plan := fx.complete(kinds)
		fx.completeSanitized(kinds)

		// then — Email (trim match), Cost, Notes are featured in the model's
		// order; "City" fell past the cap, the miss cost only its slot
		require.Len(t, plan.NewTypes, 1)
		var featured []domain.RelationKey
		for _, prop := range plan.NewTypes[0].Properties {
			if prop.Featured {
				featured = append(featured, prop.Key)
			}
		}
		require.Len(t, featured, 3)
		assert.Equal(t, bundle.RelationKeyEmail, featured[0])
	})

	t.Run("ambiguous featured name resolves to the majority format", func(t *testing.T) {
		// given — "Ref" is text in one member and number in two
		fx := newFixture(t, []schemaplan.ContainerSchema{
			{Id: "c1", Name: "A", Properties: []schemaplan.PropertySchema{property("p1", "Ref", model.RelationFormat_longtext)}},
			{Id: "c2", Name: "B", Properties: []schemaplan.PropertySchema{property("q1", "Ref", model.RelationFormat_number)}},
			{Id: "c3", Name: "C", Properties: []schemaplan.PropertySchema{property("r1", "Ref", model.RelationFormat_number)}},
		})
		kinds := []schemaplan.KindPlan{{Name: "Record", ContainerIds: []string{"c1", "c2", "c3"}, FeaturedNames: []string{"Ref"}}}

		// when
		plan := fx.complete(kinds)

		// then — the number pair is featured, the text one is a regular entry
		require.Len(t, plan.NewTypes, 1)
		var featured []schemaplan.TypeProperty
		for _, prop := range plan.NewTypes[0].Properties {
			if prop.Featured {
				featured = append(featured, prop)
			}
		}
		require.Len(t, featured, 1)
		assert.Equal(t, model.RelationFormat_number, featured[0].Format)
	})
}

// --- fixture-suite quality tests (design §8) ---

// suiteKinds builds the grouping the suite's expectations describe: every
// sameKind group is one kind, every remaining container its own kind.
func suiteKinds(f planfixture.Fixture) []schemaplan.KindPlan {
	nameOf := map[string]string{}
	for _, container := range f.Containers {
		nameOf[container.Id] = container.Name
	}
	grouped := map[string]bool{}
	var kinds []schemaplan.KindPlan
	for _, group := range f.Expect.SameKind {
		kinds = append(kinds, schemaplan.KindPlan{Name: nameOf[group[0]], ContainerIds: group})
		for _, containerId := range group {
			grouped[containerId] = true
		}
	}
	for _, container := range f.Containers {
		if !grouped[container.Id] {
			kinds = append(kinds, schemaplan.KindPlan{Name: container.Name, ContainerIds: []string{container.Id}})
		}
	}
	return kinds
}

func TestWhitelistFixtureSuite(t *testing.T) {
	fixtures, err := planfixture.All()
	require.NoError(t, err)
	require.NotEmpty(t, fixtures)

	for _, f := range fixtures {
		t.Run(f.Id, func(t *testing.T) {
			// given
			fx := newFixture(t, f.Schemas())
			kinds := suiteKinds(f)

			// when — sanitize also proves the zero-drop structural claim on
			// every fixture
			plan := fx.complete(kinds)
			fx.completeSanitized(kinds)

			// then — every asserted bundled hit lands
			for containerId, mapping := range f.Expect.Bundled {
				container := plan.Containers[containerId]
				for propertyId, target := range mapping {
					if target == bundle.RelationKeyTag {
						// The tag target is owned by the shipped notion
						// redirect: the matcher SKIPS the property so it stays
						// unplanned and reaches the redirect (§4.1).
						assert.NotContains(t, container.Properties, propertyId,
							"%s.%s must stay unplanned for the tag redirect", containerId, propertyId)
						continue
					}
					require.Contains(t, container.Properties, propertyId,
						"%s.%s must be planned", containerId, propertyId)
					assert.Equal(t, target, container.Properties[propertyId].Key,
						"%s.%s must map to %s", containerId, propertyId, target)
				}
			}

			// then — THE TRAPS: not one asserted trap may reach any bundled
			// relation; an unmapped property keeps the user's own name, a
			// wrong mapping renames their field
			for containerId, propertyIds := range f.Expect.NotBundled {
				container := plan.Containers[containerId]
				for _, propertyId := range propertyIds {
					entry, planned := container.Properties[propertyId]
					if !planned {
						continue // unplanned imports as-is: trivially not bundled
					}
					assert.False(t, bundle.HasRelation(entry.Key),
						"TRAP TAKEN: %s.%s must not be redirected onto %s", containerId, propertyId, entry.Key)
				}
			}
		})
	}
}

func TestFixtureSuiteGuardArithmetic(t *testing.T) {
	fixtures, err := planfixture.All()
	require.NoError(t, err)

	t.Run("every sameKind group clears the coverage gate", func(t *testing.T) {
		for _, f := range fixtures {
			// given
			fx := newFixture(t, f.Schemas())
			kinds := suiteKinds(f)

			// when
			plan := fx.complete(kinds)

			// then — the gate did not split the group: one shared type key
			for _, group := range f.Expect.SameKind {
				first := plan.Containers[group[0]].TypeKey
				for _, containerId := range group[1:] {
					assert.Equal(t, first, plan.Containers[containerId].TypeKey,
						"%s: sameKind group %v must survive the gate as one kind", f.Id, group)
				}
			}
		}
	})

	t.Run("every separateRelation pair is justified by options or format", func(t *testing.T) {
		for _, f := range fixtures {
			properties := map[string]schemaplan.PropertySchema{}
			for _, container := range f.Containers {
				for _, prop := range container.Properties {
					properties[container.Id+":"+prop.Id] = prop
				}
			}
			for _, group := range f.Expect.SeparateRelation {
				for i := 0; i < len(group); i++ {
					for j := i + 1; j < len(group); j++ {
						left, ok := properties[group[i]]
						require.True(t, ok, "%s: unknown property %s", f.Id, group[i])
						right, ok := properties[group[j]]
						require.True(t, ok, "%s: unknown property %s", f.Id, group[j])
						if left.Format != right.Format {
							continue // structurally separate: format is part of the key
						}
						overlap := optionOverlap(left.Options, right.Options)
						smaller := len(left.Options)
						if len(right.Options) < smaller {
							smaller = len(right.Options)
						}
						assert.Less(t, 2*overlap, smaller,
							"%s: %s vs %s would share under the option guard", f.Id, group[i], group[j])
					}
				}
			}
		}
	})
}

func optionOverlap(a, b []string) int {
	set := make(map[string]bool, len(a))
	for _, option := range a {
		set[option] = true
	}
	overlap := 0
	for _, option := range b {
		if set[option] {
			overlap++
		}
	}
	return overlap
}
