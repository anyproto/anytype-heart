package bundle

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gosimple/unidecode"
	"github.com/iancoleman/strcase"
	"golang.org/x/text/unicode/norm"

	"github.com/anyproto/anytype-heart/core/domain"
)

// apislug.go is the derived api-slug table for bundled keys — the authority
// the identifier layer names: bundled
// relations and types are addressed on the API surface by a snake_case slug
// derived from the internal key IN CODE, both directions. The stored
// apiObjectKey detail cannot be the authority for bundled keys (old spaces
// predate it and no reviser backfills it), and a case transform cannot be
// the reverse mechanism (mediaArtistURL → media_artist_url → ToLowerCamel
// yields mediaArtistUrl; _score does not round-trip) — so the reverse is a
// table lookup, never a string transform.
//
// ApiSlug is deliberately the same transform objectcreator's
// injectApiObjectKey applies at mint, so a derived slug and a stored
// apiObjectKey for the same key can never disagree.

// ApiSlug derives the snake_case api key ("slug") from an internal key or a
// caller-supplied key. It is the mint-time normalization: v2 creates store
// its result as apiObjectKey, and the bundled tables below are built with it.
func ApiSlug(key string) string {
	return strcase.ToSnake(key)
}

// ApiSlugFromName derives a slug from a display name (transliterate, then
// snake) — the transform objectcreator applies when no key is supplied.
func ApiSlugFromName(name string) string {
	return strcase.ToSnake(unidecode.Unidecode(strings.TrimSpace(name)))
}

// SanitizeApiSlug constrains a DERIVED slug (from a display name or a
// document key — inputs no pattern ever checked) to the advertised key
// grammar `^[a-zA-Z0-9_]+$` and a maximum length: every disallowed rune
// becomes `_`, runs collapse, edges trim. Without it, "50% done", "C++" or
// "☕" (unidecode: "?") become identity-bearing apiObjectKey values that no
// key route can accept. An empty result means "no derivable slug" — the
// caller falls back to the minted internal key as the only address.
func SanitizeApiSlug(raw string, maxLen int) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range raw {
		valid := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !valid {
			r = '_'
		}
		if r == '_' {
			if lastUnderscore {
				continue
			}
			lastUnderscore = true
		} else {
			lastUnderscore = false
		}
		b.WriteRune(r)
	}
	out := strings.Trim(b.String(), "_")
	if len(out) > maxLen {
		out = strings.Trim(out[:maxLen], "_")
	}
	return out
}

// FoldApiKey is the forgiving-layer fold: lowercase
// with `_` and `-` stripped, so `dueDate`, `due_date` and `due-date` fold
// together. Exact match always wins before folding is consulted; two keys
// folding together is an ambiguity the caller must surface loudly, never
// resolve by guess — which is why the fold lookups below return every match.
func FoldApiKey(s string) string {
	// NFC first. A label is MINTED in NFC, but a hand-edited document or an
	// editor that decomposes on write can spell the same word in NFD — and a
	// decomposed `tiếng_việt` is a different byte sequence from the composed
	// one, so every exact-match step of the chain misses it and the value
	// lands under a key no relation owns. Folding is the layer whose whole job
	// is forgiving a near-miss a reader cannot see; a normalization difference
	// is the least visible near-miss there is, so it belongs here rather than
	// nowhere. Exact matching upstream is untouched.
	return strings.Map(func(r rune) rune {
		switch r {
		case '_', '-':
			return -1
		}
		return r
	}, strings.ToLower(norm.NFC.String(s)))
}

var (
	relationKeyByApiSlug map[string]domain.RelationKey
	typeKeyByApiSlug     map[string]domain.TypeKey
	relationKeysByFold   map[string][]domain.RelationKey
	typeKeysByFold       map[string][]domain.TypeKey
)

// init builds the two reverse tables, SORTED and with an injectivity guard.
//
// The guard is the point. `key -> slug` is a lossy transform, so two bundled
// keys can in principle land on one slug (or one fold), and the reverse table
// is a plain map: the winner would be whichever key Go's map iteration
// reached last — a different address per process, with no signal anywhere.
// The bundled table is the ONE authority for bundled keys in every space and
// offline (§7.5a-1); an authority that disagrees with itself between restarts
// is worse than no authority. Today the table is injective on both counts
// (194 relations → 194 slugs → 194 folds; 29 types → 29 → 29), so this can
// only fire on a bundled key ADDED later, at the moment it is added, in every
// test binary — which is exactly when it is cheap to rename.
//
// The fold arm panics too, even though relationKeysByFold is a slice and
// could hold both: two bundled keys sharing a fold would make that whole fold
// class permanently ambiguous for every caller of the forgiving layer, which
// is a defect to fix in the table, not to serve.
func init() {
	relationKeys := sortedApiSlugKeys(len(relations), func(yield func(string)) {
		for key := range relations {
			yield(key.String())
		}
	})
	if err := checkApiSlugInjectivity("relation", relationKeys); err != nil {
		panic(err)
	}
	relationKeyByApiSlug = make(map[string]domain.RelationKey, len(relationKeys))
	relationKeysByFold = make(map[string][]domain.RelationKey, len(relationKeys))
	for _, raw := range relationKeys {
		slug := ApiSlug(raw)
		relationKeyByApiSlug[slug] = domain.RelationKey(raw)
		fold := FoldApiKey(slug)
		relationKeysByFold[fold] = append(relationKeysByFold[fold], domain.RelationKey(raw))
	}

	typeKeys := sortedApiSlugKeys(len(types), func(yield func(string)) {
		for key := range types {
			yield(key.String())
		}
	})
	if err := checkApiSlugInjectivity("type", typeKeys); err != nil {
		panic(err)
	}
	typeKeyByApiSlug = make(map[string]domain.TypeKey, len(typeKeys))
	typeKeysByFold = make(map[string][]domain.TypeKey, len(typeKeys))
	for _, raw := range typeKeys {
		slug := ApiSlug(raw)
		typeKeyByApiSlug[slug] = domain.TypeKey(raw)
		fold := FoldApiKey(slug)
		typeKeysByFold[fold] = append(typeKeysByFold[fold], domain.TypeKey(raw))
	}
}

func sortedApiSlugKeys(size int, each func(yield func(string))) []string {
	out := make([]string, 0, size)
	each(func(key string) { out = append(out, key) })
	sort.Strings(out)
	return out
}

// checkApiSlugInjectivity is the guard init panics on. Two keys sharing a
// slug make the reverse table a coin flip; two keys sharing a fold make that
// whole fold class permanently ambiguous for the forgiving layer. Both are
// defects in the TABLE, to be fixed by renaming a key, never served.
func checkApiSlugInjectivity(kind string, keys []string) error {
	bySlug := make(map[string]string, len(keys))
	byFold := make(map[string]string, len(keys))
	for _, key := range keys {
		slug := ApiSlug(key)
		if first, taken := bySlug[slug]; taken {
			return fmt.Errorf("bundled %s keys %q and %q both derive the api slug %q — the reverse table would resolve it to whichever key the map reached last; rename one", kind, first, key, slug)
		}
		bySlug[slug] = key
		fold := FoldApiKey(slug)
		if first, taken := byFold[fold]; taken {
			return fmt.Errorf("bundled %s keys %q and %q fold together (%q) — the forgiving layer would be permanently ambiguous for that spelling; rename one", kind, first, key, fold)
		}
		byFold[fold] = key
	}
	return nil
}

// RelationKeyByApiSlug resolves a bundled relation's derived slug back to its
// internal key (`due_date` → `dueDate`). Bundled keys only; stored keys and
// stored slugs are the space's to resolve.
func RelationKeyByApiSlug(slug string) (domain.RelationKey, bool) {
	key, ok := relationKeyByApiSlug[slug]
	return key, ok
}

// TypeKeyByApiSlug resolves a bundled type's derived slug back to its
// internal key (`object_type` → `objectType`).
func TypeKeyByApiSlug(slug string) (domain.TypeKey, bool) {
	key, ok := typeKeyByApiSlug[slug]
	return key, ok
}

// RelationKeysByApiFold returns every bundled relation key whose derived slug
// folds to the input's fold — the forgiving layer's candidate set. Zero
// matches: not bundled; one: the match; two or more: ambiguous, fail loud.
func RelationKeysByApiFold(input string) []domain.RelationKey {
	return relationKeysByFold[FoldApiKey(input)]
}

// TypeKeysByApiFold is RelationKeysByApiFold for bundled type keys.
func TypeKeysByApiFold(input string) []domain.TypeKey {
	return typeKeysByFold[FoldApiKey(input)]
}
