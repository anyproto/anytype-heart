// Package typesuggest maps what a converter knows about a container of
// imported pages (a Notion database, a csv collection) onto a bundled
// Anytype object type: a database called "Tasks" whose rows would otherwise
// import as plain Pages becomes Task objects.
//
// The Suggestor seam is the contract; the naive implementation matches
// normalized container names against keyword tables and corroborates with
// property shapes. A learned model can replace it behind the same interface.
// Rules of the seam:
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

// CompletionNames are checkbox property names that read as task completion,
// as read by this suggestor. Widening it changes what the no-LLM default path
// does, so it stays as shipped — see MappingCompletionNames.
var CompletionNames = map[string]bool{
	"done": true, "complete": true, "completed": true, "finished": true, "checked": true,
}

// MappingCompletionNames is the wider set the schemaplan whitelist's `done`
// rule uses. It is a superset of CompletionNames, and the two deliberately differ
// because the stakes differ: Suggest infers a type for an ENTIRE database from
// one checkbox (a "Comments" table with a "Resolved" column would become Task,
// every row carrying the todo layout), while the mapping rule only routes ONE
// property onto the bundled done relation, checkbox→checkbox and
// format-preserving. The wider vocabulary is safe at the second stake and not
// at the first.
//
// "resolved" and "got it" are completion predicates on their own row
// ("Resolved?" on a ticket, "Got It?" on a grocery item). State flags like
// shipped/paid/sent are deliberately absent from both sets — mapping them to
// done marks an expense or an order as a finished task.
var MappingCompletionNames = func() map[string]bool {
	out := make(map[string]bool, len(CompletionNames)+2)
	for name := range CompletionNames {
		out[name] = true
	}
	out["resolved"] = true
	out["got it"] = true
	return out
}()

// dueNames are date property names that read as a task due date.
var dueNames = map[string]bool{
	"due": true, "due date": true, "deadline": true,
}

func (naive) Suggest(evidence Evidence) (Suggestion, bool) {
	if typeKey, ok := containerNames[Normalize(evidence.ContainerName)]; ok {
		return Suggestion{TypeKey: typeKey, Confidence: 0.9, Reason: "container name"}, true
	}
	var hasEmail, hasPhone, hasCompletionCheckbox, hasDueDate, hasStatus bool
	for _, property := range evidence.Properties {
		name := Normalize(property.Name)
		switch property.Format {
		case model.RelationFormat_email:
			hasEmail = true
		case model.RelationFormat_phone:
			hasPhone = true
		case model.RelationFormat_checkbox:
			hasCompletionCheckbox = hasCompletionCheckbox || CompletionNames[name]
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

// Normalize lowercases and keeps only letters, digits and single spaces, so
// "✅ To-Do" and "Tasks " both hit the table. Exported because the schemaplan
// whitelist normalizes property names with the same rules before matching
// them against CompletionNames and its due-date tokens.
func Normalize(name string) string {
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
