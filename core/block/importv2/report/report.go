// Package report builds the per-run import report page (§16 item 1, decision
// §13.7) — the primary user surface for the issue ledger. The engine persists
// it through the normal pool at the end of a run that produced issues; the
// notification and EventImportFinish carry its id so clients can open it and
// render a discard button. Output is deterministic (stable block ids, sorted
// groups) so golden tests stay stable.
package report

import (
	"fmt"
	"sort"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	textutil "github.com/anyproto/anytype-heart/util/text"
)

// SourceKey is the report page's claim key. Ids are minted per run, so every
// run with issues yields its own report page.
const SourceKey = "import-report"

const iconEmoji = "📋"

// Build renders the issue ledger as a page object: a summary table
// (severity × code × count) followed by one toggle per group holding a line
// per issue. resolve maps a source key to its created object id for mention
// links; unresolvable keys degrade to plain text.
func Build(title string, issues []importv2.Issue, dropped int64, resolve func(sourceKey string) (id string, ok bool)) *importv2.Object {
	groups := groupIssues(issues)

	var blocks []*model.Block
	blocks = append(blocks, summaryTable(groups, dropped)...)
	for groupIndex, group := range groups {
		blocks = append(blocks, toggleGroup(groupIndex, group, resolve)...)
	}
	if dropped > 0 {
		blocks = append(blocks, textBlock("overflow",
			fmt.Sprintf("%d further issues were not recorded (report cap reached)", dropped), nil))
	}

	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyName, title)
	details.SetString(bundle.RelationKeyIconEmoji, iconEmoji)
	return &importv2.Object{
		SourceKey: SourceKey,
		SbType:    coresb.SmartBlockTypePage,
		Payload: &importv2.Snapshot{
			Blocks:      blocks,
			Details:     details,
			ObjectTypes: []string{bundle.TypeKeyPage.String()},
		},
	}
}

// group is one (severity, code) bucket, issues in stream order.
type group struct {
	severity importv2.Severity
	code     importv2.IssueCode
	issues   []importv2.Issue
}

func groupIssues(issues []importv2.Issue) []group {
	type key struct {
		severity importv2.Severity
		code     importv2.IssueCode
	}
	byKey := map[key]*group{}
	var order []key
	for _, issue := range issues {
		k := key{issue.Severity, issue.Code}
		g, ok := byKey[k]
		if !ok {
			g = &group{severity: issue.Severity, code: issue.Code}
			byKey[k] = g
			order = append(order, k)
		}
		g.issues = append(g.issues, issue)
	}
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].severity != order[j].severity {
			return order[i].severity > order[j].severity // most severe first
		}
		return order[i].code < order[j].code
	})
	groups := make([]group, 0, len(order))
	for _, k := range order {
		groups = append(groups, *byKey[k])
	}
	return groups
}

// summaryTable emits the anytype table subtree: table → [columns layout,
// rows layout]; cell ids are `rowId-colId` with exactly one dash, so row and
// column ids themselves stay dash-free (ParseCellID splits on the first one).
func summaryTable(groups []group, dropped int64) []*model.Block {
	headers := []string{"Severity", "Issue", "Count"}
	columnIds := make([]string, len(headers))
	columns := make([]*model.Block, len(headers))
	for i := range headers {
		columnIds[i] = fmt.Sprintf("sumc%d", i)
		columns[i] = &model.Block{
			Id:      columnIds[i],
			Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}},
		}
	}

	rows := make([]*model.Block, 0, len(groups)+1)
	var cells []*model.Block
	makeRow := func(index int, header bool, values []string) {
		rowId := fmt.Sprintf("sumr%d", index)
		row := &model.Block{
			Id:      rowId,
			Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{IsHeader: header}},
		}
		for i, value := range values {
			cell := textBlock(rowId+"-"+columnIds[i], value, nil)
			row.ChildrenIds = append(row.ChildrenIds, cell.Id)
			cells = append(cells, cell)
		}
		rows = append(rows, row)
	}
	makeRow(0, true, headers)
	for i, g := range groups {
		makeRow(i+1, false, []string{g.severity.String(), string(g.code), fmt.Sprintf("%d", len(g.issues))})
	}
	if dropped > 0 {
		makeRow(len(groups)+1, false, []string{"", "(not recorded)", fmt.Sprintf("%d", dropped)})
	}

	columnsLayout := &model.Block{
		Id:          "summary-columns",
		Content:     &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}},
		ChildrenIds: ids(columns),
	}
	rowsLayout := &model.Block{
		Id:          "summary-rows",
		Content:     &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}},
		ChildrenIds: ids(rows),
	}
	table := &model.Block{
		Id:          "summary",
		Content:     &model.BlockContentOfTable{Table: &model.BlockContentTable{}},
		ChildrenIds: []string{columnsLayout.Id, rowsLayout.Id},
	}

	blocks := []*model.Block{table, columnsLayout, rowsLayout}
	blocks = append(blocks, columns...)
	blocks = append(blocks, rows...)
	blocks = append(blocks, cells...)
	return blocks
}

// toggleGroup emits one toggle per (severity, code) group with a line per
// issue nested under it.
func toggleGroup(groupIndex int, g group, resolve func(string) (string, bool)) []*model.Block {
	toggle := &model.Block{
		Id: fmt.Sprintf("group%d", groupIndex),
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:  fmt.Sprintf("%s — %s (%d)", g.severity, g.code, len(g.issues)),
			Style: model.BlockContentText_Toggle,
		}},
	}
	blocks := []*model.Block{toggle}
	for issueIndex, issue := range g.issues {
		line := issueLine(fmt.Sprintf("group%d-%d", groupIndex, issueIndex), issue, resolve)
		toggle.ChildrenIds = append(toggle.ChildrenIds, line.Id)
		blocks = append(blocks, line)
	}
	return blocks
}

// issueLine renders one issue as "sourceKey — message". When the source key
// resolves to a created object, the key's range gets a mention mark whose
// Param stays the SOURCE KEY — the persist-side resolver rewrites every mark
// param, and an unresolvable one would degrade to _missing_object, so the
// mark is only added when resolution is known to succeed.
func issueLine(blockId string, issue importv2.Issue, resolve func(string) (string, bool)) *model.Block {
	message := issue.Message
	if message == "" && issue.Err != nil {
		message = issue.Err.Error()
	}
	text := message
	var marks []*model.BlockContentTextMark
	if issue.SourceKey != "" {
		text = issue.SourceKey
		if message != "" {
			text += " — " + message
		}
		if resolvable(issue.SourceKey, resolve) {
			marks = append(marks, &model.BlockContentTextMark{
				Range: &model.Range{From: 0, To: int32(textutil.UTF16RuneCountString(issue.SourceKey))},
				Type:  model.BlockContentTextMark_Mention,
				Param: issue.SourceKey,
			})
		}
	}
	return textBlock(blockId, text, marks)
}

func resolvable(sourceKey string, resolve func(string) (string, bool)) bool {
	if resolve == nil {
		return false
	}
	id, ok := resolve(sourceKey)
	return ok && id != ""
}

func textBlock(id, text string, marks []*model.BlockContentTextMark) *model.Block {
	return &model.Block{
		Id: id,
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:  text,
			Style: model.BlockContentText_Paragraph,
			Marks: &model.BlockContentTextMarks{Marks: marks},
		}},
	}
}

func ids(blocks []*model.Block) []string {
	out := make([]string, len(blocks))
	for i, b := range blocks {
		out[i] = b.Id
	}
	return out
}
