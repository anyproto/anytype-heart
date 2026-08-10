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
			doc: `{"blocks":[{"id":"a","type":"heading2","text":"Summary"},
				{"id":"b","type":"paragraph","text":"Revenue target for Q4 is 1.2M."},
				{"id":"c","type":"heading2","text":"Owner"},
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
			doc: `{"blocks":[{"id":"a","type":"heading2","text":"Overview"},
				{"id":"b","type":"paragraph","text":"The migration runs in three stages."},
				{"id":"c","type":"heading2","text":"Risks"},
				{"id":"d","type":"bulletedListItem","text":"Vendor delay"},
				{"id":"e","type":"bulletedListItem","text":"Budget overrun"}]}`,
		},
		{
			task: "append-section", name: "added as paragraphs, not a heading and bullets", want: false,
			doc: `{"blocks":[{"id":"a","type":"heading2","text":"Overview"},
				{"id":"b","type":"paragraph","text":"The migration runs in three stages."},
				{"id":"c","type":"paragraph","text":"Risks"},
				{"id":"d","type":"paragraph","text":"- Vendor delay"},
				{"id":"e","type":"paragraph","text":"- Budget overrun"}]}`,
		},
		{
			task: "append-section", name: "one bullet missing", want: false,
			doc: `{"blocks":[{"id":"c","type":"heading2","text":"Risks"},
				{"id":"d","type":"bulletedListItem","text":"Vendor delay"},
				{"id":"b","type":"paragraph","text":"The migration runs in three stages."}]}`,
		},
		{
			task: "append-section", name: "the section replaced the document", want: false,
			doc: `{"blocks":[{"id":"c","type":"heading2","text":"Risks"},
				{"id":"d","type":"bulletedListItem","text":"Vendor delay"},
				{"id":"e","type":"bulletedListItem","text":"Budget overrun"}]}`,
		},
		{
			task: "fill-table-cell", name: "one cell changed", want: true,
			doc: `{"blocks":[{"id":"t","type":"table","columns":[{"id":"c1"},{"id":"c2"}],"rows":[
				{"id":"r1","isHeader":true,"cells":["Component","Status"]},
				{"id":"r2","cells":["Alpha","Done"]},
				{"id":"r3","cells":["Beta","Done"]},
				{"id":"r4","cells":["Gamma","Pending"]}]}]}`,
		},
		{
			task: "fill-table-cell", name: "the wrong row was filled", want: false,
			doc: `{"blocks":[{"id":"t","type":"table","columns":[{"id":"c1"},{"id":"c2"}],"rows":[
				{"id":"r1","isHeader":true,"cells":["Component","Status"]},
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
				{"id":"r1","isHeader":true,"cells":["Component","Status"]},
				{"id":"r2","cells":["Alpha","Done"]},
				{"id":"r3","cells":["Beta",{"type":"paragraph","text":"Done"}]},
				{"id":"r4","cells":["Gamma","Pending"]}]}]}`,
		},
		{
			task: "restructure-section", name: "bullets gone, paragraph in, heading kept", want: true,
			doc: `{"blocks":[{"id":"h","type":"heading2","text":"Next steps"},
				{"id":"p","type":"paragraph","text":"Deferred to Q4."}]}`,
		},
		{
			task: "restructure-section", name: "paragraph added but the bullets survive", want: false,
			doc: `{"blocks":[{"id":"h","type":"heading2","text":"Next steps"},
				{"id":"b1","type":"bulletedListItem","text":"Ship the beta"},
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
			doc: `{"blocks":[{"id":"h","type":"heading2","text":"Meeting notes"},
				{"id":"o","type":"paragraph","text":"Owner: Dana Whitfield"},
				{"id":"r","type":"paragraph","text":"Next review: 12 May"}]}`,
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
			assert.NotEmpty(t, task.Markdown, "every task needs a fixture body")
			assert.NotEmpty(t, task.TitleStem)
			require.NotNil(t, task.Prompt)
			require.NotNil(t, task.Check)

			fx := &fixture{Title: task.TitleStem + " ab12", ObjectId: "obj1"}
			prompt := task.Prompt(fx)
			assert.Contains(t, prompt, fx.Title, "the prompt must name the object the model has to find")
			assert.NotContains(t, prompt, "insertBlocks", "a prompt must not name the tool to use")
			assert.NotContains(t, prompt, "edit_text", "a prompt must not name the tool to use")

			// the fixture body must not already satisfy the check, or the task
			// would pass without the model doing anything
			doc := docFromMarkdownApproximation(task.Markdown)
			assert.False(t, task.Check(doc, fx).OK, "the fixture already satisfies the check")
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
			doc.Blocks = append(doc.Blocks, docBlock{Id: fmt.Sprintf("b%d", len(doc.Blocks)), Type: "heading2", Text: strings.TrimPrefix(trimmed, "## ")})
		case strings.HasPrefix(trimmed, "- "):
			doc.Blocks = append(doc.Blocks, docBlock{Id: fmt.Sprintf("b%d", len(doc.Blocks)), Type: "bulletedListItem", Text: strings.TrimPrefix(trimmed, "- ")})
		default:
			doc.Blocks = append(doc.Blocks, docBlock{Id: fmt.Sprintf("b%d", len(doc.Blocks)), Type: "paragraph", Text: trimmed})
		}
	}
	if len(table.Rows) > 0 {
		doc.Blocks = append(doc.Blocks, table)
	}
	return doc
}

func TestRestructureIsWithheldFromTheTierThatCannotDoIt(t *testing.T) {
	// given — the small tier deliberately serves no delete_block (§8.20), so
	// running the restructure task there would measure a documented omission
	// rather than the loop
	task := taskById(t, "restructure-section")

	// then
	assert.False(t, task.runsOnTier(wrapper.TierSmall))
	assert.True(t, task.runsOnTier(wrapper.TierLarge))
	assert.NotContains(t, wrapper.ToolNamesForTier(wrapper.TierSmall), "delete_block")
}

func TestArmParsing(t *testing.T) {
	// when
	arms, err := parseArms("wrapper/small,ops")

	// then
	require.NoError(t, err)
	require.Len(t, arms, 2)
	assert.Equal(t, wrapper.TierSmall, arms[0].tier)
	assert.Equal(t, surfaceOps, arms[1].surface)

	_, err = parseArms("wrapper/medium")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrapper/small")
}
