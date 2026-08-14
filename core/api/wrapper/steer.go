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
	// supersedes is a generic hint the specific repair makes redundant, cut
	// from the message when the repair fires. Two repairs in one refusal
	// compete, and the run that produced §8.34 shows which one loses: the
	// model had already run `spaces`, been served the full ids, and gone
	// straight back to the truncated form.
	supersedes string
	repair     func(r *Runner, ctx context.Context, def Tool, session *Session, ref string) string
}

// argSteers is the table. Adding an argument whose mis-shapes are
// recognisable means adding a row here — not a hook in an executor.
var argSteers = []argSteer{
	{arg: "object", repair: objectRefRepair},
	{arg: "space", supersedes: spacesListRepair, repair: spaceRefRepair},
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
	deRest(te)
	if te.Status != http.StatusNotFound {
		return te
	}
	for _, s := range argSteers {
		if _, takes := def.arg(s.arg); !takes {
			continue
		}
		ref := strArg(args, s.arg)
		// only when the server's message names the caller's OWN value: a
		// not-found about anything else is a different failure and must not
		// be re-explained as a bad argument
		if ref == "" || !strings.Contains(te.Text, strconv.Quote(ref)) {
			continue
		}
		if repair := s.repair(r, ctx, def, session, ref); repair != "" {
			if s.supersedes != "" {
				te.Text = strings.Replace(te.Text, " — "+s.supersedes, "", 1)
			}
			te.Text += " — " + repair
		}
	}
	return te
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
const spacesListRepair = "list them with the `spaces` tool"

// restVocab re-spells the server's repair hints for a caller that has tools
// and no routes. The server is right to name the route on the HTTP surface —
// that IS its vocabulary — but a tool-calling model handed
// "list spaces with GET /v2/spaces" is told to do something it cannot do,
// while the tool that does it (`spaces`) goes unnamed. The run that produced
// §8.34 shows the cost: the model had already called `spaces` and the hint
// sent it nowhere.
//
// The space id in these patterns is interpolated by the server, so the
// patterns match it as a segment; some hints ship the literal `{spaceId}`
// placeholder and those match too.
var restVocab = []struct {
	from *regexp.Regexp
	to   string
}{
	{regexp.MustCompile(`list spaces with GET /v2/spaces\b`), spacesListRepair},
	{regexp.MustCompile(`list (?:all|keys|available keys) with GET /v2/spaces/[^/\s]+/types\b`),
		"check the type key (find results show each object's type)"},
	{regexp.MustCompile(`list all with GET /v2/spaces/[^/\s]+/properties, or create it with POST /v2/spaces/[^/\s]+/properties\b`),
		"run describe on the type to list the property keys it takes"},
	{regexp.MustCompile(`check the names against GET /v2/spaces/[^/\s]+/properties/[^/\s]+/options\b`),
		"check the names against describe, which lists the live option names"},
	// the removed-property refusal (§8.41): the remove-the-key repair works
	// verbatim on this surface; only the list-route tail needs the tool
	// spelling
	{regexp.MustCompile(`for a different property, list them with GET /v2/spaces/[^/\s]+/properties\b`),
		"for a different property, run describe on the type to list its live property keys"},
	// the removed-type refusal (§8.41)
	{regexp.MustCompile(`use a live type instead — list them with GET /v2/spaces/[^/\s]+/types\b`),
		"use a live type instead (find results show each object's type)"},
	{regexp.MustCompile(`GET the object with \?outline=true to list (?:them|block ids)\b`),
		"run read with mode=outline to list the block labels"},
	{regexp.MustCompile(`list members with GET /v2/spaces/[^/\s]+/members\b`),
		"the tool set has no member listing"},
}

// restRoute catches any REST route the vocabulary above does not name — a
// hint added server-side tomorrow, on a route the wrapper calls today. The
// replacement is deliberately a plain noun phrase: it reads grammatically
// wherever a route can appear in a sentence, and it says the true thing,
// which is that the repair is not on this surface. A test asserts nothing
// route-shaped survives this pass.
//
// A dot is part of the route only when route characters follow it — real
// space ids are dotted (`bafyreiabc.28y6mgnwgodt7`), and the earlier
// `[^\s,;.)]*` stopped at the id's dot, leaving its tail glued to the
// replacement ("the HTTP API.28y6mgnwgodt7/properties"). A sentence-ending
// dot is still excluded: `\.` must be followed by at least one route
// character to match.
var restRoute = regexp.MustCompile(`(?:GET|POST|PATCH|PUT|DELETE) /v[0-9]+[^\s,;.)]*(?:\.[^\s,;.)]+)*`)

const restRouteFallback = "the HTTP API"

// deRest rewrites a ToolError in place — text, issue messages and issue
// hints, since the text is built from all three.
func deRest(te *ToolError) {
	rewrite := func(s string) string {
		if s == "" {
			return s
		}
		for _, sub := range restVocab {
			s = sub.from.ReplaceAllString(s, sub.to)
		}
		return restRoute.ReplaceAllString(s, restRouteFallback)
	}
	te.Text = rewrite(te.Text)
	for i := range te.Issues {
		te.Issues[i].Message = rewrite(te.Issues[i].Message)
		te.Issues[i].Hint = rewrite(te.Issues[i].Hint)
	}
}
