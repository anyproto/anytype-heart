package bundle

import (
	"strings"

	"github.com/gosimple/unidecode"
	"github.com/iancoleman/strcase"

	"github.com/anyproto/anytype-heart/core/domain"
)

// apislug.go is the derived api-slug table for bundled keys — the authority
// the identifier-layer design names in ADDRESSING.md §7.5a-1: bundled
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

// FoldApiKey is the forgiving-layer fold (ADDRESSING.md §7.5a-3): lowercase
// with `_` and `-` stripped, so `dueDate`, `due_date` and `due-date` fold
// together. Exact match always wins before folding is consulted; two keys
// folding together is an ambiguity the caller must surface loudly, never
// resolve by guess — which is why the fold lookups below return every match.
func FoldApiKey(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '_', '-':
			return -1
		}
		return r
	}, strings.ToLower(s))
}

var (
	relationKeyByApiSlug map[string]domain.RelationKey
	typeKeyByApiSlug     map[string]domain.TypeKey
	relationKeysByFold   map[string][]domain.RelationKey
	typeKeysByFold       map[string][]domain.TypeKey
)

func init() {
	relationKeyByApiSlug = make(map[string]domain.RelationKey, len(relations))
	relationKeysByFold = make(map[string][]domain.RelationKey, len(relations))
	for key := range relations {
		slug := ApiSlug(key.String())
		relationKeyByApiSlug[slug] = key
		fold := FoldApiKey(slug)
		relationKeysByFold[fold] = append(relationKeysByFold[fold], key)
	}
	typeKeyByApiSlug = make(map[string]domain.TypeKey, len(types))
	typeKeysByFold = make(map[string][]domain.TypeKey, len(types))
	for key := range types {
		slug := ApiSlug(key.String())
		typeKeyByApiSlug[slug] = key
		fold := FoldApiKey(slug)
		typeKeysByFold[fold] = append(typeKeysByFold[fold], key)
	}
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
