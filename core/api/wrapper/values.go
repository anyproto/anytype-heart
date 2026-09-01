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
	}
	for _, row := range rows {
		idx.formats[row.Key] = row.Format
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
			return nil, fmt.Errorf("property %q is given more than once (several spellings resolve to one property) — pass it once", foldedKey)
		}
		format := idx.formats[foldedKey]
		resolved, err := r.resolveValue(ctx, session, spaceId, format, value)
		if err != nil {
			return nil, fmt.Errorf("value of %q: %w", key, err)
		}
		if guard && selectFormats[format] && !r.AllowNewOptions {
			if err := r.checkOptionNames(ctx, spaceId, foldedKey, stringEntries(resolved)); err != nil {
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
	return "", fmt.Errorf("property key %q matches several properties (%s) — use the exact spelling",
		key, strings.Join(describeRowSpellings(rows), ", "))
}

// describeRowSpellings renders ambiguity candidates: the display name with
// the api key beside it where the two differ — the name is the vocabulary
// this surface teaches, the key is the address that always resolves.
func describeRowSpellings(rows []v2model.PropertyRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		switch {
		case row.Name == "" || row.Name == row.Key:
			out = append(out, row.Key)
		default:
			out = append(out, fmt.Sprintf("%q (key %s)", row.Name, row.Key))
		}
	}
	sort.Strings(out)
	return out
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
		return "", false, fmt.Errorf("type key %q matches several types (%s) — use the exact key",
			typeKey, strings.Join(describeTypeRowSpellings(matches), ", "))
	default:
		return "", false, nil
	}
}

// describeTypeRowSpellings is describeRowSpellings for type rows.
func describeTypeRowSpellings(rows []v2model.TypeRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		switch {
		case row.Name == "" || row.Name == row.Key:
			out = append(out, row.Key)
		default:
			out = append(out, fmt.Sprintf("%q (key %s)", row.Name, row.Key))
		}
	}
	sort.Strings(out)
	return out
}

// meFormats are the property formats whose values can hold a participant id
// — the only place the @me sentinel substitutes. On any other format the
// token is DATA (a description or name literally containing "@me" must not
// silently become a participant id).
var meFormats = map[string]bool{"objects": true}

// resolveValue rewrites one value: @me on object-format keys, relative
// dates on date-format keys.
func (r *Runner) resolveValue(ctx context.Context, session *Session, spaceId, format string, value any) (any, error) {
	switch v := value.(type) {
	case string:
		if v == meSentinel && meFormats[format] {
			return r.meFor(ctx, session, spaceId)
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
// stays available behind AllowNewOptions.
func (r *Runner) checkOptionNames(ctx context.Context, spaceId, key string, names []string) error {
	for _, name := range names {
		var resp v2model.ListResponse[v2model.OptionRow]
		err := r.client.decode(ctx, apiRequest{
			method: "GET",
			path:   "/v2/spaces/" + seg(spaceId) + "/properties/" + seg(key) + "/options",
			query:  url.Values{"prefix": []string{name}, "limit": []string{"50"}},
		}, &resp)
		if err != nil {
			return fmt.Errorf("list options of %q: %w", key, err)
		}
		if optionExists(resp.Data, name) {
			continue
		}
		return r.unknownOptionError(ctx, spaceId, key, name)
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
// and a case-insensitive did-you-mean.
func (r *Runner) unknownOptionError(ctx context.Context, spaceId, key, name string) error {
	var resp v2model.ListResponse[v2model.OptionRow]
	listErr := r.client.decode(ctx, apiRequest{
		method: "GET",
		path:   "/v2/spaces/" + seg(spaceId) + "/properties/" + seg(key) + "/options",
		query:  url.Values{"limit": []string{"15"}},
	}, &resp)
	msg := fmt.Sprintf("property %q has no option named %q — this tool never creates options", key, name)
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
