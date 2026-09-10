package wrapper

// values.go — the wrapper-handler conveniences §7.3 places here: `@me`
// sentinel resolution (through the server-side GET /members/me identity),
// relative-date input resolution (wrapper math; the REST value path stays
// literal), and the A2 option-name pre-validation guard — the REST
// primitives create missing select option names by design (R9/§8.1), so
// the small-tier guard sits in FRONT of them.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

// meSentinel is the caller-identity placeholder accepted in property values
// and filter strings.
const meSentinel = "@me"

// meFor resolves the caller's participant id in a space, cached in the
// session.
func (r *Runner) meFor(ctx context.Context, session *Session, spaceId string) (string, error) {
	if id, ok := session.Me[spaceId]; ok {
		return id, nil
	}
	var row v2model.MemberRow
	err := r.client.decode(ctx, apiRequest{
		method: "GET",
		path:   "/v2/spaces/" + seg(spaceId) + "/members/me",
	}, &row)
	if err != nil {
		return "", fmt.Errorf("resolve @me: %w", err)
	}
	if session.Me == nil {
		session.Me = map[string]string{}
	}
	session.Me[spaceId] = row.Id
	return row.Id, nil
}

// resolveFilterMe substitutes the quoted "@me" value in a filter string
// with the caller's participant id. Purely textual — the sentinel is only
// meaningful as a quoted value, which is the one place the token can occur.
func (r *Runner) resolveFilterMe(ctx context.Context, session *Session, spaceId, filter string) (string, error) {
	if !strings.Contains(filter, `"`+meSentinel+`"`) {
		return filter, nil
	}
	me, err := r.meFor(ctx, session, spaceId)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(filter, `"`+meSentinel+`"`, `"`+me+`"`), nil
}

//
// ---- property formats ----
//

// maxPropertyPages bounds the format-index pagination loop.
const maxPropertyPages = 10

// propertyFormatsPageSize keeps a healthy margin under the server's
// MaxPageSize (currently 1000): an over-limit `limit` is a hard 400, so
// sitting exactly on the boundary would turn every create/set_properties
// into a 400 the day the constant is lowered.
const propertyFormatsPageSize = 500

// propertyRows loads the space's property rows, following C10 pagination.
func (r *Runner) propertyRows(ctx context.Context, spaceId string) ([]v2model.PropertyRow, error) {
	var rows []v2model.PropertyRow
	offset := 0
	for page := 0; page < maxPropertyPages; page++ {
		var resp v2model.ListResponse[v2model.PropertyRow]
		err := r.client.decode(ctx, apiRequest{
			method: "GET",
			path:   "/v2/spaces/" + seg(spaceId) + "/properties",
			query:  url.Values{"limit": []string{strconv.Itoa(propertyFormatsPageSize)}, "offset": []string{strconv.Itoa(offset)}},
		}, &resp)
		if err != nil {
			return nil, fmt.Errorf("list properties: %w", err)
		}
		rows = append(rows, resp.Data...)
		if !resp.HasMore {
			break
		}
		offset += len(resp.Data)
	}
	return rows, nil
}

// propertyIndex is the space's property identity index for tool arguments:
// formats keyed by the served api key (the row spelling the option and
// format routes take), plus a FoldKeyTerm fold-class index over BOTH the api
// key and the display name. The wrapper teaches names (D5), so a model
// echoes "Due date" as readily as due_date or the dueDate guess — all three
// must land on one row, and the class that meets several rows must refuse.
type propertyIndex struct {
	formats map[string]string
	byFold  map[string][]v2model.PropertyRow
	// names maps api key → display name, because every message this layer
	// publishes speaks names (D5): a refusal about `status` must say
	// "Status", the spelling the surface taught.
	names map[string]string
}

// displayName spells a resolved api key in the published vocabulary — the
// row's display name, or the key itself when the space names it no better.
func (idx *propertyIndex) displayName(key string) string {
	if name := idx.names[key]; name != "" {
		return name
	}
	return key
}

// foldClasses returns the distinct non-empty FoldKeyTerm classes of the
// given spellings.
func foldClasses(spellings ...string) []string {
	var out []string
	for _, s := range spellings {
		class := anyblockjson.FoldKeyTerm(s)
		if class == "" || containsStr(out, class) {
			continue
		}
		out = append(out, class)
	}
	return out
}

// newPropertyIndex builds the index from the space's property rows.
func newPropertyIndex(rows []v2model.PropertyRow) *propertyIndex {
	idx := &propertyIndex{
		formats: make(map[string]string, len(rows)),
		byFold:  map[string][]v2model.PropertyRow{},
		names:   make(map[string]string, len(rows)),
	}
	for _, row := range rows {
		idx.formats[row.Key] = row.Format
		idx.names[row.Key] = row.Name
		for _, class := range foldClasses(row.Key, row.Name) {
			held := false
			for _, existing := range idx.byFold[class] {
				if existing.Key == row.Key {
					held = true
					break
				}
			}
			if !held {
				idx.byFold[class] = append(idx.byFold[class], row)
			}
		}
	}
	return idx
}

// propertyIndexFor loads the space's property rows and builds the index —
// once per tool call (a set_properties with set+add+remove must not fetch
// the listing three times).
func (r *Runner) propertyIndexFor(ctx context.Context, spaceId string) (*propertyIndex, error) {
	rows, err := r.propertyRows(ctx, spaceId)
	if err != nil {
		return nil, err
	}
	return newPropertyIndex(rows), nil
}

// selectFormats are the formats whose values are option NAMES the guard
// checks.
var selectFormats = map[string]bool{"select": true, "multi_select": true}

// prepareValues resolves @me and relative dates in a property-value map and
// — when guard is set — pre-validates select option names (the A2 guard).
// Keys resolve through the index first (§8.21): the resolved api key drives
// the format lookup, so "Due date": "friday" — or the DueDate guess — gets
// its date resolution too. Unknown keys pass through untouched: the server's
// referential layer owns key validation and its did-you-mean texts. idx is
// the space's property index (propertyIndexFor), loaded ONCE per tool call —
// a set_properties with set+add+remove must not fetch the listing three
// times.
func (r *Runner) prepareValues(ctx context.Context, session *Session, spaceId string, idx *propertyIndex, values map[string]any, guard bool) (map[string]any, error) {
	out := make(map[string]any, len(values))
	// sorted: every refusal below picks the FIRST offending key it meets, and
	// over a Go map that was a different message per run on the surface an
	// agent reads back. A tool that says something different each time about
	// the same body cannot be debugged from the transcript.
	for _, key := range sortedValueKeys(values) {
		value := values[key]
		foldedKey, err := idx.resolveKey(key)
		if err != nil {
			return nil, err
		}
		if _, dup := out[foldedKey]; dup {
			// the refusal names the property as the surface spells it — its
			// display name — not the resolved api key the caller never sent
			return nil, fmt.Errorf("property %q is given more than once (several spellings resolve to one property) — pass it once", idx.displayName(foldedKey))
		}
		format := idx.formats[foldedKey]
		resolved, err := r.resolveValue(ctx, session, spaceId, format, value)
		if err != nil {
			return nil, fmt.Errorf("value of %q: %w", key, err)
		}
		if guard && selectFormats[format] && !r.AllowNewOptions {
			if err := r.checkOptionNames(ctx, spaceId, foldedKey, idx.displayName(foldedKey), stringEntries(resolved)); err != nil {
				return nil, err
			}
		}
		out[foldedKey] = resolved
	}
	return out, nil
}

//
// ---- key folding (§8.21 / D5) ----
//
// The live benchmark's dominant argument error was a naming guess — "Page"
// for type page, "Name" for property name — never a structural violation.
// C2 strictness (a key is a key) is the REST surface's contract and stays;
// the wrapper is the layer that exists to be forgiving for small models, so
// it folds spellings on the way in. The one hard rule: if two properties
// answer to one fold class, refuse naming both — never guess.
//
// The fold is the FORMAT's fold (anyblockjson.FoldKeyTerm: NFC, casefold,
// `_`/`-`, whitespace and invisible code points stripped), not the api
// surface's narrower bundle.FoldApiKey. It has to be: the wrapper teaches
// display names now (D5 — its reads request ?keys=name), and a display name
// separates its words with SPACES where the api key uses `_` — a fold that
// kept spaces would put "Due date" and `due_date` in different classes. The
// server resolves either spelling anyway (D3), so an unfolded key still
// lands; what silently stops without the fold is everything the wrapper does
// with the key's FORMAT, which it looks up by the resolved api key — the
// relative-date convenience and the option guard. Loud on the server, silent
// in the layer whose whole job is forgiveness.

// resolveKey resolves one inbound property spelling: an exact api key wins;
// otherwise the input's FoldKeyTerm class — which meets a row through its
// key or its display name — resolves when ONE row holds it; a class held by
// several rows refuses listing them, unless the input is exactly one row's
// display name (a name is a better address than a fold guess); a class
// nobody holds passes the input through for the server's did-you-mean.
func (idx *propertyIndex) resolveKey(key string) (string, error) {
	if _, ok := idx.formats[key]; ok {
		return key, nil
	}
	rows := idx.byFold[anyblockjson.FoldKeyTerm(key)]
	switch len(rows) {
	case 0:
		return key, nil
	case 1:
		return rows[0].Key, nil
	}
	exact := ""
	for _, row := range rows {
		if row.Name != key {
			continue
		}
		if exact != "" {
			exact = ""
			break // two rows share the exact name — back to the refusal
		}
		exact = row.Key
	}
	if exact != "" {
		return exact, nil
	}
	spellings, keyed := describeRowSpellings(rows)
	return "", fmt.Errorf("property %q matches several properties (%s) — %s",
		key, strings.Join(spellings, ", "), ambiguityRepair(keyed))
}

// ambiguityRepair is the tail of every several-match refusal: names are the
// vocabulary this surface teaches, so the repair speaks names — and only
// when a candidate had to fall back to its key (keyed) is the key offered,
// because a shared or missing name leaves nothing else to address by.
func ambiguityRepair(keyed bool) string {
	if keyed {
		return "use the exact name, or the key in parentheses"
	}
	return "use the exact name"
}

// candidateSpelling renders one ambiguity candidate in the NAME vocabulary:
// the quoted display name when it is unique among the candidates; the key
// alone when the row has no name (or its name IS its key); name plus key
// when several candidates share one name — the name then addresses nothing,
// and the key is the one spelling that still resolves (values.go step 1),
// so withholding it would make the refusal unactionable. keyed reports that
// fallback so the caller's repair sentence can name it.
func candidateSpelling(name, key string, nameCount map[string]int) (string, bool) {
	switch {
	case name == "" || name == key:
		return key, false
	case nameCount[name] > 1:
		return fmt.Sprintf("%q (key %s)", name, key), true
	default:
		return fmt.Sprintf("%q", name), false
	}
}

// describeRowSpellings renders ambiguity candidates name-first (see
// candidateSpelling).
func describeRowSpellings(rows []v2model.PropertyRow) ([]string, bool) {
	nameCount := map[string]int{}
	for _, row := range rows {
		nameCount[row.Name]++
	}
	out := make([]string, 0, len(rows))
	keyed := false
	for _, row := range rows {
		spelling, k := candidateSpelling(row.Name, row.Key, nameCount)
		out = append(out, spelling)
		keyed = keyed || k
	}
	sort.Strings(out)
	return out, keyed
}

// maxTypePages bounds the type-index pagination loop.
const maxTypePages = 4

// typeRows lists the space's type rows — key AND name, because the fold's
// reference set spans both spellings now (D5).
func (r *Runner) typeRows(ctx context.Context, spaceId string) ([]v2model.TypeRow, error) {
	var rows []v2model.TypeRow
	offset := 0
	for page := 0; page < maxTypePages; page++ {
		var resp v2model.ListResponse[v2model.TypeRow]
		err := r.client.decode(ctx, apiRequest{
			method: "GET",
			path:   "/v2/spaces/" + seg(spaceId) + "/types",
			query:  url.Values{"limit": []string{strconv.Itoa(propertyFormatsPageSize)}, "offset": []string{strconv.Itoa(offset)}},
		}, &resp)
		if err != nil {
			return nil, fmt.Errorf("list types: %w", err)
		}
		rows = append(rows, resp.Data...)
		if !resp.HasMore {
			break
		}
		offset += len(resp.Data)
	}
	return rows, nil
}

// isTypeNotFound reports whether err is the server's type-key miss for
// typeKey — every route that takes a type key says `type "<key>" not
// found` (describe's GET answers 404 not_found, create and find answer
// 400 validation_failed).
func isTypeNotFound(err error, typeKey string) bool {
	var te *ToolError
	return errors.As(err, &te) && strings.Contains(te.Text, fmt.Sprintf("type %q not found", typeKey))
}

// foldTypeArg is the fold retry gate for the tools that send a type key
// (find, describe, create): after a failed first attempt it reports the
// resolved key to retry with — only when err is the server's not-found for
// exactly that value and a UNIQUE fold-class variant exists among the
// space's type rows. The class spans key AND display name (FoldKeyTerm), so
// a model quoting the name a read served — "Task" for `task` — self-heals
// exactly like the case guess did. The fold runs on the error path, so the
// correct-key common case never pays a type listing. A fold collision is
// its own refusal (an exact display name still cuts through it); a listing
// failure or a fold miss keeps the original (candidate-bearing) error.
func (r *Runner) foldTypeArg(ctx context.Context, spaceId, typeKey string, err error) (string, bool, error) {
	if typeKey == "" || !isTypeNotFound(err, typeKey) {
		return "", false, nil
	}
	rows, listErr := r.typeRows(ctx, spaceId)
	if listErr != nil {
		return "", false, nil // best-effort: the original error stands
	}
	fold := anyblockjson.FoldKeyTerm(typeKey)
	var matches []v2model.TypeRow
	for _, row := range rows {
		if containsStr(foldClasses(row.Key, row.Name), fold) {
			held := false
			for _, existing := range matches {
				if existing.Key == row.Key {
					held = true
					break
				}
			}
			if !held {
				matches = append(matches, row)
			}
		}
	}
	switch {
	case len(matches) == 1:
		if matches[0].Key == typeKey {
			return "", false, nil // the fold answers the failed spelling itself
		}
		return matches[0].Key, true, nil
	case len(matches) > 1:
		exact := ""
		for _, row := range matches {
			if row.Name != typeKey {
				continue
			}
			if exact != "" {
				exact = ""
				break // two rows share the exact name — refuse below
			}
			exact = row.Key
		}
		if exact != "" && exact != typeKey {
			return exact, true, nil
		}
		spellings, keyed := describeTypeRowSpellings(matches)
		return "", false, fmt.Errorf("type %q matches several types (%s) — %s",
			typeKey, strings.Join(spellings, ", "), ambiguityRepair(keyed))
	default:
		return "", false, nil
	}
}

// describeTypeRowSpellings is describeRowSpellings for type rows.
func describeTypeRowSpellings(rows []v2model.TypeRow) ([]string, bool) {
	nameCount := map[string]int{}
	for _, row := range rows {
		nameCount[row.Name]++
	}
	out := make([]string, 0, len(rows))
	keyed := false
	for _, row := range rows {
		spelling, k := candidateSpelling(row.Name, row.Key, nameCount)
		out = append(out, spelling)
		keyed = keyed || k
	}
	sort.Strings(out)
	return out, keyed
}

// typeNameIndex loads the space's type display names (key → name),
// best-effort: the text channel spells types by NAME (the wrapper's
// vocabulary — a user calls the `set` type "Query"), but prettier text is
// never worth failing a call that already succeeded, so a listing failure
// degrades to nil and the caller falls back to the key.
func (r *Runner) typeNameIndex(ctx context.Context, spaceId string) map[string]string {
	rows, err := r.typeRows(ctx, spaceId)
	if err != nil {
		return nil
	}
	names := make(map[string]string, len(rows))
	for _, row := range rows {
		if row.Name != "" {
			names[row.Key] = row.Name
		}
	}
	return names
}

// typeLabel spells a type key in the published vocabulary: the display name
// when the index knows one, the key itself otherwise (an unnamed type has no
// other spelling — its key is its name in every practical sense).
func typeLabel(names map[string]string, key string) string {
	if name := names[key]; name != "" {
		return name
	}
	return key
}

// objectRefFormats are the property formats whose values are OBJECT
// REFERENCES — where an id is expected, so @me substitutes and a handle or
// a name resolves. On any other format those spellings are DATA (a
// description literally containing "@me", a text property whose value is
// "2", a name that happens to match another object's name must all stay
// exactly what the caller wrote).
var objectRefFormats = map[string]bool{"objects": true}

// resolveValue rewrites one value: object references on object-format keys
// (@me, a handle number, an exact object name — see resolveObjectRefValue),
// relative dates on date-format keys.
func (r *Runner) resolveValue(ctx context.Context, session *Session, spaceId, format string, value any) (any, error) {
	switch v := value.(type) {
	case string:
		if objectRefFormats[format] {
			if v == meSentinel {
				return r.meFor(ctx, session, spaceId)
			}
			return r.resolveObjectRefValue(ctx, session, spaceId, v)
		}
		if format == "date" {
			if abs, ok := resolveRelativeDate(v, r.now()); ok {
				return abs, nil
			}
		}
		return v, nil
	case []any:
		out := make([]any, len(v))
		for i, entry := range v {
			resolved, err := r.resolveValue(ctx, session, spaceId, format, entry)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	default:
		return value, nil
	}
}

//
// ---- object-reference values ----
//
// Measured: on cross-object work (read from A, write to B) small models
// fail at ADDRESSING the second object, not at reasoning about it —
// gemma-4-e4b scored 1/4 where bonsai-27b scored 4/4, and the step that
// broke was copying a full object id out of one call's output into the next
// call's argument. Every other reference channel on this surface already
// takes something a model can retype: `object` takes a handle number,
// `block` takes a 5-char label, `space` takes the row `spaces` printed. An
// objects-format PROPERTY value was the last slot that took nothing but the
// id, so this is the same repair where it was still missing.

// maxObjectNameCandidates bounds the name lookup's page — and with it the
// candidate list an ambiguity refusal prints.
const maxObjectNameCandidates = 10

// resolveObjectRefValue resolves one objects-format value, in order:
//
//  1. an id the session has already handed out (a find result, a create, a
//     list read) wins over every other reading — an id IS the address, and
//     this resolution must never re-point a caller who already got it right.
//  2. a handle number from the last find — the same numbers `object` takes,
//     so one number means one thing on every slot.
//  3. an exact object name in the space: ONE match resolves; several REFUSE,
//     listing what to pass instead (the rule matchSpaceRef, resolveKey and
//     resolveTablePart already share — ambiguity is never guessed); none
//     passes the caller's value through untouched, which leaves the server's
//     own not-found refusal exactly as it was.
//
// Step 3 is EXACT, deliberately unlike the fold the key and type resolvers
// apply (§8.21). Those fold against a listing they already hold; an object
// name has no listing — it is a store query — and the only case-insensitive
// query available is a substring match, whose PAGE can crowd the exact match
// out behind ten near-misses and silently resolve nothing. The equality
// lookup used here cannot miss, and it costs one request that a value which
// is already an id skips entirely.
func (r *Runner) resolveObjectRefValue(ctx context.Context, session *Session, spaceId, ref string) (string, error) {
	// the lookup ignores surrounding whitespace, as spaceArg does with the
	// row it accepts back; what survives an UNRESOLVED lookup is the
	// original string, because an id has to reach the server byte for byte
	name := strings.TrimSpace(ref)
	if name == "" || spaceId == "" {
		return ref, nil
	}
	if _, known := session.handleFor(spaceId, name); known {
		return name, nil // rule 1: an id this session itself served
	}
	if handleRe.MatchString(name) {
		space, id, err := r.resolveObject(session, name, "")
		if err != nil {
			return "", err
		}
		// a handle is the last find's numbering and resolves through the
		// session's space; using it in ANOTHER space would store a reference
		// to whatever that number means somewhere it was never assigned —
		// resolveObject's own rule, restated at the value slot
		if space != spaceId {
			return "", fmt.Errorf("handle %s is from the last find in space %q, but this object lives in %q — pass the object's id, or run find in %q first",
				name, space, spaceId, spaceId)
		}
		return id, nil
	}
	rows, err := r.objectsNamedExactly(ctx, spaceId, name)
	if err != nil {
		// best-effort, like every other convenience in this file: a lookup
		// that could not run must not fail a write the server would have
		// accepted — an id needs no lookup at all
		return ref, nil
	}
	switch len(rows) {
	case 1:
		return rows[0].Id, nil
	case 0:
		return ref, nil
	default:
		return "", r.ambiguousObjectValueError(ctx, session, spaceId, name, rows)
	}
}

// objectsNamedExactly lists the space's objects whose name is exactly name.
// The lookup is a search with an EQUALITY filter, not a full-text query:
// full text ranks and stems, and what this resolution promises is an exact
// name. Each row's name is re-tested here because the promise is the
// wrapper's to keep, not the store's.
func (r *Runner) objectsNamedExactly(ctx context.Context, spaceId, name string) ([]v2model.ObjectRow, error) {
	var resp v2model.ListResponse[v2model.ObjectRow]
	err := r.client.decode(ctx, apiRequest{
		method: "POST",
		path:   "/v2/spaces/" + seg(spaceId) + "/search",
		query:  url.Values{"limit": []string{strconv.Itoa(maxObjectNameCandidates)}, "keys": []string{"name"}},
		body:   map[string]any{"filter": "name = " + quoteFilterValue(name)},
	}, &resp)
	if err != nil {
		return nil, fmt.Errorf("look up objects named %q: %w", name, err)
	}
	out := make([]v2model.ObjectRow, 0, len(resp.Data))
	for _, row := range resp.Data {
		if row.Name == name {
			out = append(out, row)
		}
	}
	return out, nil
}

// filterValueEscaper renders the four escapes the compact filter grammar
// defines (\" \\ \n \t) and no others — strconv.Quote would additionally
// emit \xNN and \uNNNN forms the filter lexer does not accept, turning a
// name with an unusual character into a parse error instead of a lookup.
var filterValueEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`)

// quoteFilterValue renders a string as a quoted filter-grammar value.
func quoteFilterValue(s string) string { return `"` + filterValueEscaper.Replace(s) + `"` }

// ambiguousObjectValueError refuses a name several objects answer to. The
// candidates are spelled in what the caller can actually retype: the handle
// number when the last find numbered the object (the cheapest address on
// this surface), the id otherwise, each with its type name so the choice is
// makeable without a second read.
func (r *Runner) ambiguousObjectValueError(ctx context.Context, session *Session, spaceId, name string, rows []v2model.ObjectRow) error {
	// best-effort, text-only (find's rule): a failed type listing spells
	// each candidate's type by key rather than failing the refusal
	typeNames := r.typeNameIndex(ctx, spaceId)
	candidates := make([]string, 0, len(rows))
	for _, row := range rows {
		label := typeLabel(typeNames, row.Type)
		if n, ok := session.handleFor(spaceId, row.Id); ok {
			candidates = append(candidates, fmt.Sprintf("handle %d (%s)", n, label))
			continue
		}
		candidates = append(candidates, fmt.Sprintf("%s (%s)", row.Id, label))
	}
	return fmt.Errorf("%q names %d objects in this space (%s) — pass one of those instead: a handle number addresses what the last find numbered, an id addresses the object outright",
		name, len(rows), strings.Join(candidates, ", "))
}

// stringEntries flattens a value into its option-name strings.
func stringEntries(value any) []string {
	switch v := value.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		var out []string
		for _, entry := range v {
			if s, ok := entry.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// checkOptionNames verifies each name exists as an option of the property —
// this tool never creates options; the REST surface's create-missing (R9)
// stays available behind AllowNewOptions. key is the resolved api key the
// option route takes; label is the property's display name, because that is
// how every message here spells the property (D5).
func (r *Runner) checkOptionNames(ctx context.Context, spaceId, key, label string, names []string) error {
	for _, name := range names {
		var resp v2model.ListResponse[v2model.OptionRow]
		err := r.client.decode(ctx, apiRequest{
			method: "GET",
			path:   "/v2/spaces/" + seg(spaceId) + "/properties/" + seg(key) + "/options",
			query:  url.Values{"prefix": []string{name}, "limit": []string{"50"}, "keys": []string{"name"}},
		}, &resp)
		if err != nil {
			return fmt.Errorf("list options of %q: %w", label, err)
		}
		if optionExists(resp.Data, name) {
			continue
		}
		return r.unknownOptionError(ctx, spaceId, key, label, name)
	}
	return nil
}

func optionExists(options []v2model.OptionRow, name string) bool {
	for _, o := range options {
		if o.Name == name {
			return true
		}
	}
	return false
}

// unknownOptionError builds the guard's steering error: the existing names
// and a case-insensitive did-you-mean. The property is named by label (its
// display name), while key still drives the option route.
func (r *Runner) unknownOptionError(ctx context.Context, spaceId, key, label, name string) error {
	var resp v2model.ListResponse[v2model.OptionRow]
	listErr := r.client.decode(ctx, apiRequest{
		method: "GET",
		path:   "/v2/spaces/" + seg(spaceId) + "/properties/" + seg(key) + "/options",
		query:  url.Values{"limit": []string{"15"}, "keys": []string{"name"}},
	}, &resp)
	msg := fmt.Sprintf("property %q has no option named %q — this tool never creates options", label, name)
	if listErr == nil && len(resp.Data) > 0 {
		names := make([]string, 0, len(resp.Data))
		var suggestion string
		for _, o := range resp.Data {
			names = append(names, o.Name)
			if strings.EqualFold(o.Name, name) {
				suggestion = o.Name
			}
		}
		msg += "; existing: " + strings.Join(names, ", ")
		if suggestion != "" {
			msg += fmt.Sprintf(" — did you mean %q?", suggestion)
		}
	}
	return fmt.Errorf("%s", msg)
}

//
// ---- relative dates ----
//

var weekdays = map[string]time.Weekday{
	"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday,
	"wednesday": time.Wednesday, "thursday": time.Thursday,
	"friday": time.Friday, "saturday": time.Saturday,
}

// resolveRelativeDate maps the wrapper's relative-date words to an absolute
// RFC 3339 timestamp at local midnight: today / tomorrow / yesterday,
// weekday names (the next occurrence, today included), and +Nd / -Nd day
// offsets. Anything else reports ok=false and passes through literally.
func resolveRelativeDate(input string, now time.Time) (string, bool) {
	word := strings.ToLower(strings.TrimSpace(input))
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch word {
	case "today":
		return midnight.Format(time.RFC3339), true
	case "tomorrow":
		return midnight.AddDate(0, 0, 1).Format(time.RFC3339), true
	case "yesterday":
		return midnight.AddDate(0, 0, -1).Format(time.RFC3339), true
	}
	if wd, ok := weekdays[word]; ok {
		ahead := (int(wd) - int(midnight.Weekday()) + 7) % 7
		return midnight.AddDate(0, 0, ahead).Format(time.RFC3339), true
	}
	if len(word) >= 3 && (word[0] == '+' || word[0] == '-') && word[len(word)-1] == 'd' {
		if n, err := strconv.Atoi(word[1 : len(word)-1]); err == nil && n >= 0 && n <= 36500 {
			if word[0] == '-' {
				n = -n
			}
			return midnight.AddDate(0, 0, n).Format(time.RFC3339), true
		}
	}
	return "", false
}

// sortedValueKeys is prepareValues' deterministic input order.
func sortedValueKeys(values map[string]any) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
