package main

// tasks.go — the task table. Each task is a realistic small edit with a
// fixture the harness creates fresh per attempt and a check that asks the
// API what the document says afterwards. No check reads the model's own
// account of what it did, and none is a string match on chat output.
//
// Tasks are also chosen to reach the three instrumented questions: the
// add-content task is the one where insert_blocks authors a payload, the
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
	// Siblings are the extra objects a task was seeded with, read back
	// FRESH at check time and keyed the way the task named them. A check
	// that only ever sees its own object cannot tell "edited the right
	// one" from "edited everything" — which is the whole point of the
	// disambiguation and precision tasks.
	Siblings map[string]*document
}

// checkResult is a task check's verdict.
type checkResult struct {
	OK     bool
	Detail string
}

// sibling is an extra object seeded beside the fixture: a near-identical
// page to disambiguate against, or a second document to copy from. Its
// title lands in fixture.Extra[Key] and its id in Extra[Key+"Id"], so a
// prompt can name it and a check can read it back.
type sibling struct {
	Key string
	// TitleSuffix is appended to the SAME minted stem as the fixture, so
	// siblings are deliberately confusable — "Kolamira draft" beside
	// "Kolamira" — which is what makes a find ambiguous on purpose.
	TitleSuffix string
	Markdown    string
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
	// Type is the object type to seed, default "page". A dataview task
	// needs "collection": a collection is created WITH a dataview block and
	// one view, where a set's dataview needs a configured source and comes
	// back with none.
	Type string
	// Siblings are seeded before the attempt runs (setupFixture).
	Siblings []sibling
	Prompt   func(fx *fixture) string
	Check    func(doc *document, fx *fixture) checkResult
	// CheckAPI replaces Check for tasks whose result is not IN the fixture
	// document — a created type, a space's property list. It gets the live
	// client, so it can read back whatever the intent actually produced.
	// Exactly one of Check/CheckAPI must be set.
	CheckAPI func(ctx context.Context, client *apiClient, spaceId string, fx *fixture) checkResult
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
// wrapper re-verbs the op vocabulary (replace_text → edit_text, insert_blocks
// → add_blocks) and the gate has to ask both surfaces the same question.
// The two surfaces no longer share a KEY vocabulary — the wrapper teaches
// display names via ?keys=name while the ops arm and
// every harness read speak the slug default — but that split never reaches
// this mapping: capabilities name ACTIONS, and the graders read documents
// through the harness's own slug-default GETs, so their expected spellings
// (`due_date`, `created_date`) stay exact whatever arm produced the write.
type capability string

const (
	capRead          capability = "read the document"
	capEditText      capability = "replace text in a block"
	capAddBlocks     capability = "add blocks"
	capSetCell       capability = "write a table cell"
	capDeleteBlock   capability = "delete a block"
	capSetProperties capability = "set a property"
	capUpdateView    capability = "change a dataview's view"
	capCreateType    capability = "create an object type"
)

// capabilityTools names the tool each surface would use for each capability
// — the ONE place the two vocabularies meet. A hand-kept list of which task
// runs where is what let fill-table-cell run on a tier with no set_cell:
// the model recognised the limit and said so, and the matrix scored six
// guaranteed losses into the headline rate (§8.31's drift class, one level
// up from the schema).
var capabilityTools = map[capability]map[string]string{
	capRead:          {surfaceWrapper: "read", surfaceOps: "read_object"},
	capEditText:      {surfaceWrapper: "edit_text", surfaceOps: "replace_text"},
	capAddBlocks:     {surfaceWrapper: "add_blocks", surfaceOps: "insert_blocks"},
	capSetCell:       {surfaceWrapper: "set_cell", surfaceOps: "set_cell"},
	capDeleteBlock:   {surfaceWrapper: "delete_block", surfaceOps: "delete_block"},
	capSetProperties: {surfaceWrapper: "set_properties", surfaceOps: "set_properties"},
	capUpdateView:    {surfaceWrapper: "update_view", surfaceOps: "update_view"},
	// the ops arm has no type-creation op — POST /types is a route, not a
	// PATCH op — so this capability exists only on the wrapper, and
	// checkTaskGating skips the ops cells rather than scoring a surface for
	// something it cannot express
	capCreateType: {surfaceWrapper: "create_type"},
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

// surfaceCannotExpress reports a capability a surface genuinely has no way
// to express — distinct from one that is merely unmapped. Type creation is
// the first: POST /v2/spaces/{s}/types is a ROUTE, and the ops arm serves
// PATCH ops only, so there is no op to name. The distinction matters
// because checkTaskGating exists to catch the unmapped case loudly, and
// folding a real gap into that check would either fail every run or force
// a fake mapping that scores a surface for something it cannot do.
func surfaceCannotExpress(c capability, surface string) bool {
	return c == capCreateType && surface == surfaceOps
}

// checkTaskGating verifies every declared capability resolves on every
// surface, at startup rather than an hour into a run: an unmapped capability
// would otherwise skip a cell silently, which reads in the report exactly
// like a cell that was deliberately not measured.
func checkTaskGating() error {
	for _, t := range tasks() {
		for _, c := range t.Requires {
			for _, surface := range []string{surfaceWrapper, surfaceOps} {
				if surfaceCannotExpress(c, surface) {
					continue
				}
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
					if b.Type != "bulleted_list_item" {
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
			// the literal is QUOTED because the check demands its final
			// period: unquoted, "reading exactly: Deferred to Q4. Keep the
			// heading" reads that period as the sentence's own, and three
			// otherwise-perfect runs failed for writing "Deferred to Q4" —
			// the task measured how a model resolves an ambiguous prompt,
			// not whether it can restructure a section
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("In the page titled %q, replace the three bullet points under Next steps with a single "+
					"paragraph reading exactly \"Deferred to Q4.\" Keep the Next steps heading.", fx.Title)
			},
			Check: func(doc *document, fx *fixture) checkResult {
				if _, ok := doc.findBlock(func(b docBlock) bool {
					return strings.HasPrefix(b.Type, "heading") && strings.TrimSpace(b.Text) == "Next steps"
				}); !ok {
					return checkResult{Detail: "the Next steps heading is gone: " + describeBlocks(doc)}
				}
				for _, b := range doc.Blocks {
					if b.Type == "bulleted_list_item" {
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
				return fmt.Sprintf("Set the description property of the page titled %q to exactly \"Reviewed by the ops team.\"", fx.Title)
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
			// the §7.5a sweep's coverage: every OTHER task names only
			// single-word keys, which cannot tell a camelCase wire
			// vocabulary from a snake_case one. This one can — a model that
			// re-spells `due_date` back to `dueDate` from its training prior
			// still succeeds (the fold layer forgives it), but a SERVER that
			// stops advertising the slug fails the check.
			Id:       "set-multiword-property",
			Intent:   "set a bundled property whose key re-spells on the wire",
			Requires: []capability{capSetProperties},
			Markdown: "## Scope\n" +
				"The ops review is due at the end of the week.\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("Set the due date of the page titled %q to 2026-08-01.", fx.Title)
			},
			Check: func(doc *document, fx *fixture) checkResult {
				got, ok := doc.stringProperty("due_date")
				if !ok {
					return checkResult{Detail: fmt.Sprintf("no due_date property; properties: %v", propertyKeys(doc))}
				}
				if !strings.HasPrefix(got, "2026-08-01") {
					return checkResult{Detail: fmt.Sprintf("due_date is %q", got)}
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
		{
			// AMBIGUITY. Three pages share a stem, so a find on the name
			// alone returns all three numbered. The model has to read the
			// prompt's qualifier and pick, rather than take handle 1 —
			// gemma-4-e2b took the first row of a listing 14 times in 18,
			// which is the same reflex one step earlier.
			Id:       "disambiguate-match",
			Intent:   "several objects match the name; edit the one the prompt qualifies",
			Requires: []capability{capEditText},
			Markdown: "## Status\nThe rollout is on hold.\n",
			Siblings: []sibling{
				{Key: "draft", TitleSuffix: " draft", Markdown: "## Status\nThe rollout is on hold.\n"},
				{Key: "archive", TitleSuffix: " archive", Markdown: "## Status\nThe rollout is on hold.\n"},
			},
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("Three pages are named after %[1]q: %[1]q, %[2]q and %[3]q. "+
					"In %[1]q only — not the draft, not the archive — change \"on hold\" to \"approved\".",
					fx.Title, fx.Extra["draft"], fx.Extra["archive"])
			},
			Check: func(doc *document, fx *fixture) checkResult {
				if !strings.Contains(doc.allText(), "The rollout is approved.") {
					return checkResult{Detail: "the named page still reads: " + strings.Join(doc.blockTexts(), " | ")}
				}
				for key, sib := range fx.Siblings {
					if sib == nil {
						continue
					}
					if !strings.Contains(sib.allText(), "on hold") {
						return checkResult{Detail: fmt.Sprintf("collateral damage: the %s page was edited too", key)}
					}
				}
				return checkResult{OK: true}
			},
		},
		{
			// RECOVERY. The snippet appears in two blocks, so the locator
			// refuses and names the candidates. Passing means reading that
			// refusal and narrowing — the repair loop the wrapper's steering
			// exists to teach, measured directly for the first time.
			Id:       "recover-from-ambiguous-edit",
			Intent:   "the obvious edit is refused as ambiguous; narrow it",
			Requires: []capability{capEditText},
			Markdown: "## Owner\nStatus: pending\n\n## Reviewer\nStatus: pending\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("In the page titled %q, under the Reviewer heading only, change \"Status: pending\" "+
					"to \"Status: signed off\". Leave the Owner section exactly as it is.", fx.Title)
			},
			Check: func(doc *document, fx *fixture) checkResult {
				var ownerOK, reviewerOK bool
				section := ""
				for _, b := range doc.Blocks {
					if strings.HasPrefix(b.Type, "heading") {
						section = strings.TrimSpace(b.Text)
						continue
					}
					text := strings.TrimSpace(b.Text)
					if section == "Owner" && text == "Status: pending" {
						ownerOK = true
					}
					if section == "Reviewer" && text == "Status: signed off" {
						reviewerOK = true
					}
				}
				if !reviewerOK {
					return checkResult{Detail: "the Reviewer status was not changed: " + describeBlocks(doc)}
				}
				if !ownerOK {
					return checkResult{Detail: "collateral damage: the Owner status changed too: " + describeBlocks(doc)}
				}
				return checkResult{OK: true}
			},
		},
		{
			// CROSS-OBJECT. The value lives in a second document, so the
			// model must hold two objects in one attempt — and every find
			// RENUMBERS the handles, which is where the reference channel
			// is most likely to slip.
			Id:       "copy-between-objects",
			Intent:   "read a value from one object and write it onto another",
			Requires: []capability{capRead, capAddBlocks},
			Markdown: "## Release notes\nOwner to be confirmed.\n",
			Siblings: []sibling{
				{Key: "source", TitleSuffix: " source", Markdown: "## Contacts\nRelease owner: Dana Whitfield\n"},
			},
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("The page %q names a release owner. Add a paragraph to the end of the page titled %q "+
					"that reads exactly \"Release owner: \" followed by that person's name.", fx.Extra["source"], fx.Title)
			},
			Check: func(doc *document, fx *fixture) checkResult {
				want := "Release owner: Dana Whitfield"
				if _, ok := doc.findBlock(func(b docBlock) bool { return strings.TrimSpace(b.Text) == want }); !ok {
					return checkResult{Detail: fmt.Sprintf("no block reads %q; blocks: %q", want, doc.blockTexts())}
				}
				if src := fx.Siblings["source"]; src != nil && !strings.Contains(src.allText(), "Dana Whitfield") {
					return checkResult{Detail: "collateral damage: the source page was edited"}
				}
				return checkResult{OK: true}
			},
		},
		{
			// PRECISION. THREE rows read "Pending", so a model that sets
			// them all — or picks by value instead of by row — fails. The
			// shipped fill-table-cell has one Pending row and cannot catch
			// over-editing at all.
			Id:       "fill-one-of-many-cells",
			Intent:   "set one cell where several rows share its value",
			Requires: []capability{capSetCell},
			Markdown: "## Components\n" +
				"| Component | Status |\n" +
				"| --- | --- |\n" +
				"| Alpha | Pending |\n" +
				"| Beta | Pending |\n" +
				"| Gamma | Pending |\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("In the page titled %q, the Beta component is finished — set its Status cell to Done. "+
					"Alpha and Gamma are still pending and must not change.", fx.Title)
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
				want := map[string]string{"Alpha": "Pending", "Beta": "Done", "Gamma": "Pending"}
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
			// DATAVIEW. A collection arrives sorted by name ascending; the
			// task is to re-sort it. The whole intent is one call, so this
			// measures whether the model can express a STRUCTURED change
			// rather than whether it can chain — a different axis from every
			// editing task.
			Id:       "sort-a-view",
			Intent:   "change a dataview's sort order",
			Type:     "collection",
			Requires: []capability{capUpdateView},
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("The collection titled %q is sorted by name. Sort it by the created date instead, "+
					"newest first.", fx.Title)
			},
			Check: func(doc *document, fx *fixture) checkResult {
				v, ok := doc.view()
				if !ok {
					return checkResult{Detail: "no dataview view on the collection: " + describeBlocks(doc)}
				}
				if len(v.Sorts) != 1 {
					return checkResult{Detail: fmt.Sprintf("want exactly one sort, got %d: %+v", len(v.Sorts), v.Sorts)}
				}
				if v.Sorts[0].Property != "created_date" {
					return checkResult{Detail: fmt.Sprintf("sorted by %q, want created_date", v.Sorts[0].Property)}
				}
				if v.Sorts[0].Direction != "desc" {
					return checkResult{Detail: fmt.Sprintf("direction is %q, want desc (newest first)", v.Sorts[0].Direction)}
				}
				return checkResult{OK: true}
			},
		},
		{
			// DATAVIEW. The filter arrives as the compact string `find`
			// already publishes, so this measures whether one vocabulary
			// taught in one tool transfers to another.
			Id:       "filter-a-view",
			Intent:   "add a filter to a dataview",
			Type:     "collection",
			Requires: []capability{capUpdateView},
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("In the collection titled %q, show only objects whose name is not empty.", fx.Title)
			},
			Check: func(doc *document, fx *fixture) checkResult {
				v, ok := doc.view()
				if !ok {
					return checkResult{Detail: "no dataview view on the collection: " + describeBlocks(doc)}
				}
				if len(v.Filters) == 0 {
					return checkResult{Detail: "the view has no filter"}
				}
				for _, f := range v.Filters {
					if f.Property == "name" {
						return checkResult{OK: true}
					}
				}
				return checkResult{Detail: fmt.Sprintf("no filter on name; filters: %+v", v.Filters)}
			},
		},
		{
			// DATAVIEW + PRECISION. created_date starts hidden and name
			// starts visible. Showing the one without hiding the other is
			// the trap: the columns channel is a per-key merge, so a model
			// that thinks in terms of "the visible set" can blank the view.
			Id:       "show-a-column",
			Intent:   "make a hidden dataview column visible",
			Type:     "collection",
			Requires: []capability{capUpdateView},
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("The collection titled %q does not show the created date. Make that column visible, "+
					"leaving the name column visible as it is.", fx.Title)
			},
			Check: func(doc *document, fx *fixture) checkResult {
				v, ok := doc.view()
				if !ok {
					return checkResult{Detail: "no dataview view on the collection: " + describeBlocks(doc)}
				}
				var createdShown, nameShown bool
				for _, c := range v.Columns {
					switch c.Property {
					case "created_date":
						createdShown = !c.Hidden
					case "name":
						nameShown = !c.Hidden
					}
				}
				if !createdShown {
					return checkResult{Detail: fmt.Sprintf("created_date is still hidden; columns: %+v", v.Columns)}
				}
				if !nameShown {
					return checkResult{Detail: "collateral damage: the name column was hidden"}
				}
				return checkResult{OK: true}
			},
		},
		{
			// STEP 1 / rank 1 — CROSS-OBJECT ADDRESSING, the axis that
			// separates models: gemma-4-e4b scored 1/4 on copy-between-objects
			// where bonsai-27b scored 4/4. Here the second object is named by
			// an objects-format PROPERTY rather than copied as an id, which is
			// the step that fails. Two calls if addressing works: find, then
			// set_properties naming the project.
			Id:       "file-under-project",
			Intent:   "point an objects-format property at another object, by name",
			Requires: []capability{capSetProperties},
			Markdown: "## Notes\nDrafted during the offsite.\n",
			Siblings: []sibling{
				{Key: "project", TitleSuffix: " project", Markdown: "## Project\nThe umbrella for this work.\n"},
			},
			// the prompt names no property: a fresh account's Page type has
			// its own bundled set (there is no "Related"), and naming one
			// that does not exist measures the model's recovery rather than
			// its addressing. It must find a property that holds objects —
			// describe is how — and point it at the sibling.
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("On the page titled %q, record that it belongs to the page titled %q: "+
					"set whichever property of it holds links to other objects.", fx.Title, fx.Extra["project"])
			},
			// ANY property may carry it — which one the model picks is its
			// choice to make, and constraining the grader to one key made a
			// passing run read as a failure
			Check: func(doc *document, fx *fixture) checkResult {
				want := fx.Extra["projectId"]
				for key, raw := range doc.Properties {
					if strings.Contains(fmt.Sprint(raw), want) {
						_ = key
						return checkResult{OK: true}
					}
				}
				return checkResult{Detail: fmt.Sprintf("no property points at %s (%s); properties: %v",
					fx.Extra["project"], want, propertyKeys(doc))}
			},
		},
		{
			// STEP 1 / rank 3 — CHAIN LENGTH. Three objects, one intent. A
			// model that issues one call per object is doing the thing that
			// went 0/6 before batch delete_block existed; one call is the fix
			// this task measures. The siblings are the other two targets.
			Id:       "bulk-set-property",
			Intent:   "set the same property on several objects at once",
			Requires: []capability{capSetProperties},
			Markdown: "## Task\nFirst of three.\n",
			Siblings: []sibling{
				{Key: "second", TitleSuffix: " second", Markdown: "## Task\nSecond of three.\n"},
				{Key: "third", TitleSuffix: " third", Markdown: "## Task\nThird of three.\n"},
			},
			Prompt: func(fx *fixture) string {
				// the target literal deliberately ends in a WORD, not a period:
				// a required value ending in punctuation makes the sentence's
				// own full stop ambiguous, and that alone failed three
				// otherwise-perfect runs of restructure-section before it was
				// quoted — and failed this task even when quoted
				return fmt.Sprintf("Set the description of all three pages named after %q — %q, %q and %q — to exactly \"reviewed by ops\"",
					fx.Title, fx.Title, fx.Extra["second"], fx.Extra["third"])
			},
			Check: func(doc *document, fx *fixture) checkResult {
				got, ok := doc.stringProperty("description")
				if !ok || strings.TrimSpace(got) != "reviewed by ops" {
					return checkResult{Detail: fmt.Sprintf("the named page's description is %q", got)}
				}
				for key, sib := range fx.Siblings {
					if sib == nil {
						return checkResult{Detail: "sibling " + key + " was not read back"}
					}
					sibGot, sibOK := sib.stringProperty("description")
					if !sibOK || strings.TrimSpace(sibGot) != "reviewed by ops" {
						return checkResult{Detail: fmt.Sprintf("%s page's description is %q, want \"reviewed by ops\"", key, sibGot)}
					}
				}
				return checkResult{OK: true}
			},
		},
		{
			// SCHEMA AUTHORING. Published research rates this the worst agent
			// category on record (best model 42% on MCPMark), and a wrong
			// type is PERMANENT here — no delete_type, and the type PATCH
			// replaces rather than appends. So the grader checks the whole
			// shape, not just that something was created: the wrong type is
			// as bad as no type.
			Id:       "create-a-type",
			Intent:   "define a new object type with typed properties",
			Requires: []capability{capCreateType},
			Markdown: "## Scope\nThe cookbook needs a type.\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("In this space, create an object type called %q with three properties: "+
					"\"Cook time\" holding a number, \"Source\" holding a URL, and \"Rating\" a select "+
					"offering Low, Medium and High.", fx.Extra["typeName"])
			},
			CheckAPI: func(ctx context.Context, client *apiClient, spaceId string, fx *fixture) checkResult {
				want := fx.Extra["typeName"]
				t, err := client.findTypeByName(ctx, spaceId, want)
				if err != nil {
					return checkResult{Detail: fmt.Sprintf("read types: %v", err)}
				}
				if t == nil {
					return checkResult{Detail: fmt.Sprintf("no type named %q exists", want)}
				}
				got := map[string]string{}
				opts := map[string][]string{}
				for _, p := range t.Properties {
					got[strings.ToLower(strings.TrimSpace(p.Name))] = p.Format
					opts[strings.ToLower(strings.TrimSpace(p.Name))] = p.Options
				}
				for name, format := range map[string]string{"cook time": "number", "source": "url", "rating": "select"} {
					if got[name] != format {
						return checkResult{Detail: fmt.Sprintf("property %q has format %q, want %q (type has: %v)",
							name, got[name], format, got)}
					}
				}
				have := map[string]bool{}
				for _, o := range opts["rating"] {
					have[strings.ToLower(strings.TrimSpace(o))] = true
				}
				for _, want := range []string{"low", "medium", "high"} {
					if !have[want] {
						return checkResult{Detail: fmt.Sprintf("Rating is missing the %q option; it offers %v", want, opts["rating"])}
					}
				}
				return checkResult{OK: true}
			},
		},
	}
}

// newFixtureFor mints the derived names an attempt needs from one stem, so
// the runner and the well-formedness test agree about what a fixture holds.
// The type name is derived rather than fixed: a type cannot be deleted
// through this surface, so a shared name would accumulate forever.
func newFixtureFor(title string) *fixture {
	return &fixture{
		Title:    title,
		Extra:    map[string]string{"typeName": title + " Recipe"},
		Siblings: map[string]*document{},
	}
}

// setupFixture creates one attempt's object, plus any siblings the task
// asked for. Siblings share the fixture's minted stem so they collide in a
// search the way real neighbours do.
func setupFixture(ctx context.Context, client *apiClient, spaceId string, t task) (*fixture, error) {
	title := fixtureTitle()
	typeKey := t.Type
	if typeKey == "" {
		typeKey = "page"
	}
	id, err := client.createObject(ctx, spaceId, typeKey, title, t.Markdown)
	if err != nil {
		return nil, fmt.Errorf("create fixture for %s: %w", t.Id, err)
	}
	fx := newFixtureFor(title)
	fx.ObjectId = id
	for _, sib := range t.Siblings {
		sibTitle := title + sib.TitleSuffix
		sibId, err := client.createObject(ctx, spaceId, "page", sibTitle, sib.Markdown)
		if err != nil {
			return nil, fmt.Errorf("create sibling %q for %s: %w", sib.Key, t.Id, err)
		}
		fx.Extra[sib.Key] = sibTitle
		fx.Extra[sib.Key+"Id"] = sibId
	}
	return fx, nil
}

// readSiblings re-reads every seeded sibling so a check can prove the model
// left them alone. A stale copy would let collateral damage pass.
func readSiblings(ctx context.Context, client *apiClient, spaceId string, fx *fixture, t task) error {
	for _, sib := range t.Siblings {
		id := fx.Extra[sib.Key+"Id"]
		if id == "" {
			continue
		}
		doc, _, err := client.getDocument(ctx, spaceId, id)
		if err != nil {
			return fmt.Errorf("read sibling %q: %w", sib.Key, err)
		}
		fx.Siblings[sib.Key] = doc
	}
	return nil
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
