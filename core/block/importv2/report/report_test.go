package report

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func blockById(t *testing.T, blocks []*model.Block, id string) *model.Block {
	t.Helper()
	for _, b := range blocks {
		if b.Id == id {
			return b
		}
	}
	t.Fatalf("block %q not found", id)
	return nil
}

func textOf(t *testing.T, blocks []*model.Block, id string) string {
	t.Helper()
	return blockById(t, blocks, id).GetText().GetText()
}

// render walks the page as a user reads it, so a test can assert on the whole
// document instead of on block ids.
func render(object *importv2.Object) string {
	byId := map[string]*model.Block{}
	for _, b := range object.Payload.Blocks {
		byId[b.Id] = b
	}
	seen := map[string]bool{}
	var out strings.Builder
	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		b := byId[id]
		if b == nil || seen[id] {
			return
		}
		seen[id] = true
		if text := b.GetText(); text != nil {
			out.WriteString(strings.Repeat("  ", depth) + text.Text + "\n")
		}
		for _, child := range b.ChildrenIds {
			walk(child, depth+1)
		}
	}
	for _, b := range object.Payload.Blocks {
		walk(b.Id, 0)
	}
	return out.String()
}

// named resolves every key to itself with a display name, as a run does for
// objects it created.
func named(names map[string]string) Lookup {
	return func(key string) Source {
		name, ok := names[key]
		return Source{Name: name, Resolved: ok}
	}
}

var unknown = func(string) Source { return Source{} }

func TestBuild(t *testing.T) {
	t.Run("one line per kind, not per occurrence", func(t *testing.T) {
		// given — the shape a real import produces: the same sentence, over
		// and over, about different blocks of a few pages
		var issues []importv2.Issue
		for i := 0; i < 40; i++ {
			issues = append(issues, importv2.Warning(importv2.IssueUnsupportedBlock, "page-a",
				"Notion did not return the contents of this block").About("button"))
		}
		for i := 0; i < 5; i++ {
			issues = append(issues, importv2.Warning(importv2.IssueUnsupportedBlock, "page-b",
				"Notion did not return the contents of this block").About("button"))
		}

		// when
		object := Build("t", issues, 0, named(map[string]string{"page-a": "Sprint Planner", "page-b": "Meeting notes"}))

		// then — one group, two lines, biggest first, names not keys
		text := render(object)
		assert.Contains(t, text, "Notion did not return the contents of this block")
		assert.Contains(t, text, "Sprint Planner (40)")
		assert.Contains(t, text, "Meeting notes (5)")
		assert.NotContains(t, text, "page-a", "source keys are ids; the report shows names")
		assert.Less(t, strings.Index(text, "Sprint Planner"), strings.Index(text, "Meeting notes"),
			"the worst-affected object comes first")
	})

	t.Run("a tallied issue counts as many", func(t *testing.T) {
		// given — a converter that aggregated 12 blocks into one issue
		issues := []importv2.Issue{
			importv2.Warning(importv2.IssueUnsupportedBlock, "page-a", "Notion did not return the contents of this block").About("button").Times(12),
			importv2.Warning(importv2.IssueUnsupportedBlock, "page-b", "Notion did not return the contents of this block").About("button").Times(3),
		}

		// when
		object := Build("t", issues, 0, named(map[string]string{"page-a": "Sprint Planner", "page-b": "Meeting notes"}))

		// then
		text := render(object)
		assert.Contains(t, text, "Sprint Planner (12)")
		assert.Contains(t, text, "15", "the summary counts occurrences, not ledger rows")
	})

	t.Run("one subject for the whole group is said once, in the heading", func(t *testing.T) {
		// given — every one of these is about a button; repeating "— button"
		// down 55 lines says nothing new
		issues := []importv2.Issue{
			importv2.Warning(importv2.IssueUnsupportedBlock, "a", "Notion withheld these blocks").About("button"),
			importv2.Warning(importv2.IssueUnsupportedBlock, "b", "Notion withheld these blocks").About("button"),
		}

		// when
		object := Build("t", issues, 0, named(map[string]string{"a": "One", "b": "Two"}))

		// then
		assert.Equal(t, "Notion withheld these blocks (button) — 2", textOf(t, object.Payload.Blocks, "group0"))
		assert.Equal(t, "One", textOf(t, object.Payload.Blocks, "group0-0"))
		assert.Equal(t, "Notion withheld these blocks (button)", textOf(t, object.Payload.Blocks, "sumr1-sumc1"))
	})

	t.Run("a run-wide note counts no objects", func(t *testing.T) {
		// when
		object := Build("t", []importv2.Issue{importv2.Info(importv2.IssueFlavourDetected, "Obsidian vault detected")}, 0, unknown)

		// then
		assert.Equal(t, "—", textOf(t, object.Payload.Blocks, "sumr1-sumc3"))
	})

	t.Run("subjects are listed on the object's line", func(t *testing.T) {
		// given
		issues := []importv2.Issue{
			importv2.Warning(importv2.IssueDataLoss, "db", "Notion buttons cannot be imported").About("Set To Do"),
			importv2.Warning(importv2.IssueDataLoss, "db", "Notion buttons cannot be imported").About("Task Complete"),
		}

		// when
		object := Build("t", issues, 0, named(map[string]string{"db": "Tasks"}))

		// then
		assert.Contains(t, render(object), "Tasks (2) — Set To Do, Task Complete")
	})

	t.Run("many subjects are truncated, many objects are truncated", func(t *testing.T) {
		// given
		var issues []importv2.Issue
		names := map[string]string{}
		for i := 0; i < 40; i++ {
			key := fmt.Sprintf("k%02d", i)
			names[key] = fmt.Sprintf("Page %02d", i)
			issues = append(issues, importv2.Warning(importv2.IssueDataLoss, key, "same thing").About(fmt.Sprintf("s%02d", i)))
		}
		for i := 0; i < 9; i++ {
			issues = append(issues, importv2.Warning(importv2.IssueDataLoss, "k00", "same thing").About(fmt.Sprintf("extra%d", i)))
		}

		// when
		object := Build("t", issues, 0, named(names))

		// then
		text := render(object)
		assert.Contains(t, text, "and 15 more")
		assert.Contains(t, text, "+6 more", "a long subject list is cut short")
	})

	t.Run("objects that cannot be named collapse into one line", func(t *testing.T) {
		// given — 5 rows the import skipped: their keys resolve to nothing,
		// so a line each would be five Notion ids nobody can use
		var issues []importv2.Issue
		for i := 0; i < 5; i++ {
			issues = append(issues, importv2.Warning(importv2.IssueDataLoss, fmt.Sprintf("row%d", i), "empty rows were skipped").About("Contacts"))
		}
		issues = append(issues, importv2.Warning(importv2.IssueDataLoss, "known", "empty rows were skipped").About("Tasks"))

		// when
		object := Build("t", issues, 0, named(map[string]string{"known": "Tasks database"}))

		// then — the one that can be named is named; the rest are counted
		text := render(object)
		assert.Contains(t, text, "Tasks database")
		assert.Contains(t, text, "5 objects — Contacts")
		assert.NotContains(t, text, "row0")
	})

	t.Run("a few unnamed objects are still listed individually", func(t *testing.T) {
		// given — with two of them the key is the only handle there is
		issues := []importv2.Issue{
			importv2.Warning(importv2.IssueDataLoss, "a", "lost"),
			importv2.Warning(importv2.IssueDataLoss, "b", "lost"),
		}

		// when
		text := render(Build("t", issues, 0, unknown))

		// then
		assert.Contains(t, text, "a\n")
		assert.Contains(t, text, "b\n")
	})

	t.Run("groups sort by severity, then by how much they affected", func(t *testing.T) {
		// given
		issues := []importv2.Issue{
			importv2.Warning(importv2.IssueDataLoss, "a", "small warning"),
			importv2.Warning(importv2.IssueDataLoss, "b", "big warning").Times(50),
			importv2.ObjectError(importv2.IssueObjectFailed, "c", errors.New("boom")),
			importv2.Info(importv2.IssueTypeSuggested, "a note"),
		}

		// when
		object := Build("t", issues, 0, unknown)

		// then
		text := render(object)
		order := []string{"boom", "big warning", "small warning", "a note"}
		last := -1
		for _, want := range order {
			at := strings.Index(text, want)
			require.Greater(t, at, last, "%q out of order in:\n%s", want, text)
			last = at
		}
	})

	t.Run("summary table has a row per group with counts and object tallies", func(t *testing.T) {
		// given
		issues := []importv2.Issue{
			importv2.Warning(importv2.IssueDataLoss, "a", "lost").Times(3),
			importv2.Warning(importv2.IssueDataLoss, "b", "lost"),
		}

		// when
		object := Build("t", issues, 0, unknown)

		// then
		blocks := object.Payload.Blocks
		table := blockById(t, blocks, "summary")
		require.Equal(t, []string{"summary-columns", "summary-rows"}, table.ChildrenIds)
		rows := blockById(t, blocks, "summary-rows")
		require.Equal(t, []string{"sumr0", "sumr1"}, rows.ChildrenIds)
		assert.True(t, blockById(t, blocks, "sumr0").GetTableRow().GetIsHeader())
		assert.Equal(t, "Imported with changes", textOf(t, blocks, "sumr1-sumc0"))
		assert.Equal(t, "lost", textOf(t, blocks, "sumr1-sumc1"))
		assert.Equal(t, "4", textOf(t, blocks, "sumr1-sumc2"))
		assert.Equal(t, "2", textOf(t, blocks, "sumr1-sumc3"))
	})

	t.Run("a resolvable object is mention-linked by name", func(t *testing.T) {
		// given
		issues := []importv2.Issue{importv2.Warning(importv2.IssueDataLoss, "страница.md", "lost")}

		// when
		object := Build("t", issues, 0, named(map[string]string{"страница.md": "Заметка"}))

		// then — the mark spans the NAME, and its param stays the source key
		// because the persist-side resolver rewrites params, not text
		line := blockById(t, object.Payload.Blocks, "group0-0")
		marks := line.GetText().GetMarks().GetMarks()
		require.Len(t, marks, 1)
		assert.Equal(t, model.BlockContentTextMark_Mention, marks[0].Type)
		assert.Equal(t, "страница.md", marks[0].Param)
		assert.Equal(t, int32(0), marks[0].Range.From)
		assert.Equal(t, int32(7), marks[0].Range.To, "UTF-16 length of the display name")
	})

	t.Run("an unresolvable key stays plain text and falls back to the key", func(t *testing.T) {
		// when
		object := Build("t", []importv2.Issue{importv2.Warning(importv2.IssueDataLoss, "gone.md", "lost")}, 0, unknown)

		// then
		line := blockById(t, object.Payload.Blocks, "group0-0")
		assert.Empty(t, line.GetText().GetMarks().GetMarks())
		assert.Contains(t, line.GetText().GetText(), "gone.md")
	})

	t.Run("an issue about the whole run has no object line", func(t *testing.T) {
		// given — info issues carry no source key
		issues := []importv2.Issue{importv2.Info(importv2.IssueTypeSuggested, "database X imported as Task")}

		// when
		object := Build("t", issues, 0, unknown)

		// then
		assert.Contains(t, render(object), "database X imported as Task")
		assert.Empty(t, blockById(t, object.Payload.Blocks, "group0").ChildrenIds,
			"a run-wide note has no object to list")
	})

	t.Run("notes are counted apart from problems", func(t *testing.T) {
		// given — "these rows became Tasks" is not damage
		issues := []importv2.Issue{
			importv2.Warning(importv2.IssueDataLoss, "a", "lost something"),
			importv2.Info(importv2.IssueTypeSuggested, "rows became tasks"),
			importv2.Info(importv2.IssueTypeSuggested, "rows became notes"),
		}

		// when
		text := render(Build("t", issues, 0, unknown))

		// then
		assert.Contains(t, text, "1 thing in 1 object did not come over exactly")
		assert.Contains(t, text, "2 lines below are notes")
	})

	t.Run("a subject that repeats the object name is not said twice", func(t *testing.T) {
		// given — a database's note about itself carries its own title
		issues := []importv2.Issue{
			importv2.Info(importv2.IssueTypeSuggested, "rows became tasks").About("Launch Tracker"),
			importv2.Info(importv2.IssueTypeSuggested, "rows became tasks").About("Sprints"),
		}
		issues[0].SourceKey, issues[1].SourceKey = "db1", "db2"

		// when
		text := render(Build("t", issues, 0, named(map[string]string{"db1": "Launch Tracker", "db2": "Sprint board"})))

		// then
		assert.NotContains(t, text, "Launch Tracker — Launch Tracker")
		assert.Contains(t, text, "Launch Tracker\n")
		assert.Contains(t, text, "Sprint board — Sprints", "a subject that adds something is kept")
	})

	t.Run("overflow renders a table row and a trailing line", func(t *testing.T) {
		// when
		object := Build("t", []importv2.Issue{importv2.Warning(importv2.IssueDataLoss, "a.md", "x")}, 7, unknown)

		// then
		blocks := object.Payload.Blocks
		assert.Contains(t, textOf(t, blocks, "overflow"), "7 further issues")
		rows := blockById(t, blocks, "summary-rows")
		assert.Equal(t, []string{"sumr0", "sumr1", "sumr2"}, rows.ChildrenIds)
		assert.Equal(t, "7", textOf(t, blocks, "sumr2-sumc2"))
	})

	t.Run("details carry name and icon, type is page", func(t *testing.T) {
		// when
		object := Build("Import report — Notion Import", []importv2.Issue{importv2.Warning(importv2.IssueDataLoss, "a", "x")}, 0, unknown)

		// then
		assert.Equal(t, SourceKey, object.SourceKey)
		assert.Equal(t, "Import report — Notion Import", object.Payload.Details.GetString(bundle.RelationKeyName))
		assert.NotEmpty(t, object.Payload.Details.GetString(bundle.RelationKeyIconEmoji))
		assert.Equal(t, []string{bundle.TypeKeyPage.String()}, object.Payload.ObjectTypes)
	})

	t.Run("output is deterministic", func(t *testing.T) {
		// given — map iteration must not leak into the page
		issues := []importv2.Issue{
			importv2.Warning(importv2.IssueDataLoss, "b", "same").About("x"),
			importv2.Warning(importv2.IssueDataLoss, "a", "same").About("y"),
			importv2.Warning(importv2.IssueMissingTarget, "c", "other"),
		}

		// when
		first := render(Build("t", issues, 0, unknown))
		for i := 0; i < 20; i++ {
			assert.Equal(t, first, render(Build("t", issues, 0, unknown)))
		}
	})
}
