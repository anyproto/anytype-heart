package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

func docFrom(t *testing.T, body string) *document {
	t.Helper()
	var doc document
	require.NoError(t, json.Unmarshal([]byte(body), &doc))
	return &doc
}

func taskById(t *testing.T, id string) task {
	t.Helper()
	for _, task := range tasks() {
		if task.Id == id {
			return task
		}
	}
	t.Fatalf("no task %q", id)
	return task{}
}

// A check that cannot fail proves nothing, so every task is exercised on a
// document that satisfies it AND on documents that do not — including the
// near misses a model actually produces (the edit applied to the wrong
// block, the content added as plain text, the whole section rewritten).
func TestTaskChecks(t *testing.T) {
	tests := []struct {
		task string
		name string
		doc  string
		want bool
	}{
		{
			task: "edit-one-word", name: "the word is swapped and nothing else moved", want: true,
			doc: `{"blocks":[{"id":"a","type":"heading_2","text":"Summary"},
				{"id":"b","type":"paragraph","text":"Revenue target for Q4 is 1.2M."},
				{"id":"c","type":"heading_2","text":"Owner"},
				{"id":"d","type":"paragraph","text":"The finance team reviews this monthly."}]}`,
		},
		{
			task: "edit-one-word", name: "untouched", want: false,
			doc: `{"blocks":[{"id":"b","type":"paragraph","text":"Revenue target for Q3 is 1.2M."},
				{"id":"d","type":"paragraph","text":"The finance team reviews this monthly."}]}`,
		},
		{
			task: "edit-one-word", name: "the block was retyped and lost its tail", want: false,
			doc: `{"blocks":[{"id":"b","type":"paragraph","text":"Revenue target for Q4"},
				{"id":"d","type":"paragraph","text":"The finance team reviews this monthly."}]}`,
		},
		{
			task: "edit-one-word", name: "Q4 added, Q3 left behind", want: false,
			doc: `{"blocks":[{"id":"b","type":"paragraph","text":"Revenue target for Q4 is 1.2M."},
				{"id":"x","type":"paragraph","text":"Was: Revenue target for Q3 is 1.2M."},
				{"id":"d","type":"paragraph","text":"The finance team reviews this monthly."}]}`,
		},
		{
			task: "append-section", name: "heading plus two bullets after it", want: true,
			doc: `{"blocks":[{"id":"a","type":"heading_2","text":"Overview"},
				{"id":"b","type":"paragraph","text":"The migration runs in three stages."},
				{"id":"c","type":"heading_2","text":"Risks"},
				{"id":"d","type":"bulleted_list_item","text":"Vendor delay"},
				{"id":"e","type":"bulleted_list_item","text":"Budget overrun"}]}`,
		},
		{
			task: "append-section", name: "added as paragraphs, not a heading and bullets", want: false,
			doc: `{"blocks":[{"id":"a","type":"heading_2","text":"Overview"},
				{"id":"b","type":"paragraph","text":"The migration runs in three stages."},
				{"id":"c","type":"paragraph","text":"Risks"},
				{"id":"d","type":"paragraph","text":"- Vendor delay"},
				{"id":"e","type":"paragraph","text":"- Budget overrun"}]}`,
		},
		{
			task: "append-section", name: "one bullet missing", want: false,
			doc: `{"blocks":[{"id":"c","type":"heading_2","text":"Risks"},
				{"id":"d","type":"bulleted_list_item","text":"Vendor delay"},
				{"id":"b","type":"paragraph","text":"The migration runs in three stages."}]}`,
		},
		{
			task: "append-section", name: "the section replaced the document", want: false,
			doc: `{"blocks":[{"id":"c","type":"heading_2","text":"Risks"},
				{"id":"d","type":"bulleted_list_item","text":"Vendor delay"},
				{"id":"e","type":"bulleted_list_item","text":"Budget overrun"}]}`,
		},
		{
			task: "fill-table-cell", name: "one cell changed", want: true,
			doc: `{"blocks":[{"id":"t","type":"table","columns":[{"id":"c1"},{"id":"c2"}],"rows":[
				{"id":"r1","is_header":true,"cells":["Component","Status"]},
				{"id":"r2","cells":["Alpha","Done"]},
				{"id":"r3","cells":["Beta","Done"]},
				{"id":"r4","cells":["Gamma","Pending"]}]}]}`,
		},
		{
			task: "fill-table-cell", name: "the wrong row was filled", want: false,
			doc: `{"blocks":[{"id":"t","type":"table","columns":[{"id":"c1"},{"id":"c2"}],"rows":[
				{"id":"r1","is_header":true,"cells":["Component","Status"]},
				{"id":"r2","cells":["Alpha","Done"]},
				{"id":"r3","cells":["Beta","Pending"]},
				{"id":"r4","cells":["Gamma","Done"]}]}]}`,
		},
		{
			task: "fill-table-cell", name: "the table was rewritten as text", want: false,
			doc: `{"blocks":[{"id":"p","type":"paragraph","text":"Alpha Done, Beta Done, Gamma Pending"}]}`,
		},
		{
			task: "fill-table-cell", name: "a cell block object satisfies the check too", want: true,
			doc: `{"blocks":[{"id":"t","type":"table","columns":[{"id":"c1"},{"id":"c2"}],"rows":[
				{"id":"r1","is_header":true,"cells":["Component","Status"]},
				{"id":"r2","cells":["Alpha","Done"]},
				{"id":"r3","cells":["Beta",{"type":"paragraph","text":"Done"}]},
				{"id":"r4","cells":["Gamma","Pending"]}]}]}`,
		},
		{
			task: "restructure-section", name: "bullets gone, paragraph in, heading kept", want: true,
			doc: `{"blocks":[{"id":"h","type":"heading_2","text":"Next steps"},
				{"id":"p","type":"paragraph","text":"Deferred to Q4."}]}`,
		},
		{
			task: "restructure-section", name: "paragraph added but the bullets survive", want: false,
			doc: `{"blocks":[{"id":"h","type":"heading_2","text":"Next steps"},
				{"id":"b1","type":"bulleted_list_item","text":"Ship the beta"},
				{"id":"p","type":"paragraph","text":"Deferred to Q4."}]}`,
		},
		{
			task: "restructure-section", name: "the heading went with the bullets", want: false,
			doc: `{"blocks":[{"id":"p","type":"paragraph","text":"Deferred to Q4."}]}`,
		},
		{
			task: "set-property", name: "description set exactly", want: true,
			doc: `{"properties":{"name":"Vendor review ab12","description":"Reviewed by the ops team."},"blocks":[]}`,
		},
		{
			task: "set-property", name: "written into the body instead of the property", want: false,
			doc: `{"properties":{"name":"Vendor review ab12"},
				"blocks":[{"id":"p","type":"paragraph","text":"Reviewed by the ops team."}]}`,
		},
		{
			task: "set-property", name: "close but not exact", want: false,
			doc: `{"properties":{"description":"reviewed by ops"},"blocks":[]}`,
		},
		{
			task: "read-then-edit", name: "owner line rewritten, the rest intact", want: true,
			doc: `{"blocks":[{"id":"h","type":"heading_2","text":"Meeting notes"},
				{"id":"o","type":"paragraph","text":"Owner: Dana Whitfield"},
				{"id":"r","type":"paragraph","text":"Next review: 12 May"}]}`,
		},
		{
			// the shape the SERVER actually produces from the fixture markdown:
			// two lines with no blank line between them are one paragraph with
			// a soft break. The first check compared whole block texts and
			// failed this document, which a model had edited exactly right.
			task: "read-then-edit", name: "both lines in one block, as the importer makes it", want: true,
			doc: `{"blocks":[{"id":"h","type":"heading_2","text":"Meeting notes"},
				{"id":"o","type":"paragraph","text":"Owner: Dana Whitfield\nNext review: 12 May"}]}`,
		},
		{
			task: "read-then-edit", name: "one block, old owner still named", want: false,
			doc: `{"blocks":[{"id":"o","type":"paragraph","text":"Owner: Priya Raman\nNext review: 12 May"}]}`,
		},
		{
			task: "read-then-edit", name: "the owner line was rewritten past recognition", want: false,
			doc: `{"blocks":[{"id":"o","type":"paragraph","text":"The owner is now Dana Whitfield\nNext review: 12 May"}]}`,
		},
		{
			task: "read-then-edit", name: "new owner appended, old one left", want: false,
			doc: `{"blocks":[{"id":"o","type":"paragraph","text":"Owner: Priya Raman"},
				{"id":"n","type":"paragraph","text":"Owner: Dana Whitfield"},
				{"id":"r","type":"paragraph","text":"Next review: 12 May"}]}`,
		},
		{
			task: "read-then-edit", name: "the review line was collateral damage", want: false,
			doc: `{"blocks":[{"id":"o","type":"paragraph","text":"Owner: Dana Whitfield"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.task+": "+tt.name, func(t *testing.T) {
			// given
			task := taskById(t, tt.task)
			doc := docFrom(t, tt.doc)

			// when
			got := task.Check(doc, &fixture{Title: "fixture", ObjectId: "obj1"})

			// then
			assert.Equal(t, tt.want, got.OK, "detail: %s", got.Detail)
			if !tt.want {
				assert.NotEmpty(t, got.Detail, "a failing check must say what it saw")
			}
		})
	}
}

func TestTaskTableIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, task := range tasks() {
		t.Run(task.Id, func(t *testing.T) {
			assert.False(t, seen[task.Id], "duplicate task id")
			seen[task.Id] = true
			assert.NotEmpty(t, task.Intent)
			// a page's fixture body is its markdown; a collection's is the
			// dataview it is created with, and giving it markdown too would
			// seed content the task never looks at
			if task.Type == "collection" {
				assert.Empty(t, task.Markdown, "a collection's fixture is its dataview, not a markdown body")
			} else {
				assert.NotEmpty(t, task.Markdown, "every page task needs a fixture body")
			}
			assert.NotEmpty(t, task.Requires, "a task with no declared capability is gated by nothing")
			require.NotNil(t, task.Prompt)
			// exactly one result channel: the fixture document, or the live
			// API for a task whose product is not in that document
			if task.CheckAPI != nil {
				assert.Nil(t, task.Check, "a task reads its result back one way, not two")
			} else {
				require.NotNil(t, task.Check)
			}

			fx := newFixtureFor(fixtureTitle())
			fx.ObjectId = "obj1"
			prompt := task.Prompt(fx)
			assert.Contains(t, prompt, fx.Title, "the prompt must name the object the model has to find")
			assert.NotContains(t, prompt, "insert_blocks", "a prompt must not name the tool to use")
			assert.NotContains(t, prompt, "edit_text", "a prompt must not name the tool to use")

			// the fixture body must not already satisfy the check, or the task
			// would pass without the model doing anything. A CheckAPI task
			// reads the live API, so there is no offline document to test it
			// against — its "already satisfied" guard is that the thing it
			// looks for (a minted, per-attempt type name) cannot pre-exist.
			if task.Check != nil {
				doc := docFromMarkdownApproximation(task.Markdown)
				assert.False(t, task.Check(doc, fx).OK, "the fixture already satisfies the check")
			}
		})
	}
}

// docFromMarkdownApproximation renders a fixture's markdown as the document
// it roughly becomes — enough to assert that the fixture does not already
// satisfy its own check. The real server parses the markdown; this only has
// to be faithful enough to fail.
func docFromMarkdownApproximation(markdown string) *document {
	doc := &document{Properties: map[string]any{}}
	inTable := false
	table := docBlock{Id: "t", Type: "table"}
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case strings.HasPrefix(trimmed, "|"):
			cells := strings.Split(strings.Trim(trimmed, "|"), "|")
			if strings.Contains(trimmed, "---") {
				continue
			}
			row := docTableRow{Id: fmt.Sprintf("r%d", len(table.Rows)), IsHeader: !inTable}
			for _, cell := range cells {
				encoded, _ := json.Marshal(strings.TrimSpace(cell))
				row.Cells = append(row.Cells, encoded)
			}
			table.Rows = append(table.Rows, row)
			inTable = true
		case strings.HasPrefix(trimmed, "## "):
			doc.Blocks = append(doc.Blocks, docBlock{Id: fmt.Sprintf("b%d", len(doc.Blocks)), Type: "heading_2", Text: strings.TrimPrefix(trimmed, "## ")})
		case strings.HasPrefix(trimmed, "- "):
			doc.Blocks = append(doc.Blocks, docBlock{Id: fmt.Sprintf("b%d", len(doc.Blocks)), Type: "bulleted_list_item", Text: strings.TrimPrefix(trimmed, "- ")})
		default:
			doc.Blocks = append(doc.Blocks, docBlock{Id: fmt.Sprintf("b%d", len(doc.Blocks)), Type: "paragraph", Text: trimmed})
		}
	}
	if len(table.Rows) > 0 {
		doc.Blocks = append(doc.Blocks, table)
	}
	return doc
}

// The gate is derived from the arm's published tool set, not from a
// hand-kept list of tiers: fill-table-cell ran on a small tier with no
// set_cell for a whole matrix, where the model recognised the limit, said so
// in plain words, and was scored as a failure six times over.
func TestCellsAreSkippedWhenTheArmPublishesNoToolForTheTask(t *testing.T) {
	// given
	require.NoError(t, checkTaskGating())
	arms, err := parseArms(strings.Join(allArms, ","))
	require.NoError(t, err)

	// when
	cells, skipped, err := planCells([]string{"m"}, arms, tasks())
	require.NoError(t, err)

	reasons := map[cellKey]string{}
	for _, s := range skipped {
		reasons[cellKey{s.Model, s.Arm, s.Task}] = s.Reason
	}

	// then — the two tasks the small tier has no tool for are skipped there
	// and run everywhere else
	assert.Contains(t, reasons[cellKey{"m", armWrapperSmall, "fill-table-cell"}], "publishes no set_cell",
		"the small tier has no set_cell — the cell is not a measurement")
	assert.Contains(t, reasons[cellKey{"m", armWrapperSmall, "restructure-section"}], "publishes no delete_block")
	assert.True(t, cells[cellKey{"m", armWrapperSmall, "edit-one-word"}], "edit_text is served to the small tier")
	assert.True(t, cells[cellKey{"m", armWrapperLarge, "fill-table-cell"}])
	assert.True(t, cells[cellKey{"m", armOps, "fill-table-cell"}], "the ops arm publishes set_cell")
	assert.True(t, cells[cellKey{"m", armOps, "restructure-section"}])

	// and the gate reads the tier table rather than restating it
	assert.NotContains(t, wrapper.ToolNamesForTier(wrapper.TierSmall), "set_cell")
	assert.NotContains(t, wrapper.ToolNamesForTier(wrapper.TierSmall), "delete_block")
	assert.Contains(t, wrapper.ToolNamesForTier(wrapper.TierLarge), "set_cell")
}

func TestEveryArmPublishesEveryToolItsCapabilitiesName(t *testing.T) {
	// given — a capability that names no tool on a surface would skip cells
	// silently, which reads exactly like a cell nobody wanted measured
	require.NoError(t, checkTaskGating())

	// then
	for _, arm := range []armSpec{
		{name: armWrapperLarge, surface: surfaceWrapper, tier: wrapper.TierLarge},
		{name: armOps, surface: surfaceOps},
	} {
		published := arm.publishedTools()
		for c := range capabilityTools {
			if surfaceCannotExpress(c, arm.surface) {
				// a genuine gap, not an unmapped capability: the ops arm
				// serves PATCH ops, and type creation is a route
				_, err := capabilityTool(c, arm.surface)
				assert.Error(t, err, "%s: an inexpressible capability must not be mapped anyway", c)
				continue
			}
			tool, err := capabilityTool(c, arm.surface)
			require.NoError(t, err)
			assert.Contains(t, published, tool, "%s should publish %s", arm.name, tool)
		}
	}
}

func TestFixtureTitlesShareNoTokenAndNoPrefix(t *testing.T) {
	// given — the API's search matches token-wise and prefix-matches the
	// query, and fixtures can never be deleted, so a shared stem made find
	// return one more object on every attempt of a run
	seen := map[string]bool{}

	for i := 0; i < 500; i++ {
		// when
		title := fixtureTitle()

		// then
		assert.NotContains(t, title, " ", "a title must be ONE search token")
		assert.Len(t, title, titleSyllables*2, "fixed length: only an equal name can be a prefix of another")
		assert.False(t, seen[title], "collision after %d titles", i)
		seen[title] = true
	}
}

func TestArmParsing(t *testing.T) {
	// when
	arms, err := parseArms("wrapper/small,ops,wrapper/large")

	// then
	require.NoError(t, err)
	require.Len(t, arms, 3)
	assert.Equal(t, wrapper.TierSmall, arms[0].tier)
	assert.Equal(t, surfaceOps, arms[1].surface)
	assert.Equal(t, wrapper.TierLarge, arms[2].tier)

	_, err = parseArms("wrapper/medium")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrapper/small")
	assert.Contains(t, err.Error(), "wrapper/large")
}

// TestEveryTaskHasExactlyOneCheck guards the split between Check (grades the
// fixture document) and CheckAPI (grades something the API holds instead).
// A task with neither panicked the whole run on its first live attempt —
// after the model had done the work — because runAttempt called Check
// unconditionally. A task with both would silently grade twice and keep only
// the second verdict.
func TestEveryTaskHasExactlyOneCheck(t *testing.T) {
	for _, task := range tasks() {
		hasDoc, hasAPI := task.Check != nil, task.CheckAPI != nil
		assert.Truef(t, hasDoc != hasAPI,
			"task %s must set exactly one of Check / CheckAPI (Check=%v CheckAPI=%v)",
			task.Id, hasDoc, hasAPI)
	}
}
