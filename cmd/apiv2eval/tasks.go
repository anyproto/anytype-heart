package main

// tasks.go — the task table. Each task is a realistic small edit with a
// fixture the harness creates fresh per attempt and a check that asks the
// API what the document says afterwards. No check reads the model's own
// account of what it did, and none is a string match on chat output.
//
// Tasks are also chosen to reach the three instrumented questions: the
// add-content task is the one where insertBlocks authors a payload, the
// table and restructure tasks are where block ids must be echoed, and the
// deliberately ambiguous snippet in the table task is a refusal a model has
// to repair from.
//
// A task declares the CAPABILITIES it cannot be done without, and the run
// derives which cells to skip from them (capabilityTools + planCells). It
// does not name tiers by hand: that is what let fill-table-cell run on a
// small tier with no set_cell, where the model correctly reported the limit
// and the matrix scored the answer as a failure.

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

// fixture is one attempt's freshly created object.
type fixture struct {
	ObjectId string
	Title    string
	Extra    map[string]string
}

// checkResult is a task check's verdict.
type checkResult struct {
	OK     bool
	Detail string
}

// task is one evaluated intent.
type task struct {
	Id     string
	Intent string
	// Arms restricts which surfaces run it (empty = both).
	Arms []string
	// Requires names the capabilities the task cannot be completed without.
	// The gate is DERIVED from this against each arm's published tool set
	// (capabilityTools, armSpec.publishedTools): a cell whose arm publishes
	// no tool for a required capability is not run, because it asks a model
	// to do something the surface it was handed cannot do.
	Requires []capability
	// Markdown is the fixture body; the name is minted per attempt by
	// fixtureTitle, so repeated runs never edit — or find — each other's
	// objects.
	Markdown string
	// Prompt is the user turn. It names the object by TITLE (the wrapper arm
	// must find it) — the ops arm is bound to the object already.
	Prompt func(fx *fixture) string
	Check  func(doc *document, fx *fixture) checkResult
}

func (t task) runsOnArm(arm string) bool {
	if len(t.Arms) == 0 {
		return true
	}
	for _, a := range t.Arms {
		if a == arm {
			return true
		}
	}
	return false
}

//
// ---- capability gating ----
//

// capability is one thing a task cannot be done without, named once for both
// surfaces. Tasks declare capabilities rather than tool names because the
// wrapper renames the op vocabulary (setCell → set_cell, replaceText →
// edit_text), and the gate has to ask both surfaces the same question.
type capability string

const (
	capRead          capability = "read the document"
	capEditText      capability = "replace text in a block"
	capAddBlocks     capability = "add blocks"
	capSetCell       capability = "write a table cell"
	capDeleteBlock   capability = "delete a block"
	capSetProperties capability = "set a property"
)

// capabilityTools names the tool each surface would use for each capability
// — the ONE place the two vocabularies meet. A hand-kept list of which task
// runs where is what let fill-table-cell run on a tier with no set_cell:
// the model recognised the limit and said so, and the matrix scored six
// guaranteed losses into the headline rate (§8.31's drift class, one level
// up from the schema).
var capabilityTools = map[capability]map[string]string{
	capRead:          {surfaceWrapper: "read", surfaceOps: "read_object"},
	capEditText:      {surfaceWrapper: "edit_text", surfaceOps: "replaceText"},
	capAddBlocks:     {surfaceWrapper: "add_blocks", surfaceOps: "insertBlocks"},
	capSetCell:       {surfaceWrapper: "set_cell", surfaceOps: "setCell"},
	capDeleteBlock:   {surfaceWrapper: "delete_block", surfaceOps: "deleteBlock"},
	capSetProperties: {surfaceWrapper: "set_properties", surfaceOps: "setProperties"},
}

// capabilityTool returns the surface's tool for a capability.
func capabilityTool(c capability, surface string) (string, error) {
	bySurface, ok := capabilityTools[c]
	if !ok {
		return "", fmt.Errorf("capability %q is not in capabilityTools", c)
	}
	tool, ok := bySurface[surface]
	if !ok {
		return "", fmt.Errorf("capability %q names no tool on the %s surface", c, surface)
	}
	return tool, nil
}

// checkTaskGating verifies every declared capability resolves on every
// surface, at startup rather than an hour into a run: an unmapped capability
// would otherwise skip a cell silently, which reads in the report exactly
// like a cell that was deliberately not measured.
func checkTaskGating() error {
	for _, t := range tasks() {
		for _, c := range t.Requires {
			for _, surface := range []string{surfaceWrapper, surfaceOps} {
				if _, err := capabilityTool(c, surface); err != nil {
					return fmt.Errorf("task %s: %w", t.Id, err)
				}
			}
		}
	}
	return nil
}

// tasks is the table.
func tasks() []task {
	return []task{
		{
			Id:       "edit-one-word",
			Intent:   "change one word in a document",
			Requires: []capability{capEditText},
			Markdown: "## Summary\n" +
				"Revenue target for Q3 is 1.2M.\n\n" +
				"## Owner\n" +
				"The finance team reviews this monthly.\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("In the page titled %q, change Q3 to Q4. Change nothing else.", fx.Title)
			},
			Check: func(doc *document, fx *fixture) checkResult {
				want := "Revenue target for Q4 is 1.2M."
				if _, ok := doc.findBlock(func(b docBlock) bool { return strings.TrimSpace(b.Text) == want }); !ok {
					return checkResult{Detail: fmt.Sprintf("no block reads %q; blocks: %q", want, doc.blockTexts())}
				}
				if strings.Contains(doc.allText(), "Q3") {
					return checkResult{Detail: "Q3 still present: " + strings.Join(doc.blockTexts(), " | ")}
				}
				if !strings.Contains(doc.allText(), "The finance team reviews this monthly.") {
					return checkResult{Detail: "collateral damage: the Owner section was changed"}
				}
				return checkResult{OK: true}
			},
		},
		{
			Id:       "append-section",
			Intent:   "add a section of content",
			Requires: []capability{capAddBlocks},
			Markdown: "## Overview\n" +
				"The migration runs in three stages.\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("Add a section at the end of the page titled %q: a heading that reads Risks, "+
					"followed by two bullet points reading Vendor delay and Budget overrun.", fx.Title)
			},
			Check: func(doc *document, fx *fixture) checkResult {
				headingAt := -1
				for i, b := range doc.Blocks {
					if strings.HasPrefix(b.Type, "heading") && strings.TrimSpace(b.Text) == "Risks" {
						headingAt = i
					}
				}
				if headingAt < 0 {
					return checkResult{Detail: fmt.Sprintf("no heading block reads \"Risks\"; blocks: %s", describeBlocks(doc))}
				}
				want := map[string]bool{"Vendor delay": false, "Budget overrun": false}
				for _, b := range doc.Blocks[headingAt+1:] {
					if b.Type != "bulletedListItem" {
						continue
					}
					if _, ok := want[strings.TrimSpace(b.Text)]; ok {
						want[strings.TrimSpace(b.Text)] = true
					}
				}
				for text, found := range want {
					if !found {
						return checkResult{Detail: fmt.Sprintf("no bullet after the heading reads %q; blocks: %s", text, describeBlocks(doc))}
					}
				}
				if !strings.Contains(doc.allText(), "The migration runs in three stages.") {
					return checkResult{Detail: "collateral damage: the Overview section was changed"}
				}
				return checkResult{OK: true}
			},
		},
		{
			Id:       "fill-table-cell",
			Intent:   "fill a table cell",
			Requires: []capability{capSetCell},
			Markdown: "## Components\n\n" +
				"| Component | Status |\n" +
				"| --- | --- |\n" +
				"| Alpha | Done |\n" +
				"| Beta | Pending |\n" +
				"| Gamma | Pending |\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("In the page titled %q there is a components table. The Beta component is finished — "+
					"set its Status cell to Done. Leave every other row alone.", fx.Title)
			},
			Check: func(doc *document, fx *fixture) checkResult {
				table, ok := doc.table()
				if !ok {
					return checkResult{Detail: "the table block is gone: " + describeBlocks(doc)}
				}
				statusCol := -1
				for _, row := range table.Rows {
					if !row.IsHeader {
						continue
					}
					for i, cell := range row.Cells {
						if strings.EqualFold(strings.TrimSpace(cellText(cell)), "Status") {
							statusCol = i
						}
					}
				}
				if statusCol < 0 {
					return checkResult{Detail: "no Status column in the header row"}
				}
				want := map[string]string{"Alpha": "Done", "Beta": "Done", "Gamma": "Pending"}
				got := map[string]string{}
				for _, row := range table.Rows {
					if row.IsHeader || len(row.Cells) == 0 {
						continue
					}
					name := strings.TrimSpace(cellText(row.Cells[0]))
					value := ""
					if statusCol < len(row.Cells) {
						value = strings.TrimSpace(cellText(row.Cells[statusCol]))
					}
					got[name] = value
				}
				for name, value := range want {
					if got[name] != value {
						return checkResult{Detail: fmt.Sprintf("row %s status is %q, want %q (table now: %v)", name, got[name], value, got)}
					}
				}
				return checkResult{OK: true}
			},
		},
		{
			Id:       "restructure-section",
			Intent:   "replace a subtree with different content",
			Requires: []capability{capDeleteBlock, capAddBlocks},
			Markdown: "## Next steps\n" +
				"- Ship the beta\n" +
				"- Collect feedback\n" +
				"- Write the report\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("In the page titled %q, replace the three bullet points under Next steps with a single "+
					"paragraph reading exactly: Deferred to Q4. Keep the Next steps heading.", fx.Title)
			},
			Check: func(doc *document, fx *fixture) checkResult {
				if _, ok := doc.findBlock(func(b docBlock) bool {
					return strings.HasPrefix(b.Type, "heading") && strings.TrimSpace(b.Text) == "Next steps"
				}); !ok {
					return checkResult{Detail: "the Next steps heading is gone: " + describeBlocks(doc)}
				}
				for _, b := range doc.Blocks {
					if b.Type == "bulletedListItem" {
						return checkResult{Detail: "a bullet survives: " + describeBlocks(doc)}
					}
				}
				para, ok := doc.findBlock(func(b docBlock) bool {
					return b.Type == "paragraph" && strings.TrimSpace(b.Text) == "Deferred to Q4."
				})
				if !ok {
					return checkResult{Detail: "no paragraph reads \"Deferred to Q4.\": " + describeBlocks(doc)}
				}
				_ = para
				return checkResult{OK: true}
			},
		},
		{
			Id:       "set-property",
			Intent:   "set a property value",
			Requires: []capability{capSetProperties},
			Markdown: "## Scope\n" +
				"Three vendors were compared on price and support.\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("Set the description property of the page titled %q to exactly: Reviewed by the ops team.", fx.Title)
			},
			Check: func(doc *document, fx *fixture) checkResult {
				got, ok := doc.stringProperty("description")
				if !ok {
					return checkResult{Detail: fmt.Sprintf("no description property; properties: %v", propertyKeys(doc))}
				}
				if strings.TrimSpace(got) != "Reviewed by the ops team." {
					return checkResult{Detail: fmt.Sprintf("description is %q", got)}
				}
				return checkResult{OK: true}
			},
		},
		{
			Id:       "read-then-edit",
			Intent:   "multi-step: a read is required before the edit is knowable",
			Requires: []capability{capRead, capEditText},
			Markdown: "## Meeting notes\n" +
				"Owner: Priya Raman\n" +
				"Next review: 12 May\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("The owner of the page titled %q has changed to Dana Whitfield. "+
					"Update the note so the Owner line names the new owner instead of the old one.", fx.Title)
			},
			// the two lines of the fixture are ONE block: markdown without a
			// blank line between them imports as a single paragraph holding a
			// soft break. The check therefore reads LINES, not blocks — the
			// first version compared whole block texts and failed a model that
			// had done the task exactly right, which is the same defect class
			// as running a task on a tier that cannot do it.
			Check: func(doc *document, fx *fixture) checkResult {
				if !containsLine(doc, "Owner: Dana Whitfield") {
					return checkResult{Detail: "no line reads \"Owner: Dana Whitfield\": " + describeBlocks(doc)}
				}
				if strings.Contains(doc.allText(), "Priya Raman") {
					return checkResult{Detail: "the old owner is still named"}
				}
				if !containsLine(doc, "Next review: 12 May") {
					return checkResult{Detail: "collateral damage: the review line was changed"}
				}
				return checkResult{OK: true}
			},
		},
	}
}

// setupFixture creates one attempt's object.
func setupFixture(ctx context.Context, client *apiClient, spaceId string, t task) (*fixture, error) {
	title := fixtureTitle()
	id, err := client.createObject(ctx, spaceId, "page", title, t.Markdown)
	if err != nil {
		return nil, fmt.Errorf("create fixture for %s: %w", t.Id, err)
	}
	return &fixture{ObjectId: id, Title: title, Extra: map[string]string{}}, nil
}

// titleSyllables are the pieces fixtureTitle builds a name out of: 15
// consonants × 5 vowels, four syllables, always eight letters.
const (
	titleConsonants = "bdfgjklmnprstvz"
	titleVowels     = "aeiou"
	titleSyllables  = 4
)

// fixtureTitle mints one attempt's object name as a coined single-token
// codename — never a shared stem plus a nonce.
//
// The API has no object DELETE, so every attempt's fixture stays in the eval
// space forever, and search matches a query TOKEN-wise: measured against the
// live server, `find "Quarterly plan 84353d"` returned all five leftover
// "Quarterly plan …" notes, and `"Migration notes"` matched "Handover notes"
// on the word they share. Across one aborted run the same task's find went
// 1 → 2 → 3 matches, so a long run's success rate would decay for a reason
// that is the harness's, not the API's. A per-run space fixes only the
// cross-run half of that: a run makes ~17 fixtures per task itself.
//
// One coined token shares nothing with any other fixture. Two properties of
// the server's search make it exact, both measured rather than assumed:
// there is no fuzzy matching (Zafuriko and Zafurika each return only
// themselves) and there IS prefix matching (Zafurik returns both), so the
// names are fixed-length — one can be a prefix of another only by being
// equal. 75 syllables to the fourth is ~32M names.
//
// What this deliberately gives up: find becomes an unambiguous lookup. How a
// model picks among several plausible matches is a real question, and it is
// now a question a run has to ASK rather than one it answers by accident
// with whatever previous attempts left lying around.
func fixtureTitle() string {
	raw := make([]byte, titleSyllables*2)
	if _, err := rand.Read(raw); err != nil {
		// a colliding title costs the run its find isolation, nothing else
		nano := time.Now().UnixNano()
		for i := range raw {
			raw[i] = byte(nano >> (uint(i) * 8))
		}
	}
	name := make([]byte, 0, titleSyllables*2)
	for i := 0; i < titleSyllables; i++ {
		name = append(name,
			titleConsonants[int(raw[i*2])%len(titleConsonants)],
			titleVowels[int(raw[i*2+1])%len(titleVowels)])
	}
	return strings.ToUpper(string(name[:1])) + string(name[1:])
}

// containsLine reports whether any block holds a LINE equal to want. A
// served block's text can carry soft breaks, so a whole-text comparison
// asks for a document shape the markdown importer does not produce.
func containsLine(doc *document, want string) bool {
	for _, b := range doc.Blocks {
		for _, line := range strings.Split(b.Text, "\n") {
			if strings.TrimSpace(line) == want {
				return true
			}
		}
	}
	return false
}

// describeBlocks renders a document compactly for a failing check's detail.
func describeBlocks(doc *document) string {
	var parts []string
	for _, b := range doc.Blocks {
		text := b.Text
		if len(text) > 40 {
			text = text[:40] + "…"
		}
		parts = append(parts, fmt.Sprintf("%s/%s:%q", b.Id, b.Type, text))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func propertyKeys(doc *document) []string {
	keys := make([]string, 0, len(doc.Properties))
	for k := range doc.Properties {
		keys = append(keys, k)
	}
	return keys
}
