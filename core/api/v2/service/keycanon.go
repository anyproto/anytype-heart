package v2service

// keycanon.go — input canonicalization for the QUERY channels (search
// fields/filters/sorts, list fields, set creation): the listings advertise
// served spellings (a BSON-keyed property answers to its slug), so every
// channel that takes a property key must accept that spelling and translate
// it to the STORED key the store binds (review cause 3 — before this, the
// API advertised exactly the key its query channels rejected). One primed
// snapshot per request (§7.5a-2); translation through the same §7.5a-5
// chain every other channel walks, file aliases folded in front.

import (
	"encoding/json"
	"fmt"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
)

// keyCanon is the per-request canonicalizer.
type keyCanon struct {
	s       *V2Service
	entries []propertyEntry
	aliases map[string]domain.RelationKey // chain-aware active file aliases
}

func (s *V2Service) newKeyCanon(spaceId string) (*keyCanon, error) {
	entries, err := s.liveProperties(spaceId)
	if err != nil {
		return nil, err
	}
	return &keyCanon{s: s, entries: entries, aliases: s.activeFieldAliasesIn(entries)}, nil
}

// canon translates one concrete input to its stored spelling: an active
// file alias maps to its backing relation; otherwise the §7.5a-5 chain
// resolves (stored key, slug, bundled, fold). Ambiguity returns the
// candidates (the caller owes the loud 400); a miss passes through
// verbatim — membership validation owns that refusal.
func (k *keyCanon) canon(input string) (string, []string) {
	if backing, ok := k.aliases[input]; ok {
		return string(backing), nil
	}
	entry, ok, ambiguous := k.s.resolvePropertyInput(input, k.entries)
	if len(ambiguous) > 0 {
		return input, ambiguous
	}
	if ok && entry.Key != "" {
		return entry.Key, nil
	}
	return input, nil
}

// withServedSpellings widens a stored-key reference set with the served
// spelling of each key (the listing's spelling must always be accepted —
// C2's one-vocabulary promise, kept across mint and listing).
func (k *keyCanon) withServedSpellings(stored []string) []string {
	keyTaken, slugCount := servedPropertyKeySets(k.entries)
	bySlug := map[string]string{}
	for _, entry := range k.entries {
		if served := servedKey(entry.Key, entry.Slug, keyTaken, slugCount); served != entry.Key {
			bySlug[entry.Key] = served
		}
	}
	out := append([]string{}, stored...)
	for _, key := range stored {
		if served, ok := bySlug[key]; ok {
			out = append(out, served)
		}
	}
	return sortedDistinct(out)
}

// servedSpellings maps a stored-key list to its served spellings (for
// candidate lists and did-you-mean — never advertise a spelling the
// channels reject).
func (k *keyCanon) servedSpellings(stored []string) []string {
	keyTaken, slugCount := servedPropertyKeySets(k.entries)
	byKey := map[string]string{}
	for _, entry := range k.entries {
		byKey[entry.Key] = servedKey(entry.Key, entry.Slug, keyTaken, slugCount)
	}
	out := make([]string, 0, len(stored))
	for _, key := range stored {
		if served, ok := byKey[key]; ok {
			out = append(out, served)
			continue
		}
		out = append(out, key)
	}
	return sortedDistinct(out)
}

// ---- generic JSON rewriters for the §6.2 channels ----
//
// The set-create request carries filters/sorts/views as raw JSON that lands
// in the set document verbatim — a served-slug property key there would
// bind a dataview filter to a spelling the store never matches, silently.
// The rewriters walk generic JSON (no partial-struct re-marshal, so no
// field is ever dropped) and canonicalize exactly the key slots
// collectViewPropertyKeys reads: filter nodes' property (recursive), sorts'
// property, views' groupBy/sorts/columns/filters.

func (k *keyCanon) canonOrErr(input, path string) (string, error) {
	canonical, ambiguous := k.canon(input)
	if len(ambiguous) > 0 {
		return "", ambiguousKeyError("property key", input, path, ambiguous)
	}
	return canonical, nil
}

func (k *keyCanon) rewriteFilterNodes(nodes []any, path string) error {
	for i, raw := range nodes {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		nodePath := fmt.Sprintf("%s/%d", path, i)
		if prop, ok := node["property"].(string); ok && prop != "" {
			canonical, err := k.canonOrErr(prop, nodePath+"/property")
			if err != nil {
				return err
			}
			node["property"] = canonical
		}
		if nested, ok := node["filters"].([]any); ok {
			if err := k.rewriteFilterNodes(nested, nodePath+"/filters"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (k *keyCanon) rewriteSorts(sorts []any, path string) error {
	for i, raw := range sorts {
		sort, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if prop, ok := sort["property"].(string); ok && prop != "" {
			canonical, err := k.canonOrErr(prop, fmt.Sprintf("%s/%d/property", path, i))
			if err != nil {
				return err
			}
			sort["property"] = canonical
		}
	}
	return nil
}

func (k *keyCanon) rewriteViews(views []any, path string) error {
	for i, raw := range views {
		view, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		prefix := fmt.Sprintf("%s/%d", path, i)
		if groupBy, ok := view["groupBy"].(string); ok && groupBy != "" {
			canonical, err := k.canonOrErr(groupBy, prefix+"/groupBy")
			if err != nil {
				return err
			}
			view["groupBy"] = canonical
		}
		if sorts, ok := view["sorts"].([]any); ok {
			if err := k.rewriteSorts(sorts, prefix+"/sorts"); err != nil {
				return err
			}
		}
		if columns, ok := view["columns"].([]any); ok {
			for j, rawCol := range columns {
				column, ok := rawCol.(map[string]any)
				if !ok {
					continue
				}
				if prop, ok := column["property"].(string); ok && prop != "" {
					canonical, err := k.canonOrErr(prop, fmt.Sprintf("%s/columns/%d/property", prefix, j))
					if err != nil {
						return err
					}
					column["property"] = canonical
				}
			}
		}
		if filters, ok := view["filters"].([]any); ok {
			if err := k.rewriteFilterNodes(filters, prefix+"/filters"); err != nil {
				return err
			}
		}
	}
	return nil
}

// canonicalizeRawChannel decodes one raw §6.2 channel, rewrites its key
// slots, and re-encodes. kind selects the walker.
func (k *keyCanon) canonicalizeRawChannel(raw json.RawMessage, kind, path string) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	var decoded []any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return raw, nil // shape errors belong to the channel's own decoder
	}
	var err error
	switch kind {
	case "filters":
		err = k.rewriteFilterNodes(decoded, path)
	case "sorts":
		err = k.rewriteSorts(decoded, path)
	case "views":
		err = k.rewriteViews(decoded, path)
	}
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("re-encode %s: %w", kind, err)
	}
	return out, nil
}

// canonFormatName wraps a format-name resolver with canonicalization, so a
// served slug's format resolves during filter parsing/validation exactly as
// the stored spelling's does.
func canonFormatName(base func(string) (string, bool), kc *keyCanon) func(string) (string, bool) {
	return func(key string) (string, bool) {
		if name, ok := base(key); ok {
			return name, ok
		}
		canonical, ambiguous := kc.canon(key)
		if len(ambiguous) > 0 || canonical == key {
			return "", false
		}
		return base(canonical)
	}
}

// ambiguousInputIssue converts chain candidates to a path-addressed issue
// (shared shape with ambiguousKeyError, for issue-list callers).
func ambiguousInputIssue(what, input, path string, candidates []string) v2model.Issue {
	return v2model.Issue{Path: path,
		Message: fmt.Sprintf("%s %q matches %s", what, input, joinAnd(candidates)),
		Hint:    "address the intended one by its exact key"}
}

func joinAnd(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += " and " + p
	}
	return out
}
