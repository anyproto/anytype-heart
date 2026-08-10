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

import (
	"context"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/core/api/wrapper"
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
	// Tiers restricts which wrapper tiers run it (empty = both). The
	// restructure task needs delete_block, which the small tier
	// deliberately does not serve — running it there would measure a
	// documented omission, not a loop failure.
	Tiers []wrapper.Tier
	// Markdown is the fixture body; TitleStem + a per-attempt nonce is its
	// name, so concurrent and repeated runs never edit each other's objects.
	TitleStem string
	Markdown  string
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

func (t task) runsOnTier(tier wrapper.Tier) bool {
	if len(t.Tiers) == 0 {
		return true
	}
	for _, x := range t.Tiers {
		if x == tier {
			return true
		}
	}
	return false
}

// tasks is the table.
func tasks() []task {
	return []task{
		{
			Id:        "edit-one-word",
			Intent:    "change one word in a document",
			TitleStem: "Quarterly plan",
			Markdown: "## Summary\n" +
				"Revenue target for Q3 is 1.2M.\n\n" +
				"## Owner\n" +
				"The finance team reviews this monthly.\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("In the note titled %q, change Q3 to Q4. Change nothing else.", fx.Title)
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
			Id:        "append-section",
			Intent:    "add a section of content",
			TitleStem: "Migration notes",
			Markdown: "## Overview\n" +
				"The migration runs in three stages.\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("Add a section at the end of the note titled %q: a heading that reads Risks, "+
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
			Id:        "fill-table-cell",
			Intent:    "fill a table cell",
			TitleStem: "Release checklist",
			Markdown: "## Components\n\n" +
				"| Component | Status |\n" +
				"| --- | --- |\n" +
				"| Alpha | Done |\n" +
				"| Beta | Pending |\n" +
				"| Gamma | Pending |\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("In the note titled %q there is a components table. The Beta component is finished — "+
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
			Id:        "restructure-section",
			Intent:    "replace a subtree with different content",
			Tiers:     []wrapper.Tier{wrapper.TierLarge}, // small tier has no delete_block, by design
			TitleStem: "Project status",
			Markdown: "## Next steps\n" +
				"- Ship the beta\n" +
				"- Collect feedback\n" +
				"- Write the report\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("In the note titled %q, replace the three bullet points under Next steps with a single "+
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
			Id:        "set-property",
			Intent:    "set a property value",
			TitleStem: "Vendor review",
			Markdown: "## Scope\n" +
				"Three vendors were compared on price and support.\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("Set the description property of the note titled %q to exactly: Reviewed by the ops team.", fx.Title)
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
			Id:        "read-then-edit",
			Intent:    "multi-step: a read is required before the edit is knowable",
			TitleStem: "Handover notes",
			Markdown: "## Meeting notes\n" +
				"Owner: Priya Raman\n" +
				"Next review: 12 May\n",
			Prompt: func(fx *fixture) string {
				return fmt.Sprintf("The owner of the note titled %q has changed to Dana Whitfield. "+
					"Update the note so the Owner line names the new owner instead of the old one.", fx.Title)
			},
			Check: func(doc *document, fx *fixture) checkResult {
				if _, ok := doc.findBlock(func(b docBlock) bool {
					return strings.TrimSpace(b.Text) == "Owner: Dana Whitfield"
				}); !ok {
					return checkResult{Detail: "no block reads \"Owner: Dana Whitfield\": " + describeBlocks(doc)}
				}
				if strings.Contains(doc.allText(), "Priya Raman") {
					return checkResult{Detail: "the old owner is still named"}
				}
				if !strings.Contains(doc.allText(), "Next review: 12 May") {
					return checkResult{Detail: "collateral damage: the review line was changed"}
				}
				return checkResult{OK: true}
			},
		},
	}
}

// setupFixture creates one attempt's object.
func setupFixture(ctx context.Context, client *apiClient, spaceId string, t task) (*fixture, error) {
	nonce := newNonce(3)
	title := fmt.Sprintf("%s %s", t.TitleStem, nonce)
	id, err := client.createObject(ctx, spaceId, "page", title, t.Markdown)
	if err != nil {
		return nil, fmt.Errorf("create fixture for %s: %w", t.Id, err)
	}
	return &fixture{ObjectId: id, Title: title, Extra: map[string]string{"nonce": nonce}}, nil
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
