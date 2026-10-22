// Package report builds the per-run import report page — the primary user surface for the issue ledger. The engine persists
// it through the normal pool at the end of a run that produced issues; the
// notification and EventImportFinish carry its id so clients can open it and
// render a discard button. Output is deterministic (stable block ids, sorted
// groups) so golden tests stay stable.
//
// The page answers one question: what did NOT come over exactly, and where.
// A real Notion workspace produces around a thousand issues that say about a
// dozen distinct things — 435 of them "Notion did not return this block" —
// so the page groups by what happened and then by which object it happened
// to, rather than printing a line per occurrence. See "grouping" below.
package report

import (
	"fmt"
	"sort"
	"strings"

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

const (
	// maxObjectLines bounds one group's object list. A group that hit every
	// page of a big workspace is understood from its first lines and its
	// total; the rest is scrolling.
	maxObjectLines = 25
	// maxSubjects bounds the named things listed on one object's line.
	maxSubjects = 4
	// maxGroups bounds how many kinds the page spells out. One kind per issue
	// is a real shape — an error string carries the file it failed on, so a
	// hundred failures are a hundred messages — and without a bound the
	// summary table grows a row for each. The rest fold into one line that
	// still counts them.
	maxGroups = 40
	// maxUnnamedLines is how many unresolvable objects are worth a line each.
	// A source key that resolves to nothing is a Notion id: it cannot be
	// opened, searched or recognised, so past a couple of them the count and
	// what they were about say more than the ids do.
	maxUnnamedLines = 2
)

// Source is what the run knows about a source key when the report is built:
// the display name of the object it became, and whether it resolves at all.
// An unresolved key (a page that failed, a database that became nothing) is
// rendered as plain text — a mention mark pointing nowhere degrades to
// _missing_object in the client.
type Source struct {
	Name     string
	Resolved bool
}

// Lookup answers Source for a source key. nil is allowed and means "nothing
// resolves": every object is rendered by its key.
type Lookup func(sourceKey string) Source

func (l Lookup) get(sourceKey string) Source {
	if l == nil {
		return Source{}
	}
	return l(sourceKey)
}

// Build renders the issue ledger as a page object: a lead paragraph, a
// summary table (what happened × how many × how many objects), then one
// toggle per kind listing the objects it affected.
func Build(title string, issues []importv2.Issue, dropped int64, lookup Lookup) *importv2.Object {
	groups := groupIssues(issues)

	shown, folded := groups, []group(nil)
	if len(shown) > maxGroups {
		shown, folded = groups[:maxGroups], groups[maxGroups:]
	}

	var blocks []*model.Block
	blocks = append(blocks, textBlock("intro", intro(groups), nil))
	blocks = append(blocks, summaryTable(shown, folded, dropped)...)
	for groupIndex, group := range shown {
		blocks = append(blocks, toggleGroup(groupIndex, group, lookup)...)
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

// affected is one object a group touched, with everything named inside it.
type affected struct {
	sourceKey string
	count     int
	subjects  []string
	seen      map[string]bool
}

// group is one (severity, message) bucket: every issue that says the same
// thing, whichever object it happened to. The message is the grouping key
// rather than the code, because one code covers many unrelated causes —
// dataLoss alone spans skipped properties, images without a URL and people
// the integration may not read.
type group struct {
	severity importv2.Severity
	code     importv2.IssueCode
	message  string
	count    int
	objects  []*affected
	byKey    map[string]*affected
	// runWide holds the issues with no source key (run diagnostics).
	runWide int
	// subjects is every distinct subject in the group, in first-seen order.
	// When there is exactly one, it belongs in the heading rather than
	// repeated down the whole list ("— button" on 55 consecutive lines).
	subjects []string
	seen     map[string]bool
}

func groupIssues(issues []importv2.Issue) []group {
	type key struct {
		severity importv2.Severity
		message  string
	}
	byKey := map[key]*group{}
	var order []key
	for _, issue := range issues {
		message := messageOf(issue)
		k := key{issue.Severity, message}
		g, ok := byKey[k]
		if !ok {
			g = &group{severity: issue.Severity, code: issue.Code, message: message,
				byKey: map[string]*affected{}, seen: map[string]bool{}}
			byKey[k] = g
			order = append(order, k)
		}
		occurrences := issue.Occurrences()
		g.count += occurrences
		if issue.Subject != "" && !g.seen[issue.Subject] {
			g.seen[issue.Subject] = true
			g.subjects = append(g.subjects, issue.Subject)
		}
		if issue.SourceKey == "" {
			g.runWide += occurrences
			continue
		}
		object, ok := g.byKey[issue.SourceKey]
		if !ok {
			object = &affected{sourceKey: issue.SourceKey, seen: map[string]bool{}}
			g.byKey[issue.SourceKey] = object
			g.objects = append(g.objects, object)
		}
		object.count += occurrences
		if issue.Subject != "" && !object.seen[issue.Subject] {
			object.seen[issue.Subject] = true
			object.subjects = append(object.subjects, issue.Subject)
		}
	}

	// Worst first: severity, then reach. Ties break on the message so the
	// page is byte-stable across runs with the same ledger.
	sort.SliceStable(order, func(i, j int) bool {
		a, b := byKey[order[i]], byKey[order[j]]
		if a.severity != b.severity {
			return a.severity > b.severity
		}
		if a.count != b.count {
			return a.count > b.count
		}
		return a.message < b.message
	})
	groups := make([]group, 0, len(order))
	for _, k := range order {
		g := byKey[k]
		sort.SliceStable(g.objects, func(i, j int) bool {
			if g.objects[i].count != g.objects[j].count {
				return g.objects[i].count > g.objects[j].count
			}
			return g.objects[i].sourceKey < g.objects[j].sourceKey
		})
		groups = append(groups, *g)
	}
	return groups
}

func messageOf(issue importv2.Issue) string {
	if issue.Message != "" {
		return issue.Message
	}
	if issue.Err != nil {
		return issue.Err.Error()
	}
	return string(issue.Code)
}

// outcome is the severity in the reader's terms. "warning" and "objectError"
// describe the ledger; a person wants to know whether their content arrived.
func outcome(severity importv2.Severity) string {
	switch severity {
	case importv2.SeverityInfo:
		return "Note"
	case importv2.SeverityWarning:
		return "Imported with changes"
	default:
		return "Not imported"
	}
}

// intro counts what went wrong separately from what merely happened: a note
// ("these rows became Tasks") is not a thing that failed to come over, and
// adding it to the headline number would overstate the damage.
func intro(groups []group) string {
	var problems, notes, objects int
	seen := map[string]bool{}
	for _, g := range groups {
		if g.severity <= importv2.SeverityInfo {
			notes += g.count
			continue
		}
		problems += g.count
		for _, object := range g.objects {
			if !seen[object.sourceKey] {
				seen[object.sourceKey] = true
				objects++
			}
		}
	}
	switch {
	case problems == 0 && notes == 0:
		return "Everything imported as it is in the source."
	case problems == 0:
		return "Everything imported as it is in the source. The notes below say what the importer decided along the way."
	}
	text := fmt.Sprintf("%s in %s did not come over exactly. Everything not listed here imported normally.",
		plural(problems, "thing", "things"), plural(objects, "object", "objects"))
	if notes > 0 {
		text += fmt.Sprintf(" Plus %s about what the importer decided, which are not problems.", plural(notes, "note", "notes"))
	}
	return text
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// summaryTable emits the anytype table subtree: table → [columns layout,
// rows layout]; cell ids are `rowId-colId` with exactly one dash, so row and
// column ids themselves stay dash-free (ParseCellID splits on the first one).
func summaryTable(groups, folded []group, dropped int64) []*model.Block {
	headers := []string{"Result", "What happened", "Times", "Objects"}
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
		what := g.message
		if len(g.subjects) == 1 {
			what = fmt.Sprintf("%s (%s)", what, g.subjects[0])
		}
		objects := fmt.Sprintf("%d", len(g.objects))
		if len(g.objects) == 0 {
			objects = "—" // a run-wide note is about the import, not an object
		}
		makeRow(i+1, false, []string{outcome(g.severity), what, fmt.Sprintf("%d", g.count), objects})
	}
	if len(folded) > 0 {
		occurrences := 0
		for _, g := range folded {
			occurrences += g.count
		}
		makeRow(len(rows), false, []string{"", fmt.Sprintf("%d more kinds of issue", len(folded)),
			fmt.Sprintf("%d", occurrences), "—"})
	}
	if dropped > 0 {
		makeRow(len(rows), false, []string{"", "(not recorded)", fmt.Sprintf("%d", dropped), ""})
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

// toggleGroup emits one toggle per kind of issue, holding a line per object
// it affected.
func toggleGroup(groupIndex int, g group, lookup Lookup) []*model.Block {
	heading := g.message
	// One subject for the whole group is a fact about the group.
	sharedSubject := len(g.subjects) == 1
	if sharedSubject {
		heading = fmt.Sprintf("%s (%s)", heading, g.subjects[0])
	}
	if g.count > 1 {
		heading = fmt.Sprintf("%s — %d", heading, g.count)
	}
	style := model.BlockContentText_Toggle
	if len(g.objects) == 0 {
		// Nothing to unfold: an issue about the run itself (a search that
		// stopped early, a plan that failed) is the whole statement.
		style = model.BlockContentText_Paragraph
	}
	toggle := &model.Block{
		Id: fmt.Sprintf("group%d", groupIndex),
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:  heading,
			Style: style,
		}},
	}
	blocks := []*model.Block{toggle}
	add := func(line *model.Block) {
		toggle.ChildrenIds = append(toggle.ChildrenIds, line.Id)
		blocks = append(blocks, line)
	}

	named, unnamed := partitionByName(g.objects, lookup)
	shown := named
	if len(shown) > maxObjectLines {
		shown = shown[:maxObjectLines]
	}
	for index, object := range shown {
		add(objectLine(fmt.Sprintf("group%d-%d", groupIndex, index), object, lookup, sharedSubject))
	}
	if rest := len(named) - len(shown); rest > 0 {
		add(textBlock(fmt.Sprintf("group%d-more", groupIndex), fmt.Sprintf("…and %d more", rest), nil))
	}
	if len(unnamed) > 0 {
		add(unnamedLine(fmt.Sprintf("group%d-unnamed", groupIndex), unnamed, sharedSubject))
	}
	return blocks
}

// partitionByName splits a group's objects into the ones the report can name
// and the ones it cannot. A handful of unnameable objects still get a line
// each — their key is the only handle anyone has — but a crowd of them
// becomes a count.
func partitionByName(objects []*affected, lookup Lookup) (named, unnamed []*affected) {
	for _, object := range objects {
		if lookup.get(object.sourceKey).Name != "" {
			named = append(named, object)
		} else {
			unnamed = append(unnamed, object)
		}
	}
	if len(unnamed) <= maxUnnamedLines {
		return append(named, unnamed...), nil
	}
	return named, unnamed
}

// unnamedLine is the one line that stands for every object the report cannot
// name: how many there were, and what they were about.
func unnamedLine(blockId string, objects []*affected, subjectInHeading bool) *model.Block {
	occurrences := 0
	var subjects []string
	seen := map[string]bool{}
	for _, object := range objects {
		occurrences += object.count
		for _, subject := range object.subjects {
			if !seen[subject] {
				seen[subject] = true
				subjects = append(subjects, subject)
			}
		}
	}
	text := plural(len(objects), "object with no name", "objects with no name")
	if occurrences > len(objects) {
		text += fmt.Sprintf(", %d times", occurrences)
	}
	if !subjectInHeading {
		if list := subjectList(subjects); list != "" {
			text += " — " + list
		}
	}
	return textBlock(blockId, text, nil)
}

// objectLine renders one affected object: its name, how many times it was
// hit, and what inside it was affected. The name carries a mention mark whose
// Param stays the SOURCE KEY — the persist-side resolver rewrites every mark
// param, and an unresolvable one would degrade to _missing_object, so the
// mark is only added when resolution is known to succeed.
func objectLine(blockId string, object *affected, lookup Lookup, subjectInHeading bool) *model.Block {
	source := lookup.get(object.sourceKey)
	label := source.Name
	if label == "" {
		label = object.sourceKey
	}
	text := label
	if object.count > 1 {
		text += fmt.Sprintf(" (%d)", object.count)
	}
	if !subjectInHeading {
		// A subject that repeats the object's own name says it twice
		// ("Launch Tracker — Launch Tracker").
		subjects := make([]string, 0, len(object.subjects))
		for _, subject := range object.subjects {
			if subject != label {
				subjects = append(subjects, subject)
			}
		}
		if list := subjectList(subjects); list != "" {
			text += " — " + list
		}
	}
	var marks []*model.BlockContentTextMark
	if source.Resolved {
		marks = append(marks, &model.BlockContentTextMark{
			Range: &model.Range{From: 0, To: int32(textutil.UTF16RuneCountString(label))},
			Type:  model.BlockContentTextMark_Mention,
			Param: object.sourceKey,
		})
	}
	return textBlock(blockId, text, marks)
}

func subjectList(subjects []string) string {
	if len(subjects) == 0 {
		return ""
	}
	shown := subjects
	if len(shown) > maxSubjects {
		shown = shown[:maxSubjects]
	}
	out := strings.Join(shown, ", ")
	if rest := len(subjects) - len(shown); rest > 0 {
		out += fmt.Sprintf(", +%d more", rest)
	}
	return out
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
