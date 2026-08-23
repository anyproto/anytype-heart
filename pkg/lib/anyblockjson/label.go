package anyblockjson

// label.go — the spelling a document writes for a key the BUNDLED table does
// not speak for (§3).
//
// A bundled key spells its derived api slug and always has: the table ships
// with every reader, resolves offline, and every one of its 223 slugs is
// already a legal key in the §6.2.1 grammar. This file is about the other
// population — the keys a SPACE mints — and it exists because the spelling
// they used to get was borrowed from a surface with a different constraint.
//
// The old rule was "the stored `apiObjectKey`, or the stored key when there
// is none". Both halves leak:
//
//   - `apiObjectKey` is minted for API v2, where a slug is a URL PATH
//     SEGMENT, so its charset is `^[a-zA-Z0-9_]+$` — a constraint this format
//     does not have, and one that transliterates `Тоггл` to `toggl` and
//     `日本語のプロパティ` to `ri_ben_yu_nopuropatei`: unguessable AND
//     unreadable. Nothing in a JSON document needs to survive a URL.
//   - a key with no slug at all was spelled by its stored key, which for a
//     space-minted relation is a 24-character bson id. 39 of them in a
//     36,966-object corpus, every one an ordinary user property with an
//     ordinary name (`Publish Date`, `Active competitors`, `Website`).
//
// The constraint this format DOES have is §6.2.1's, and it is narrower in one
// direction and far wider in the other: a key is a Unicode identifier —
// `identStart identPart*`, letters (any script), digits, `_` — and not one of
// the grammar's reserved words. It is not a style rule. A label outside it
// cannot be written in a compact filter string, which is a surface this
// format serves to models AS AN EBNF GRAMMAR; a property spelled
// `697c7255877a9148c777f79c` cannot even be filtered on, because a bson id
// starts with a digit and `identStart` is a letter or `_`.
//
// So the label is normalized through THAT grammar instead: `Тоггл` stays
// `тоггл`, `日本語のプロパティ` stays `日本語のプロパティ`, and `Publish Date`
// — which had no slug and no readable spelling of any kind — becomes
// `publish_date`. The legend line the document already owed for every
// non-bundled key (§3, the exhaustive rule) inverts it, so this costs no
// bytes beyond the label's own length and no reader has to know the rule.
//
// **Non-Latin scripts are not transliterated.** They are already legal, and
// transliteration produces the worst outcome available: a label that is
// neither the name the user typed nor a word in any language.

import (
	"strings"
	"unicode"

	"github.com/iancoleman/strcase"
	"golang.org/x/text/unicode/norm"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
)

// PropertyLabel is the spelling a document writes for one space-minted
// PROPERTY key, given the entity's stored api slug and its display name.
// An empty answer means the key has no label but itself: the stored key is
// written verbatim, which is always its own address (§3 chain step 4).
//
// The ladder, first answer wins:
//
//  1. the stored slug, when it is already a legal key — an explicit,
//     rename-stable address the space minted on purpose, and the case that
//     covers 96% of slugged relations unchanged;
//  2. the stored slug NORMALIZED, when it is not — `apiObjectKey` is not
//     sanitized at mint, and four relations in the production corpus store
//     `"#"`;
//  3. the display NAME normalized, when there is no usable slug — the case
//     that gives the 39 slugless relations a spelling at all;
//  4. nothing.
//
// A slug that merely repeats the stored key is not a slug (some rows store
// the bson id as their own `apiObjectKey`), so it falls to the name.
//
// `id` and `type` are refused: §2 refuses both SPELLINGS in `properties`
// before any resolution, so minting one would produce a label the exporter
// throws away with a warning. The type namespace does not share that
// reservation — its home surface is a value, not a member name — which is
// the one place the two namespaces differ and why TypeLabel is a separate
// function rather than a flag.
func PropertyLabel(key, slug, name string) string {
	label := keyLabel(key, slug, name)
	if label == detailKeyId || label == detailKeyType {
		return ""
	}
	return label
}

// TypeLabel is PropertyLabel for the type namespace.
func TypeLabel(key, slug, name string) string {
	return keyLabel(key, slug, name)
}

func keyLabel(key, slug, name string) string {
	var label string
	if slug != "" && slug != key {
		label = slug
		if !filterstring.IsBareKey(label) {
			label = normalizeKeyLabel(label)
		}
	} else {
		label = normalizeKeyLabel(name)
	}
	// isWritablePropertyKey is the format's own shape rule (§3) — non-empty
	// and inside the schema's 128-character bound. A label longer than that
	// is refused rather than truncated: a truncation invents a spelling
	// nobody chose, and the stored key is right there.
	if label == key || !isWritablePropertyKey(label) {
		return ""
	}
	return label
}

// normalizeKeyLabel turns an arbitrary human string into a key in the §6.2.1
// grammar, or "" when nothing is left to name.
//
// It is `ApiSlugFromName` with the transliteration removed and the charset
// widened to the actual grammar: the SAME `strcase.ToSnake` the api slug is
// minted with, so an ASCII name lands on exactly the string GO-7458's
// backfill would store (`Publish Date` → `publish_date`) and the two
// surfaces converge instead of drifting — then constrained by what a filter
// key may contain rather than by what a URL path segment may contain.
//
// Four decisions worth stating, because each has a plausible alternative:
//
//   - **NFC.** Two visually identical names can be different byte sequences,
//     and a label is compared byte-for-byte by every reader. Export is safe
//     by construction (the same string is written as the label AND as the
//     legend key), but a hand-edited document in the other form must not
//     silently miss, so one form is picked and §3 states it.
//   - **Lowercase.** The format's own vocabulary is snake_case (§1), the
//     bundled slugs are, and the fold layer is case-insensitive anyway, so
//     case carries nothing here. Non-ASCII lowercases too: `Тоггл` → `тоггл`.
//   - **Combining marks are dropped, not separated.** A mark modifies the
//     letter before it; turning it into `_` would cut `क्षत्रिय` into pieces
//     at every virama. Dropping keeps the word one word.
//   - **A leading `_` is the escape for both grammar faults.** A label
//     starting with a digit (`50% done` → `50_done`) and a label that IS a
//     keyword (`All` → `all`) are the only two ways this construction can
//     fail the grammar, and both are repaired by one prefix — `_50_done`,
//     `_all`. Dropping the leading digits instead would silently turn
//     `50% done` into `done`, which is a DIFFERENT bundled property; falling
//     back to the stored key would throw away a name that is perfectly
//     readable one character later.
func normalizeKeyLabel(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	gap := false // a separator run is pending, emitted only before the next letter
	for _, r := range strcase.ToSnake(norm.NFC.String(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if gap && b.Len() > 0 {
				b.WriteRune('_')
			}
			gap = false
			b.WriteRune(unicode.ToLower(r))
		case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r):
			// A combining mark belongs to the letter it follows and is KEPT:
			// in Devanagari, Thai, Bengali, Tamil, Khmer and Myanmar the
			// vowels ARE marks, so dropping them does not shorten a word, it
			// changes it — मिल/मूल/मल/मैल would all become मल. The grammar
			// admits them (identPart, UAX #31 ID_Continue), so they pass
			// through with the letter rather than being treated as a break.
			// No `gap` reset: a mark cannot start a token, and one arriving
			// with a pending separator is malformed input, not a word.
			if b.Len() > 0 && !gap {
				b.WriteRune(r)
			}
		default:
			gap = true // `_` included: runs collapse and edges trim
		}
	}
	label := b.String()
	if label == "" {
		return ""
	}
	if !filterstring.IsBareKey(label) {
		label = "_" + label
	}
	if !filterstring.IsBareKey(label) {
		// unreachable by construction — every rune is already an identPart,
		// so the only faults are a leading digit and a keyword, both cured
		// above. It is a guard rather than a path: IsBareKey is another
		// package's rule and may grow one, and the honest degradation is no
		// label at all, never a label that package would refuse.
		return ""
	}
	return label
}
