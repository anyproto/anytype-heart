package v2service

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/anyproto/any-block/codec/anyblockjson"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// docissues.go turns the format's validation issues into the C6 shape and
// attaches the repair each recurring one needs. The format speaks for the
// document; this layer knows which endpoint took it (the schema kind the
// caller can read) and adds that. Round-two eval F6, F9 and F10 were all the
// same finding: a verdict that names the fault but not the fix costs the
// caller a round trip per fault, and the fix was one literal away.

// formatVersionRequired is the one legal value, spelled once.
var formatVersionRequired = fmt.Sprintf(`include "formatVersion":%q`, anyblockjson.FormatVersion)

// missingMemberIssue matches the validator's "missing property 'x'" verdict.
var missingMemberIssue = regexp.MustCompile(`^missing property '([^']+)'$`)

// keyMemberNotAllowed is the format's verdict on a `key` member — its prose
// explains the format's own history and cancels itself ("spelled property
// in every structure … spelled it key earlier"); the repair is one sentence.
const keyMemberNotAllowed = `property "key" is not allowed`

// documentIssues maps the format's issues onto v2 issues for a document the
// `kind` schema describes ("object", "type"; "" for a fragment nothing
// documents whole).
func documentIssues(kind string, issues []anyblockjson.Issue) []v2model.Issue {
	out := make([]v2model.Issue, 0, len(issues))
	// the validator states a one-of-three identity requirement on a type's
	// definition as three "missing property" verdicts at one path — a caller
	// who obeys all three writes three members where one was asked for
	missingAt := map[string][]string{}
	for _, issue := range issues {
		if m := missingMemberIssue.FindStringSubmatch(issue.Message); m != nil {
			missingAt[issue.Path] = append(missingAt[issue.Path], m[1])
		}
	}
	collapsed := map[string]bool{}
	for _, issue := range issues {
		v := v2model.Issue{Path: issue.Path, Message: issue.Message}
		switch {
		case issue.Path == "/formatVersion" && strings.HasPrefix(issue.Message, "formatVersion "):
			if strings.Contains(issue.Message, "newer than") {
				break
			}
			if kind == "" {
				v = v.WithHint(v2model.Plain(formatVersionRequired))
			} else {
				v = v.Hintf("%s — %s shows the document shape", formatVersionRequired, v2model.RefGetSchema(kind))
			}
		case strings.HasPrefix(issue.Message, keyMemberNotAllowed):
			v.Message = keyMemberNotAllowed + ` — the member that names a property is spelled "property"; rename the member and keep its value`
			v = schemaRef(v, kind)
		case len(missingAt[issue.Path]) > 1 && missingMemberIssue.MatchString(issue.Message):
			if collapsed[issue.Path] {
				continue
			}
			collapsed[issue.Path] = true
			members := missingAt[issue.Path]
			v.Message = fmt.Sprintf("missing property: one of %s is required", strings.Join(quoteAll(members), ", "))
			if isTypeDefinitionPath(issue.Path) {
				v.Message = `missing property — a definition names its property: give "name" (its display name; an unknown one is created) or "property" (a key the space serves)`
			}
			v = schemaRef(v, kind)
		case strings.Contains(issue.Message, "is not allowed"):
			v = schemaRef(v, kind)
		}
		out = append(out, v)
	}
	return out
}

// schemaRef points an issue at the schema of the document's kind, when one
// documents it whole.
func schemaRef(v v2model.Issue, kind string) v2model.Issue {
	if kind == "" || v.Hint != "" {
		return v
	}
	return v.Hintf("%s shows the members this document takes", v2model.RefGetSchema(kind))
}

func isTypeDefinitionPath(path string) bool {
	return strings.HasPrefix(path, "/type_settings/property_definitions/") || strings.HasPrefix(path, "/property_definitions/")
}

// rebaseIssuePaths rewrites every issue path of a v2 error through fn, so a
// refusal addresses the request the caller sent rather than the document
// the endpoint built from it.
func rebaseIssuePaths(err error, fn func(string) string) error {
	var v2Err *v2model.Error
	if !errors.As(err, &v2Err) {
		return err
	}
	for i := range v2Err.Issues {
		v2Err.Issues[i].Path = fn(v2Err.Issues[i].Path)
	}
	return v2Err
}

// typeDefinitionMemberIssues catches the two guesses a type definition
// invites before the format sees them: `key` for the member that names the
// property, and `type` for its format. The format refuses `key` on its own,
// but prunes `type` as unreliable beside the other fault — so a caller was
// told to rename one member and nothing about the other, dropped it, and
// every field silently became text (F10). Both are named here, at the path
// the caller sent them.
func typeDefinitionMemberIssues(definitions []map[string]any, pathPrefix, kind string) []v2model.Issue {
	var issues []v2model.Issue
	for i, def := range definitions {
		at := fmt.Sprintf("%s/%d", pathPrefix, i)
		_, hasKey := def["key"]
		_, hasProperty := def["property"]
		_, hasName := def["name"]
		if hasKey && !hasProperty && !hasName {
			issues = append(issues, v2model.Issue{
				Path:    at + "/key",
				Message: keyMemberNotAllowed + ` — the member that names a property is spelled "property"; rename the member and keep its value`,
			})
		}
		_, hasType := def["type"]
		_, hasFormat := def["format"]
		if hasType && !hasFormat {
			issues = append(issues, v2model.Issue{
				Path:    at + "/type",
				Message: `property "type" is not allowed — a definition's value kind is spelled "format" (text, number, select, multi_select, date, checkbox, url, email, phone, objects, files); rename the member and keep its value`,
			})
		}
	}
	for i := range issues {
		issues[i] = schemaRef(issues[i], kind)
	}
	return issues
}

// typeBodyKind names the schema kind that documents a type body: the flat
// body is kind type, the interchange document kind type_document — the two
// differ in every member, so a repair pointing at the wrong one contradicts
// itself.
func typeBodyKind(flat bool) string {
	if flat {
		return "type"
	}
	return "type_document"
}

// rawTypeDefinitions decodes a type body's property_definitions as sent,
// for the member pre-scan; nil when absent or not an array.
func rawTypeDefinitions(fields map[string]json.RawMessage) []map[string]any {
	raw, ok := fields["type_settings"]
	if !ok {
		return nil
	}
	var settings struct {
		Definitions []map[string]any `json:"property_definitions"`
	}
	if json.Unmarshal(raw, &settings) != nil {
		return nil
	}
	return settings.Definitions
}
