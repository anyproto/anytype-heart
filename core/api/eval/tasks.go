package eval

// tasks.go: the Phase-0 task fixtures (APIV2.md §2 Phase 0 task set). Each
// edit task carries a starting AnyBlock document plus a forward instruction
// and its inverse, so the DELEGATE-52 backtranslation (forward + inverse,
// then ScoreCorruption against the untouched document) is runnable as soon
// as an edit method exists. Create tasks start from nothing and are scored
// on apply-success only.

import (
	"embed"
	"fmt"
)

//go:embed testdata/*.json
var taskDocs embed.FS

// Task is one benchmark task of the harness's fixed set.
type Task struct {
	// Name identifies the task in reports.
	Name string
	// Instruction is the model-facing forward edit/create instruction.
	Instruction string
	// Inverse undoes Instruction — the DELEGATE-52 backtranslation pair.
	// Empty for create tasks, which have nothing to invert.
	Inverse string
	// InitialDoc is the AnyBlock JSON document the task starts from; nil
	// for create-from-scratch tasks.
	InitialDoc []byte
}

// IsEditTask reports whether the task participates in backtranslation
// scoring (has a starting document and an inverse instruction).
func (t Task) IsEditTask() bool {
	return len(t.InitialDoc) > 0 && t.Inverse != ""
}

func mustDoc(name string) []byte {
	data, err := taskDocs.ReadFile("testdata/" + name)
	if err != nil {
		panic(fmt.Sprintf("embedded task fixture %s: %v", name, err))
	}
	return data
}

// Tasks returns the fixed Phase-0 task set: append paragraph · edit one
// word · toggle a checkbox · restructure a section · fill a table cell
// (set_cell) · create task with properties · build a set with filter.
func Tasks() []Task {
	return []Task{
		{
			Name:        "append-paragraph",
			Instruction: "Append a paragraph reading \"Reviewed by the team.\" at the end of the document.",
			Inverse:     "Delete the paragraph reading \"Reviewed by the team.\" at the end of the document.",
			InitialDoc:  mustDoc("append-paragraph.json"),
		},
		{
			Name:        "edit-one-word",
			Instruction: "In the paragraph about the launch, change the word \"Q3\" to \"Q4\".",
			Inverse:     "In the paragraph about the launch, change the word \"Q4\" back to \"Q3\".",
			InitialDoc:  mustDoc("edit-one-word.json"),
		},
		{
			Name:        "toggle-checkbox",
			Instruction: "Mark the \"Ship the release\" checkbox as done.",
			Inverse:     "Mark the \"Ship the release\" checkbox as not done.",
			InitialDoc:  mustDoc("toggle-checkbox.json"),
		},
		{
			Name:        "restructure-section",
			Instruction: "Move the \"Risks\" section (its heading and paragraph) above the \"Plan\" section.",
			Inverse:     "Move the \"Risks\" section (its heading and paragraph) back below the \"Plan\" section.",
			InitialDoc:  mustDoc("restructure-section.json"),
		},
		{
			Name:        "fill-table-cell",
			Instruction: "In the feature status table, set the Status cell of the Export row to \"in progress\".",
			Inverse:     "In the feature status table, clear the Status cell of the Export row.",
			InitialDoc:  mustDoc("fill-table-cell.json"),
		},
		{
			Name:        "create-task-with-properties",
			Instruction: "Create a task named \"Water the plants\" with status \"Todo\" and a due date of next Friday.",
		},
		{
			Name:        "build-set-with-filter",
			Instruction: "Create a set of tasks filtered to done = false, sorted by due date ascending.",
		},
	}
}
