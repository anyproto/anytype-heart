package schemaplan

import (
	"testing"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeTypeNames(t *testing.T) {
	// A type carries both names: "Project" for one of them, "Projects" for a
	// list. A model asked for both often answers one — the same word twice, or
	// the plural in the singular field and nothing in the other — and the
	// container-name fallback (a Notion database is called "AI Projects")
	// never had a plural at all.
	cases := []struct {
		name           string
		inName, inPlur string
		wantName       string
		wantPlural     string
	}{
		{"both given, both kept", "Task", "Tasks", "Task", "Tasks"},
		{"a plural in the singular field, nothing in the other", "Projects", "", "Project", "Projects"},
		{"the same word twice", "Projects", "Projects", "Project", "Projects"},
		{"the same singular word twice", "Project", "Project", "Project", "Projects"},
		{"a singular with no plural", "Recipe", "", "Recipe", "Recipes"},
		{"only the last word of a phrase inflects", "AI Projects", "", "AI Project", "AI Projects"},
		{"team member", "Team Member", "", "Team Member", "Team Members"},
		{"y becomes ies", "Company", "", "Company", "Companies"},
		{"ies becomes y", "Companies", "", "Company", "Companies"},
		{"sibilants take es", "Class", "", "Class", "Classes"},
		{"and give it back", "Classes", "", "Class", "Classes"},
		{"an irregular is not guessed at", "Person", "", "Person", "People"},
		{"and back", "People", "", "Person", "People"},
		{"a word that only looks plural is left alone", "Analysis", "", "Analysis", "Analyses"},
		{"as is one that has no plural", "Research", "", "Research", "Research"},
		{"and one that is its own plural", "Series", "", "Series", "Series"},
		{"a plural ending in as is one", "Ideas", "", "Idea", "Ideas"},
		{"life areas, the Notion database", "Life Areas", "", "Life Area", "Life Areas"},
		{"a plural ending in os is one", "Photos", "", "Photo", "Photos"},
		{"a singular ending in as is not", "Canvas", "", "Canvas", "Canvases"},
		{"nor is one ending in us", "Status", "", "Status", "Statuses"},
		{"both sides of a conjunction inflect", "Tasks & Features", "", "Task & Feature", "Tasks & Features"},
		{"even when one side is already singular", "Notes and Clippings", "", "Note and Clipping", "Notes and Clippings"},
		{"nothing at all stays nothing", "", "", "", ""},
		// Every name below is one gpt-5.6-luna returned for the recorded
		// workspace: the model echoes the Notion database title, tag and all.
		{"a trailing tag is not the head noun", "Reminders (SB)", "", "Reminder (SB)", "Reminders (SB)"},
		{"nor when the head is an initialism", "CRM (SB)", "", "CRM (SB)", "CRMs (SB)"},
		{"nor when the head is a phrase", "All notes (SB)", "", "All note (SB)", "All notes (SB)"},
		{"a head that is not a word does not inflect", "Q1 2024", "", "Q1 2024", "Q1 2024"},
		{"nor does a word ending in punctuation", "Stay Organized, Connected, and In Control", "",
			"Stay Organized, Connected, and In Control", "Stay Organized, Connected, and In Controls"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// when
			gotName, gotPlural := normalizeTypeNames(c.inName, c.inPlur)

			// then
			assert.Equal(t, c.wantName, gotName, "singular")
			assert.Equal(t, c.wantPlural, gotPlural, "plural")
		})
	}
}

func TestSanitizeGivesEveryTypeBothNames(t *testing.T) {
	sanitize := func(def TypeDefinition) TypeDefinition {
		plan := Plan{NewTypes: []TypeDefinition{def}, Containers: map[string]ContainerPlan{"ds1": {TypeKey: def.Key}}}
		var issues []importv2.Issue
		got := Sanitize(plan, taskSchemas(), collectIssues(&issues))
		if len(got.NewTypes) == 0 {
			t.Fatalf("type dropped: %v", issues)
		}
		return got.NewTypes[0]
	}

	t.Run("a plural answer in the singular field is put right", func(t *testing.T) {
		// given — what a model returns when it answers one question twice
		got := sanitize(TypeDefinition{Key: "k", Name: "Projects"})

		// then
		assert.Equal(t, "Project", got.Name)
		assert.Equal(t, "Projects", got.PluralName)
	})

	t.Run("a name echoed from the database title gets a plural too", func(t *testing.T) {
		// given — what the model does when it cannot think of a name: it
		// hands back the Notion database's own title, and those are plural
		got := sanitize(TypeDefinition{Key: "k", Name: "AI Projects"})

		// then
		assert.Equal(t, "AI Project", got.Name)
		assert.Equal(t, "AI Projects", got.PluralName)
	})

	t.Run("a name cut to the length cap keeps its plural whole", func(t *testing.T) {
		// given — an over-long name falls back to the container's own title,
		// cut at the cap, and that cut can land mid-word ("…In Contro").
		// Inflecting half a word invents one, so a cut name is its own plural.
		long := "Contact List Template - Stay Organized, Connected, and In Control"
		schemas := taskSchemas()
		schemas[0].Name = long
		plan := Plan{
			NewTypes:   []TypeDefinition{{Key: "k", Name: long + " and Then Some"}},
			Containers: map[string]ContainerPlan{"ds1": {TypeKey: "k"}},
		}

		// when
		var issues []importv2.Issue
		got := Sanitize(plan, schemas, collectIssues(&issues))

		// then
		require.Len(t, got.NewTypes, 1, "type dropped: %v", issues)
		assert.Len(t, []rune(got.NewTypes[0].Name), maxNameRunes)
		assert.Equal(t, got.NewTypes[0].Name, got.NewTypes[0].PluralName)
	})

	t.Run("names the model got right are left alone", func(t *testing.T) {
		// when
		got := sanitize(TypeDefinition{Key: "k", Name: "Meeting note", PluralName: "Meeting notes"})

		// then
		assert.Equal(t, "Meeting note", got.Name)
		assert.Equal(t, "Meeting notes", got.PluralName)
	})
}
