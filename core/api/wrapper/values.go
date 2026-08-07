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

// propertyFormats loads the space's property key → format index.
func (r *Runner) propertyFormats(ctx context.Context, spaceId string) (map[string]string, error) {
	formats := map[string]string{}
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
		for _, row := range resp.Data {
			formats[row.Key] = row.Format
		}
		if !resp.HasMore {
			break
		}
		offset += len(resp.Data)
	}
	return formats, nil
}

// selectFormats are the formats whose values are option NAMES the guard
// checks.
var selectFormats = map[string]bool{"select": true, "multiSelect": true}

// prepareValues resolves @me and relative dates in a property-value map and
// — when guard is set — pre-validates select option names (the A2 guard).
// Keys fold case first (§8.21): the folded key drives the format lookup, so
// "DueDate": "friday" gets its date resolution too. Unknown keys pass
// through untouched: the server's referential layer owns key validation and
// its did-you-mean texts. formats is the space's property key → format
// index (propertyFormats), loaded ONCE per tool call — a set_properties
// with set+add+remove must not fetch the index three times.
func (r *Runner) prepareValues(ctx context.Context, session *Session, spaceId string, formats map[string]string, values map[string]any, guard bool) (map[string]any, error) {
	out := make(map[string]any, len(values))
	for key, value := range values {
		foldedKey, err := foldPropertyKey(formats, key)
		if err != nil {
			return nil, err
		}
		if _, dup := out[foldedKey]; dup {
			return nil, fmt.Errorf("property %q is given more than once (case variants fold to one key) — pass it once", foldedKey)
		}
		format := formats[foldedKey]
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
// ---- key case folding (§8.21) ----
//
// The live benchmark's dominant argument error was a naming/capitalisation
// guess — "Page" for type page, "Name" for property name — never a
// structural violation. C2 strictness (a key is a key) is the REST
// surface's contract and stays; the wrapper is the layer that exists to be
// forgiving for small models, so it folds case on the way in. The one hard
// rule: if two keys differ only by case, refuse naming both — never guess.

// foldPropertyKey resolves a property key against the space's visible key
// index: an exact key wins; otherwise a UNIQUE case-insensitive match
// resolves; a still-unknown key passes through for the server's
// did-you-mean.
func foldPropertyKey(formats map[string]string, key string) (string, error) {
	if _, ok := formats[key]; ok {
		return key, nil
	}
	var matches []string
	for k := range formats {
		if strings.EqualFold(k, key) {
			matches = append(matches, k)
		}
	}
	sort.Strings(matches)
	switch len(matches) {
	case 0:
		return key, nil
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("property key %q matches several keys differing only by case (%s) — use the exact key", key, strings.Join(matches, ", "))
	}
}

// maxTypePages bounds the type-index pagination loop.
const maxTypePages = 4

// typeKeys lists the space's type keys (the fold's reference set).
func (r *Runner) typeKeys(ctx context.Context, spaceId string) ([]string, error) {
	var keys []string
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
		for _, row := range resp.Data {
			keys = append(keys, row.Key)
		}
		if !resp.HasMore {
			break
		}
		offset += len(resp.Data)
	}
	return keys, nil
}

// isTypeNotFound reports whether err is the server's type-key miss for
// typeKey — every route that takes a type key says `type "<key>" not
// found` (describe's GET answers 404 not_found, create and find answer
// 400 validation_failed).
func isTypeNotFound(err error, typeKey string) bool {
	var te *ToolError
	return errors.As(err, &te) && strings.Contains(te.Text, fmt.Sprintf("type %q not found", typeKey))
}

// foldTypeArg is the case-fold retry gate for the tools that send a type
// key (find, describe, create): after a failed first attempt it reports
// the folded key to retry with — only when err is the server's not-found
// for exactly that key and a UNIQUE case variant exists among the space's
// type keys. The fold runs on the error path, so the correct-key common
// case never pays a type listing. A case collision is its own refusal; a
// listing failure or a fold miss keeps the original (candidate-bearing)
// error.
func (r *Runner) foldTypeArg(ctx context.Context, spaceId, typeKey string, err error) (string, bool, error) {
	if typeKey == "" || !isTypeNotFound(err, typeKey) {
		return "", false, nil
	}
	keys, listErr := r.typeKeys(ctx, spaceId)
	if listErr != nil {
		return "", false, nil // best-effort: the original error stands
	}
	var matches []string
	for _, k := range keys {
		if strings.EqualFold(k, typeKey) {
			matches = append(matches, k)
		}
	}
	sort.Strings(matches)
	switch {
	case len(matches) == 1 && matches[0] != typeKey:
		return matches[0], true, nil
	case len(matches) > 1:
		return "", false, fmt.Errorf("type key %q matches several keys differing only by case (%s) — use the exact key", typeKey, strings.Join(matches, ", "))
	default:
		return "", false, nil
	}
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
