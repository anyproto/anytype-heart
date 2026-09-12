package schemaplan

import (
	"strings"
	"unicode"
)

// A type carries two names: "Project" for one member, "Projects" for a list of
// them. Both are the model's to write, and a model asked for both routinely
// answers one — the plural in the singular field, the same word twice, or
// nothing in the second field at all. The container-name fallback below never
// had a plural either: a Notion database called "AI Projects" became a type
// called "AI Projects" with an empty plural, which is what the user sees as a
// blank "Type plural name" field.
//
// So the grammar is code's job, the way property mapping is: the model names
// the kind, and this turns that name into the pair Anytype needs. Only the
// last word of a phrase inflects ("Team Member" → "Team Members").

// irregularPlurals are the words no rule gets right. Small on purpose: a
// wrong guess here reads worse than the rule-based plural it replaces.
var irregularPlurals = map[string]string{
	"person": "people", "child": "children", "man": "men", "woman": "women",
	"foot": "feet", "tooth": "teeth", "goose": "geese", "mouse": "mice",
	"ox": "oxen", "datum": "data", "medium": "media", "criterion": "criteria",
	"analysis": "analyses", "thesis": "theses", "diagnosis": "diagnoses",
	"index": "indices", "appendix": "appendices", "matrix": "matrices",
	"life": "lives", "leaf": "leaves", "shelf": "shelves", "wife": "wives",
	"knife": "knives", "half": "halves", "self": "selves",
}

// singularEndingInS are singular words that end in s where the rules would
// otherwise strip it. Without them "Ideas" and "Life Areas" — ordinary names
// for a Notion database — would keep their s and gain another ("Ideases"),
// which is why the list holds the exceptions rather than the pattern.
var singularEndingInS = map[string]bool{
	"atlas": true, "canvas": true, "gas": true, "alias": true, "bias": true,
	"chaos": true, "christmas": true, "campus": true, "lens": true,
}

// uncountable words are their own plural. "Research", "Series", "Feedback":
// pluralizing them produces something no one writes.
var uncountable = map[string]bool{
	"research": true, "series": true, "species": true, "news": true, "info": true,
	"information": true, "equipment": true, "software": true, "hardware": true,
	"feedback": true, "content": true, "staff": true, "media": true, "data": true,
	"money": true, "music": true, "traffic": true, "training": true, "furniture": true,
	"progress": true, "analytics": true,
}

// normalizeTypeNames returns the singular and plural a type should carry,
// from whatever the plan supplied. A plural that adds nothing to the singular
// — absent, or the same word — is treated as unanswered.
func normalizeTypeNames(name, plural string) (string, string) {
	name, plural = strings.TrimSpace(name), strings.TrimSpace(plural)
	if name == "" {
		return name, plural
	}
	if plural != "" && !strings.EqualFold(plural, name) {
		return name, plural
	}
	if singular, ok := singularize(name); ok {
		return singular, name
	}
	return name, pluralize(name)
}

// lastWord splits a noun phrase into everything before the head noun and the
// head noun itself, which is the only part that inflects.
func lastWord(phrase string) (prefix, word string) {
	if cut := strings.LastIndex(phrase, " "); cut >= 0 {
		return phrase[:cut+1], phrase[cut+1:]
	}
	return "", phrase
}

// trailingTag splits off a bracketed tag at the end of a phrase. The model
// echoes the Notion database title, and those titles carry workspace
// shorthand — "Reminders (SB)" — where the noun that inflects is the part in
// front of the tag, not the tag.
func trailingTag(phrase string) (base, tag string) {
	trimmed := strings.TrimRight(phrase, " ")
	var opener byte
	switch {
	case strings.HasSuffix(trimmed, ")"):
		opener = '('
	case strings.HasSuffix(trimmed, "]"):
		opener = '['
	default:
		return phrase, ""
	}
	cut := strings.LastIndexByte(trimmed, opener)
	if cut <= 0 {
		return phrase, ""
	}
	base = strings.TrimRight(trimmed[:cut], " ")
	if base == "" {
		return phrase, ""
	}
	return base, phrase[len(base):]
}

// inflectable reports whether a word ends in a letter, which is what every
// rule below assumes. "(SB)", "2024" and "Connected," do not, and an s on the
// end of those writes something no one typed.
func inflectable(word string) bool {
	runes := []rune(word)
	return len(runes) > 0 && unicode.IsLetter(runes[len(runes)-1])
}

// matchCase gives a derived word the capitalization of the word it came from,
// so "PROJECTS" does not become "PROJECTs".
func matchCase(source, derived string) string {
	if source == strings.ToUpper(source) && source != strings.ToLower(source) {
		return strings.ToUpper(derived)
	}
	if runes := []rune(source); len(runes) > 0 && strings.ToUpper(string(runes[0])) == string(runes[0]) {
		derivedRunes := []rune(derived)
		return strings.ToUpper(string(derivedRunes[0])) + string(derivedRunes[1:])
	}
	return derived
}

// conjunctions join two nouns that each inflect: "Tasks & Features" is two
// plurals, and inflecting only the last word leaves "Tasks & Feature".
var conjunctions = []string{" & ", " and ", " + ", " / "}

// splitConjunction breaks a phrase at the LAST joiner it contains.
func splitConjunction(phrase string) (head, joiner, tail string, ok bool) {
	for _, candidate := range conjunctions {
		if cut := strings.LastIndex(phrase, candidate); cut >= 0 {
			return phrase[:cut], candidate, phrase[cut+len(candidate):], true
		}
	}
	return "", "", "", false
}

func pluralize(phrase string) string {
	if base, tag := trailingTag(phrase); tag != "" {
		return pluralize(base) + tag
	}
	if head, joiner, tail, ok := splitConjunction(phrase); ok {
		return pluralize(head) + joiner + pluralize(tail)
	}
	prefix, word := lastWord(phrase)
	if !inflectable(word) {
		return phrase
	}
	lower := strings.ToLower(word)
	if uncountable[lower] {
		return phrase
	}
	if irregular, ok := irregularPlurals[lower]; ok {
		return prefix + matchCase(word, irregular)
	}
	switch {
	case strings.HasSuffix(lower, "s"), strings.HasSuffix(lower, "x"), strings.HasSuffix(lower, "z"),
		strings.HasSuffix(lower, "ch"), strings.HasSuffix(lower, "sh"):
		return prefix + word + "es"
	case strings.HasSuffix(lower, "y") && len(lower) > 1 && !isVowel(lower[len(lower)-2]):
		return prefix + word[:len(word)-1] + "ies"
	default:
		return prefix + word + "s"
	}
}

// singularize reports the singular of a word that IS a plural. Not-ok means
// the word does not look like one — "Analysis" and "Status" end in s and are
// not, and guessing there is how a type ends up called "Statu".
func singularize(phrase string) (string, bool) {
	if base, tag := trailingTag(phrase); tag != "" {
		if singular, ok := singularize(base); ok {
			return singular + tag, true
		}
		return "", false
	}
	if head, joiner, tail, ok := splitConjunction(phrase); ok {
		singularHead, headOk := singularize(head)
		singularTail, tailOk := singularize(tail)
		if !headOk && !tailOk {
			return "", false
		}
		if !headOk {
			singularHead = head
		}
		if !tailOk {
			singularTail = tail
		}
		return singularHead + joiner + singularTail, true
	}
	prefix, word := lastWord(phrase)
	if !inflectable(word) {
		return "", false
	}
	lower := strings.ToLower(word)
	if uncountable[lower] {
		return "", false
	}
	for singular, plural := range irregularPlurals {
		if lower == plural {
			return prefix + matchCase(word, singular), true
		}
	}
	if irregularPlurals[lower] != "" {
		return "", false // it is the singular of an irregular pair
	}
	switch {
	case strings.HasSuffix(lower, "ies") && len(lower) > 4:
		return prefix + word[:len(word)-3] + "y", true
	case strings.HasSuffix(lower, "sses"), strings.HasSuffix(lower, "shes"), strings.HasSuffix(lower, "ches"),
		strings.HasSuffix(lower, "xes"), strings.HasSuffix(lower, "zes"):
		return prefix + word[:len(word)-2], true
	case strings.HasSuffix(lower, "ss"), strings.HasSuffix(lower, "us"), strings.HasSuffix(lower, "is"):
		return "", false // class, status, bonus, analysis, basis
	case singularEndingInS[lower]:
		return "", false // atlas, canvas, chaos — the exceptions to the next line
	case strings.HasSuffix(lower, "s") && len(lower) > 2:
		return prefix + word[:len(word)-1], true
	}
	return "", false
}

func isVowel(b byte) bool {
	return strings.IndexByte("aeiou", b) >= 0
}
