// Package typesuggest maps what a converter knows about a container of
// imported pages (a Notion database, a csv collection) onto a bundled
// Anytype object type: a database called "Tasks" whose rows would otherwise
// import as plain Pages becomes Task objects.
//
// The Suggestor seam is the contract; the naive implementation matches
// normalized container names against keyword tables and corroborates with
// property shapes. A learned model can replace it behind the same interface
// (design doc §11.5). Rules of the seam:
//   - suggestions only ever fill the default-Page gap — an explicit type
//     (front matter, schema) always wins, enforced by the callers;
//   - implementations return only suggestions confident enough to APPLY,
//     never "maybes" (there is no suggestion UI in the import flow);
//   - output must be deterministic for identical evidence.
package typesuggest

import (
	"strings"
	"unicode"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Property is one schema property of the container, when the source exposes
// a schema (Notion databases do; csv collections carry names only).
type Property struct {
	Name   string
	Format model.RelationFormat
}

// Evidence is everything a converter knows about a container of pages.
type Evidence struct {
	ContainerName string // database title / csv collection title, id-stripped
	Properties    []Property
}

// Suggestion is an adoptable type verdict.
type Suggestion struct {
	TypeKey    domain.TypeKey
	Confidence float64 // 0..1; naive rules use fixed per-rule scores
	Reason     string  // short human phrase for the observability issue
}

// Suggestor decides the object type for a container's pages. ok=false means
// no confident suggestion — the pages keep their default type.
type Suggestor interface {
	Suggest(evidence Evidence) (Suggestion, bool)
}

// NewNaive returns the keyword/shape rule implementation.
func NewNaive() Suggestor {
	return naive{}
}

type naive struct{}

// containerNames maps a normalized container name to a bundled type. Exact
// matches only — plural forms are listed, not derived. Deliberately
// conservative: a wrong type on every row of a database is worse than Page.
var containerNames = map[string]domain.TypeKey{
	"task": bundle.TypeKeyTask, "tasks": bundle.TypeKeyTask,
	"todo": bundle.TypeKeyTask, "todos": bundle.TypeKeyTask,
	"to do": bundle.TypeKeyTask, "to dos": bundle.TypeKeyTask,
	"action item": bundle.TypeKeyTask, "action items": bundle.TypeKeyTask,
	"ticket": bundle.TypeKeyTask, "tickets": bundle.TypeKeyTask,
	"issue": bundle.TypeKeyTask, "issues": bundle.TypeKeyTask,
	"backlog": bundle.TypeKeyTask,
	"chore":   bundle.TypeKeyTask, "chores": bundle.TypeKeyTask,

	"project": bundle.TypeKeyProject, "projects": bundle.TypeKeyProject,
	"initiative": bundle.TypeKeyProject, "initiatives": bundle.TypeKeyProject,
	"epic": bundle.TypeKeyProject, "epics": bundle.TypeKeyProject,

	"contact": bundle.TypeKeyContact, "contacts": bundle.TypeKeyContact,
	"people": bundle.TypeKeyContact, "person": bundle.TypeKeyContact,
	"client": bundle.TypeKeyContact, "clients": bundle.TypeKeyContact,
	"customer": bundle.TypeKeyContact, "customers": bundle.TypeKeyContact,
	"vendor": bundle.TypeKeyContact, "vendors": bundle.TypeKeyContact,
	"employee": bundle.TypeKeyContact, "employees": bundle.TypeKeyContact,
	"staff":       bundle.TypeKeyContact,
	"team member": bundle.TypeKeyContact, "team members": bundle.TypeKeyContact,

	"note": bundle.TypeKeyNote, "notes": bundle.TypeKeyNote,
	"memo": bundle.TypeKeyNote, "memos": bundle.TypeKeyNote,
	"quick note": bundle.TypeKeyNote, "quick notes": bundle.TypeKeyNote,

	"journal": bundle.TypeKeyDiaryEntry, "diary": bundle.TypeKeyDiaryEntry,
	"daily note": bundle.TypeKeyDiaryEntry, "daily notes": bundle.TypeKeyDiaryEntry,
	"daily log": bundle.TypeKeyDiaryEntry, "daily logs": bundle.TypeKeyDiaryEntry,

	"goal": bundle.TypeKeyGoal, "goals": bundle.TypeKeyGoal,
	"objective": bundle.TypeKeyGoal, "objectives": bundle.TypeKeyGoal,
	"okr": bundle.TypeKeyGoal, "okrs": bundle.TypeKeyGoal,

	"book": bundle.TypeKeyBook, "books": bundle.TypeKeyBook,
	"reading list": bundle.TypeKeyBook, "bookshelf": bundle.TypeKeyBook,

	"movie": bundle.TypeKeyMovie, "movies": bundle.TypeKeyMovie,
	"film": bundle.TypeKeyMovie, "films": bundle.TypeKeyMovie,

	"recipe": bundle.TypeKeyRecipe, "recipes": bundle.TypeKeyRecipe,
	"cookbook":  bundle.TypeKeyRecipe,
	"meal plan": bundle.TypeKeyRecipe, "meal plans": bundle.TypeKeyRecipe,
}

// completionNames are checkbox property names that read as task completion.
var completionNames = map[string]bool{
	"done": true, "complete": true, "completed": true, "finished": true, "checked": true,
}

// dueNames are date property names that read as a task due date.
var dueNames = map[string]bool{
	"due": true, "due date": true, "deadline": true,
}

func (naive) Suggest(evidence Evidence) (Suggestion, bool) {
	if typeKey, ok := containerNames[normalize(evidence.ContainerName)]; ok {
		return Suggestion{TypeKey: typeKey, Confidence: 0.9, Reason: "container name"}, true
	}
	var hasEmail, hasPhone, hasCompletionCheckbox, hasDueDate, hasStatus bool
	for _, property := range evidence.Properties {
		name := normalize(property.Name)
		switch property.Format {
		case model.RelationFormat_email:
			hasEmail = true
		case model.RelationFormat_phone:
			hasPhone = true
		case model.RelationFormat_checkbox:
			hasCompletionCheckbox = hasCompletionCheckbox || completionNames[name]
		case model.RelationFormat_date:
			hasDueDate = hasDueDate || dueNames[name]
		case model.RelationFormat_status:
			hasStatus = true
		}
	}
	switch {
	case hasEmail && hasPhone:
		return Suggestion{TypeKey: bundle.TypeKeyContact, Confidence: 0.8, Reason: "email and phone properties"}, true
	case hasCompletionCheckbox:
		return Suggestion{TypeKey: bundle.TypeKeyTask, Confidence: 0.75, Reason: "completion checkbox"}, true
	case hasDueDate && hasStatus:
		return Suggestion{TypeKey: bundle.TypeKeyTask, Confidence: 0.7, Reason: "due date and status properties"}, true
	}
	return Suggestion{}, false
}

// normalize lowercases and keeps only letters, digits and single spaces, so
// "✅ To-Do" and "Tasks " both hit the table.
func normalize(name string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastSpace = false
		case !lastSpace:
			b.WriteRune(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}
