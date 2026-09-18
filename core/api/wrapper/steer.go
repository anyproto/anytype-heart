package wrapper

// steer.go — the one place an argument that is wrong in a DIAGNOSABLE way
// earns its repair, and the one place the server's REST-shaped repair hints
// are re-spelled in the tool vocabulary.
//
// The rule both mechanisms implement (APIV2.md §8.34): a refusal that is
// correct and unactionable is a defect. Three live small-model runs have now
// produced the same failure — the model gets a true "not found", cannot see
// what to change, and re-sends the identical call until the turn budget ends
// the attempt. Every instance was one recognisable wrong shape with one known
// repair, so the wrapper detects the shape and names the repair.
//
// Both passes run on Run's error path rather than inside an executor: the
// mistake is a property of the ARGUMENT, not of the tool, so every tool
// taking that argument is covered by construction and a new tool inherits the
// steer without a line of code.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

//
// ---- the diagnosis table ----
//

// argSteer diagnoses one argument. repair returns the sentence to append, or
// "" to leave the server's refusal exactly as it was written.
type argSteer struct {
	arg string
	// supersedes is the operation whose generic hint the specific repair
	// makes redundant: every issue whose references name it loses its hint
	// when the repair fires. Two repairs in one refusal compete, and the run
	// that produced §8.34 shows which one loses: the model had already run
	// `spaces`, been served the full ids, and gone straight back to the
	// truncated form.
	supersedes string
	repair     func(r *Runner, ctx context.Context, def Tool, session *Session, ref string) string
}

// argSteers is the table. Adding an argument whose mis-shapes are
// recognisable means adding a row here — not a hook in an executor.
var argSteers = []argSteer{
	{arg: "object", repair: objectRefRepair},
	{arg: "space", supersedes: v2model.OpListSpaces, repair: spaceRefRepair},
}

// steerError is Run's error path. It re-spells the server's REST hints for
// every failure, and appends an argument repair when the refusal is a
// not-found naming a value the caller itself supplied.
//
// The repair is APPENDED, never substituted: the server's 404 is the fact
// (which reference failed), the hint is the repair. It fires only AFTER the
// server has refused, so nothing has to be proven impossible up front — the
// 404 has already established the value is not a valid one, and the cost is
// one HTTP round trip on a mistake.
func (r *Runner) steerError(ctx context.Context, def Tool, session *Session, args map[string]any, err error) error {
	if err == nil {
		return nil
	}
	var te *ToolError
	if !errors.As(err, &te) {
		return err
	}
	var repairs []string
	if te.Status == http.StatusNotFound {
		for _, s := range argSteers {
			if _, takes := def.arg(s.arg); !takes {
				continue
			}
			ref := strArg(args, s.arg)
			// only when the server's message names the caller's OWN value: a
			// not-found about anything else is a different failure and must
			// not be re-explained as a bad argument
			if ref == "" || !strings.Contains(te.Text, strconv.Quote(ref)) {
				continue
			}
			if repair := s.repair(r, ctx, def, session, ref); repair != "" {
				if s.supersedes != "" {
					dropHintsNaming(te, s.supersedes)
				}
				repairs = append(repairs, repair)
			}
		}
	}
	// the vocabulary pass runs after the steers have dropped what they
	// supersede, and the repairs are appended to the re-spelled text
	deRest(te)
	for _, repair := range repairs {
		te.Text += " — " + repair
	}
	return te
}

// dropHintsNaming blanks the hint of every issue whose references name op —
// the generic repair a specific one supersedes — on the issue and in the
// rendered text. It runs before deRest, while the references are still on
// the issues.
func dropHintsNaming(te *ToolError, op string) {
	for i, issue := range te.Issues {
		for _, ref := range issue.SeeAlso {
			if ref.Op == op {
				te.Text = strings.Replace(te.Text, " ("+issue.Hint+")", "", 1)
				te.Issues[i].Hint = ""
				te.Issues[i].SeeAlso = nil
				break
			}
		}
	}
}

//
// ---- `object` (§8.33 defect 2) ----
//

// blockRefRe recognises a reference from the BLOCK vocabulary: a served
// block label (a hex suffix, 5 chars and up — anyblockjson's
// compactIdMinLen) or a full minted block/row/column id (24 hex). Object ids
// are CIDs, `_bundled` keys or participant ids and are never pure hex, so a
// value of this shape in `object` is a category error, not a typo.
var blockRefRe = regexp.MustCompile(`^[0-9a-f]{5,24}$`)

// handleRepair is the sentence every mis-shaped `object` ends on: where the
// numbers come from. It is the phrasing errNoSession already uses, because
// one repair should read the same wherever it is offered.
const handleRepair = "`object` takes a handle number from the last find (1, 2, …)"

// spaceInObjectRepair is the tail shared by the two ways a space id lands in
// `object` — whole, and truncated at the dot (§8.34).
const spaceInObjectRepair = "find searches inside a space and numbers the objects it matches. " + handleRepair

// minSpacePrefixLen bounds the space-prefix check below. Every object and
// space id here is a CIDv1 in the same multibase and codec, so any two of
// them share a leading run (`bafyrei…`) about eight characters long; a
// shorter match would call a truncated OBJECT id a space id.
const minSpacePrefixLen = 16

// isSpaceIdPrefix reports whether ref is the space id with its tail cut off
// — the §8.34 truncation, arriving in `object` instead of `space`.
func isSpaceIdPrefix(spaceId, ref string) bool {
	return spaceId != "" && ref != spaceId && len(ref) >= minSpacePrefixLen && strings.HasPrefix(spaceId, ref)
}

// objectRefRepair names the repair for a reference that is not an object.
func objectRefRepair(_ *Runner, _ context.Context, def Tool, session *Session, ref string) string {
	switch {
	case handleRe.MatchString(ref):
		// a handle resolved to an id before the call, so a 404 quoting the
		// handle itself is not this mistake
		return ""
	case ref == session.Space:
		return "that is the space id, not an object: " + spaceInObjectRepair
	case isSpaceIdPrefix(session.Space, ref):
		return fmt.Sprintf("that is the start of the space id %q, not an object: ", session.Space) + spaceInObjectRepair
	case blockRefRe.MatchString(ref):
		where := "a block reference belongs in a block argument, not in `object`"
		if _, ok := def.arg("block"); ok {
			where = "that is a block reference: read serves those, and they go in `block`"
		}
		return where + ". " + handleRepair
	default:
		return handleRepair + "; to address an object by name, run find with query naming it"
	}
}

//
// ---- `space` (§8.34) ----
//

// steerSpaceListLimit bounds the space list the space repair reads. A caller
// with more spaces than this gets the server's plain refusal — the steer is a
// repair for a recognisable mistake, not a search.
const steerSpaceListLimit = 100

// maxSteerSpaceMatches bounds how many candidate ids the repair spells out.
const maxSteerSpaceMatches = 3

// spaceRefRepair names the repair for the mistake a live `gemma4:e4b` run
// made on 74 of 79 find calls: a space id has two dot-joined parts
// (`bafyrei….28y6mgnwgodt7`) and the model passed only the part before the
// dot, plausibly reading the rest as a file extension. It called `spaces`,
// received the full ids, and went straight back to the truncated form — so
// the refusal has to name the mistake, not the tool that lists the ids.
//
// A rejected space id that is a PREFIX of a real one is exactly that mistake
// and its repair is the full id, which the wrapper can read. Anything else
// gets the server's refusal unchanged.
func spaceRefRepair(r *Runner, ctx context.Context, _ Tool, _ *Session, ref string) string {
	matches := r.spaceIdsWithPrefix(ctx, ref)
	if len(matches) == 0 {
		return ""
	}
	const rule = "a space id has two parts joined by a dot and BOTH are part of the id — pass it whole, exactly as `spaces` prints it"
	if len(matches) == 1 {
		return fmt.Sprintf("that is the first part of the space id %q: %s", matches[0], rule)
	}
	if len(matches) > maxSteerSpaceMatches {
		matches = matches[:maxSteerSpaceMatches]
	}
	return fmt.Sprintf("that is the first part of several space ids (%s): %s", strings.Join(matches, ", "), rule)
}

// spaceIdsWithPrefix returns the known space ids the rejected value is a
// strict prefix of. A failure to list is not an error the caller should see:
// the steer is best-effort on top of a refusal that already stands.
func (r *Runner) spaceIdsWithPrefix(ctx context.Context, prefix string) []string {
	if prefix == "" {
		return nil
	}
	var resp v2model.ListResponse[v2model.SpaceRow]
	err := r.client.decode(ctx, apiRequest{
		method: "GET",
		path:   "/v2/spaces",
		query:  url.Values{"limit": []string{strconv.Itoa(steerSpaceListLimit)}},
	}, &resp)
	if err != nil {
		return nil
	}
	var out []string
	for _, row := range resp.Data {
		if row.Id != prefix && strings.HasPrefix(row.Id, prefix) {
			out = append(out, row.Id)
		}
	}
	return out
}

//
// ---- the REST → tool vocabulary (§8.34) ----
//

// spacesListRepair is the tool-shaped spelling of the server's own
// space-not-found hint, named because the space steer supersedes it.
const spacesListRepair = "list spaces with " + spacesToolSpelling

const spacesToolSpelling = "the `spaces` tool"

// toolVocab re-spells the server's repair hints for a caller that has tools
// and no routes. The server names an operation in REST on the HTTP surface —
// that IS its vocabulary — but a tool-calling model handed "list spaces
// with GET /v2/spaces" is told to do something it cannot do, while the tool
// that does it (`spaces`) goes unnamed. The run that produced §8.34 shows
// the cost: the model had already called `spaces` and the hint sent it
// nowhere.
//
// The server ships every route it names as a typed reference beside the
// prose (Issue.SeeAlso, model/ref.go), keyed by OpenAPI operationId, and the
// prose spells the reference exactly as Ref.String renders it. So this is a
// lookup, not a regex table: for each reference, the REST spelling is found
// in the hint and replaced with the row below. An operation with no row is
// one this tool set does not offer, and the fallback says so by name — the
// caller learns the repair exists and is not on this surface, instead of
// guessing at a route. A hint added server-side tomorrow is translated
// without a change here, as long as it names an operation this table knows.
//
// Every row is a NOUN PHRASE: the server's sentences supply the verb ("list
// keys with %s", "%s lists them", "use %s"), so a row that carries its own
// predicate collides with theirs. A row must also promise only what the tool
// does: there is no type listing here, so the type-list row says where types
// are visible rather than pretending `find` lists them.
var toolVocab = map[string]func(ref v2model.Ref) string{
	v2model.OpListSpaces: func(v2model.Ref) string { return spacesToolSpelling },
	v2model.OpListTypes: func(v2model.Ref) string {
		return "a type listing (not in this tool set; `find` results show each object's type)"
	},
	v2model.OpGetType:        func(v2model.Ref) string { return "`describe`" },
	v2model.OpCreateType:     func(v2model.Ref) string { return "`create_type`" },
	v2model.OpListProperties: func(v2model.Ref) string { return "`describe` on the type" },
	v2model.OpListPropertyOptions: func(ref v2model.Ref) string {
		if key := ref.Params["key"]; key != "" {
			return "`describe` with options=" + key
		}
		return "`describe` with options naming the property"
	},
	// the object read; ?outline=true is the outline mode, and ?ids=full is a
	// shape `read` cannot ask for (it serves compact labels)
	v2model.OpGetObject: func(ref v2model.Ref) string {
		if ref.Query["ids"] == "full" {
			return "a full-id read (not in this tool set)"
		}
		if ref.Query["outline"] == "true" {
			return "`read` with mode=outline"
		}
		return "`read`"
	},
	// query and collection rows are read by `read` on the list object
	v2model.OpGetQueryObjects:      func(v2model.Ref) string { return "`read` on the query" },
	v2model.OpGetCollectionObjects: func(v2model.Ref) string { return "`read` on the collection" },
	v2model.OpPatchObject:          func(v2model.Ref) string { return "the editing tools of this tool set" },
	v2model.OpSearchSpace:          func(v2model.Ref) string { return "`find`" },
	v2model.OpListObjects:          func(v2model.Ref) string { return "`find`" },
}

// toolSpelling renders one typed reference in the tool vocabulary. A
// resend reference (no op) is the parameter the server wants on the same
// request, spelled bare; an operation without a row is named as outside
// this tool set.
func toolSpelling(ref v2model.Ref) string {
	if ref.Op == "" {
		return strings.TrimPrefix(ref.String(), "?")
	}
	if spell, ok := toolVocab[ref.Op]; ok {
		return spell(ref)
	}
	return fmt.Sprintf("an operation outside this tool set (%s)", ref.Op)
}

// restRoute catches any REST route the references above do not cover — a
// hint from a server build older than the typed references, or one written
// as a bare route. The replacement is deliberately a plain noun phrase: it
// reads grammatically wherever a route can appear in a sentence, and it
// says the true thing, which is that the repair is not on this surface. A
// test asserts nothing route-shaped survives this pass.
//
// A dot is part of the route only when route characters follow it — real
// space ids are dotted (`bafyreiabc.28y6mgnwgodt7`), and the earlier
// `[^\s,;.)]*` stopped at the id's dot, leaving its tail glued to the
// replacement ("the HTTP API.28y6mgnwgodt7/properties"). A sentence-ending
// dot is still excluded: `\.` must be followed by at least one route
// character to match.
var restRoute = regexp.MustCompile(`(?:GET|POST|PATCH|PUT|DELETE) /v[0-9]+[^\s,;.)]*(?:\.[^\s,;.)]+)*`)

const restRouteFallback = "the HTTP API"

// deRestText re-spells the references in s: each reference's REST rendering
// becomes its tool spelling, and the text between the renderings falls to
// restRoute. The two never touch each other's output: the renderings are
// located in ONE pass over the original text (a regexp alternation, longest
// first, so a rendering that extends another — the outline read extends
// the plain read — wins where both match), and only the gaps between them
// see the catch-all, so a tool spelling that happens to contain something
// route-shaped (an option name, say) is not redacted into "the HTTP API".
func deRestText(s string, refs []v2model.Ref) string {
	if s == "" {
		return s
	}
	spellings := make(map[string]string, len(refs))
	patterns := make([]string, 0, len(refs))
	for _, ref := range refs {
		rest := ref.String()
		if rest == "" {
			continue
		}
		if _, seen := spellings[rest]; !seen {
			patterns = append(patterns, rest)
			spellings[rest] = toolSpelling(ref)
		}
	}
	if len(patterns) == 0 {
		return restRoute.ReplaceAllString(s, restRouteFallback)
	}
	sort.SliceStable(patterns, func(i, j int) bool { return len(patterns[i]) > len(patterns[j]) })
	for i, p := range patterns {
		patterns[i] = regexp.QuoteMeta(p)
	}
	spans := regexp.MustCompile(strings.Join(patterns, "|"))
	var b strings.Builder
	last := 0
	for _, m := range spans.FindAllStringIndex(s, -1) {
		b.WriteString(restRoute.ReplaceAllString(s[last:m[0]], restRouteFallback))
		b.WriteString(spellings[s[m[0]:m[1]]])
		last = m[1]
	}
	b.WriteString(restRoute.ReplaceAllString(s[last:], restRouteFallback))
	return b.String()
}

// deRestIssue re-spells one issue's hint by its own references and drops
// the references, which have been rendered. The message is a fact and names
// no operation (the references are the hint's, by contract), so it only
// gets the catch-all — a quoted value in it that happens to look like a
// route rendering must not be rewritten into a tool name.
func deRestIssue(issue v2model.Issue) v2model.Issue {
	issue.Message = deRestText(issue.Message, nil)
	issue.Hint = deRestText(issue.Hint, issue.SeeAlso)
	issue.SeeAlso = nil
	return issue
}

// deRest rewrites a ToolError in place — issues, message and text. The
// references rewrite HINTS only, and the text — rendered from the same
// issues, then possibly edited by an executor that re-spelled op paths into
// its own argument names or appended what was and was not written — is
// rewritten by substituting each hint as a whole: the old hint is a span of
// the text, and it becomes the re-spelled one, so an executor's edits
// around it survive and the message part of the text is never touched by a
// reference. The catch-all then covers the rest of the text and the message.
func deRest(te *ToolError) {
	for i := range te.Issues {
		before := te.Issues[i].Hint
		te.Issues[i] = deRestIssue(te.Issues[i])
		if before != "" && te.Issues[i].Hint != before {
			te.Text = strings.ReplaceAll(te.Text, before, te.Issues[i].Hint)
		}
	}
	te.Message = deRestText(te.Message, nil)
	te.Text = deRestText(te.Text, nil)
}

// warningsText renders success-path warnings for the tool surface, hint
// included: a warning's repair lives in its hint (the P0 query and
// collection reads put it there), and a renderer that prints the message
// alone leaves the caller with the fact and no fix.
func warningsText(warnings []v2model.Issue) string {
	var b strings.Builder
	for _, w := range warnings {
		w = deRestIssue(w)
		b.WriteString("\nwarning: ")
		b.WriteString(w.Message)
		if w.Hint != "" {
			b.WriteString(" — ")
			b.WriteString(w.Hint)
		}
	}
	return b.String()
}
