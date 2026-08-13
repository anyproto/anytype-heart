// Package filterstring parses the compact filter syntax of AnyBlock JSON
// (SPEC §6.2.1) into the §6.2 structured filter tree. The grammar and this
// parser ship as a library consumed by the API v2 request surface (APIV2.md
// Phase 4 — POST search and the POST sets `filter` field); the *document*
// view field `filter` stays reserved post-v1, so nothing in the parent
// package reads or writes the string form.
//
// Parse returns the structured filters ARRAY as canonical JSON — exactly the
// §6.2 shape the structured `filters` request field carries — so both request
// forms land on one internal tree through the same downstream codec
// (anyblockjson.UnmarshalFilters). Every parse error is offset-addressed
// (*Error: byte offset + offending token) and, where a reference set is
// wired in via Options, carries a did-you-mean hint — the agent repair loop.
//
// Deliberately absent, per SPEC §6.2.1: free-standing NOT(…) (the internal
// model has no NOT-group), joins, subqueries, arbitrary functions.
package filterstring

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// maxFilterLength bounds the input in bytes. It matches the maxLength the
	// discovery schemas advertise for the `filter` field (4096); anything
	// longer is rejected before lexing so no attacker-sized input reaches the
	// recursive parser.
	maxFilterLength = 4096
	// maxGroupDepth bounds parenthesis nesting — the recursion depth of the
	// parser. It matches the document side's nesting bound (SPEC §4: [0,32]).
	// Without it a paren-bomb input overflows the goroutine stack, which is a
	// runtime FATAL (not a recoverable panic) and kills the whole process.
	maxGroupDepth = 32
	// maxDayCount bounds the counting presets' operand (~100 years); larger
	// values wrap around inside time.AddDate and produce meaningless ranges.
	maxDayCount = 36500
	// maxTokenRunes bounds how much of an offending token an error echoes
	// back — an unterminated string would otherwise mirror the whole rest of
	// the input into the response (and the agent's context window).
	maxTokenRunes = 32
)

// Options wires the space's reference sets into the parser. All fields are
// optional; a zero Options parses purely syntactically.
type Options struct {
	// KnownKeys, when non-nil, is the closed reference set for property keys:
	// a key outside it is an offset-addressed parse error with did-you-mean.
	KnownKeys []string
	// ResolveFormat, when non-nil, resolves a property key to its §3 format
	// name ("date", "select", "multiSelect", …). It drives the RFC 3339 →
	// unix conversion for date properties (the §6.2.1 mapping: the string
	// form uses RFC 3339, the structured form unix numbers) and targets the
	// option-name validation at select-shaped keys.
	ResolveFormat func(key string) (format string, ok bool)
	// KnownOptions, when non-nil, lists the existing option names of a
	// select/multiSelect property (ok=false: unknown property — no check).
	// A name outside the list is an offset-addressed parse error with
	// did-you-mean: the query path resolves option names READ-ONLY, and a
	// silent no-match would be worse than an error (APIV2.md Phase-4 rule 3).
	KnownOptions func(key string) (names []string, ok bool)
}

// Error is an offset-addressed parse error: the byte offset into the input,
// the offending token (empty at end of input), and a message naming allowed
// values, plus an optional repair hint (did-you-mean).
type Error struct {
	Offset  int
	Token   string
	Message string
	Hint    string
}

func (e *Error) Error() string {
	where := fmt.Sprintf("near %q", e.Token)
	if e.Token == "" {
		where = "at end of input"
	}
	msg := fmt.Sprintf("parse error at offset %d %s: %s", e.Offset, where, e.Message)
	if e.Hint != "" {
		msg += " — " + e.Hint
	}
	return msg
}

// datePresets maps the preset function names (SPEC §6.2.1) to the structured
// form's datePreset names (§6.2). The two counting presets take an operand.
var datePresets = map[string]string{
	"yesterday":    "yesterday",
	"today":        "today",
	"tomorrow":     "tomorrow",
	"lastWeek":     "lastWeek",
	"currentWeek":  "currentWeek",
	"nextWeek":     "nextWeek",
	"lastMonth":    "lastMonth",
	"currentMonth": "currentMonth",
	"nextMonth":    "nextMonth",
	"lastYear":     "lastYear",
	"currentYear":  "currentYear",
	"nextYear":     "nextYear",
	"daysAgo":      "numberOfDaysAgo",
	"daysFromNow":  "numberOfDaysNow",
}

// countingPresets are the preset functions that require a day-count operand.
var countingPresets = map[string]bool{"daysAgo": true, "daysFromNow": true}

// presetFunctionList renders the function vocabulary for error messages, in
// SPEC order.
const presetFunctionList = "yesterday() · today() · tomorrow() · lastWeek() · currentWeek() · nextWeek() · lastMonth() · currentMonth() · nextMonth() · lastYear() · currentYear() · nextYear() · daysAgo(n) · daysFromNow(n)"

// reservedWords are the keywords of the grammar; a property key cannot be
// one of them (matched case-insensitively).
var reservedWords = map[string]bool{
	"and": true, "or": true, "not": true, "is": true, "in": true,
	"contains": true, "has": true, "all": true, "empty": true,
	"exists": true, "true": true, "false": true,
}

//
// ---- lexer ----
//

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokString
	tokNumber
	tokOp     // = != > < >= <=
	tokLParen // (
	tokRParen // )
	tokComma  // ,
)

type token struct {
	kind   tokenKind
	text   string // the raw token text (for strings: the decoded value)
	raw    string // the raw source text (for error reporting)
	offset int    // byte offset of the token's first character
}

type lexer struct {
	input string
	pos   int
	toks  []token
}

func lex(input string) ([]token, *Error) {
	lx := &lexer{input: input}
	for {
		lx.skipSpace()
		if lx.pos >= len(lx.input) {
			lx.toks = append(lx.toks, token{kind: tokEOF, offset: lx.pos})
			return lx.toks, nil
		}
		start := lx.pos
		c := lx.input[lx.pos]
		switch {
		case c == '(':
			lx.emit(tokLParen, "(", start)
		case c == ')':
			lx.emit(tokRParen, ")", start)
		case c == ',':
			lx.emit(tokComma, ",", start)
		case c == '"':
			if err := lx.lexString(start); err != nil {
				return nil, err
			}
		case c == '=':
			lx.emit(tokOp, "=", start)
		case c == '!':
			if lx.pos+1 < len(lx.input) && lx.input[lx.pos+1] == '=' {
				lx.pos++
				lx.emit(tokOp, "!=", start)
			} else {
				return nil, &Error{Offset: start, Token: "!", Message: "unexpected character '!'; the negated conditions are !=, NOT CONTAINS, NOT IN, NOT HAS ALL, IS NOT EMPTY"}
			}
		case c == '>' || c == '<':
			op := string(c)
			if lx.pos+1 < len(lx.input) && lx.input[lx.pos+1] == '=' {
				lx.pos++
				op += "="
			}
			lx.emit(tokOp, op, start)
		case c == '-' || (c >= '0' && c <= '9'):
			lx.lexNumber(start)
		case c == '\'' || c == '`':
			return nil, &Error{Offset: start, Token: string(c),
				Message: fmt.Sprintf("unexpected character %q", string(c)),
				Hint:    `string values use double quotes, e.g. severity = "High"`}
		default:
			// decode a full rune so multi-byte input is classified (and
			// reported) as the character the caller wrote, never a stray byte
			r, size := utf8.DecodeRuneInString(lx.input[lx.pos:])
			if isIdentStart(r) {
				lx.lexIdent(start)
				continue
			}
			return nil, &Error{Offset: start, Token: lx.input[start : start+size], Message: fmt.Sprintf("unexpected character %q", string(r))}
		}
	}
}

func (lx *lexer) skipSpace() {
	for lx.pos < len(lx.input) {
		c := lx.input[lx.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			lx.pos++
			continue
		}
		return
	}
}

func (lx *lexer) emit(kind tokenKind, text string, start int) {
	lx.pos = start + len(text)
	lx.toks = append(lx.toks, token{kind: kind, text: text, raw: text, offset: start})
}

func (lx *lexer) lexString(start int) *Error {
	var sb strings.Builder
	i := start + 1
	for i < len(lx.input) {
		c := lx.input[i]
		switch c {
		case '\\':
			if i+1 >= len(lx.input) {
				return &Error{Offset: start, Token: truncateToken(lx.input[start:]), Message: "unterminated string literal"}
			}
			next := lx.input[i+1]
			switch next {
			case '"', '\\':
				sb.WriteByte(next)
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			default:
				return &Error{Offset: i, Token: lx.input[i : i+2], Message: fmt.Sprintf(`unknown escape \%c in string literal; allowed: \" \\ \n \t`, next)}
			}
			i += 2
		case '"':
			lx.toks = append(lx.toks, token{kind: tokString, text: sb.String(), raw: lx.input[start : i+1], offset: start})
			lx.pos = i + 1
			return nil
		default:
			sb.WriteByte(c)
			i++
		}
	}
	return &Error{Offset: start, Token: truncateToken(lx.input[start:]), Message: `unterminated string literal — close it with "`}
}

// truncateToken caps an offending token at maxTokenRunes for error echoing —
// errors name the problem, they do not mirror the input back.
func truncateToken(s string) string {
	runes := []rune(s)
	if len(runes) <= maxTokenRunes {
		return s
	}
	return string(runes[:maxTokenRunes]) + "…"
}

func (lx *lexer) lexNumber(start int) {
	i := start
	if lx.input[i] == '-' {
		i++
	}
	for i < len(lx.input) && (lx.input[i] >= '0' && lx.input[i] <= '9' || lx.input[i] == '.') {
		i++
	}
	text := lx.input[start:i]
	lx.toks = append(lx.toks, token{kind: tokNumber, text: text, raw: text, offset: start})
	lx.pos = i
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentPart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func (lx *lexer) lexIdent(start int) {
	i := start
	for i < len(lx.input) {
		r, size := utf8.DecodeRuneInString(lx.input[i:])
		if !isIdentPart(r) {
			break
		}
		i += size
	}
	text := lx.input[start:i]
	lx.toks = append(lx.toks, token{kind: tokIdent, text: text, raw: text, offset: start})
	lx.pos = i
}

//
// ---- parse tree ----
//

// node is the parsed tree before emission: either a group (op + children) or
// a leaf.
type node struct {
	op       string // "and" | "or"; "" = leaf
	children []*node

	property   string
	condition  string
	value      any // nil = no value (presence conditions, valueless presets)
	hasValue   bool
	datePreset string
}

//
// ---- parser ----
//

type parser struct {
	toks  []token
	pos   int
	depth int // current parenthesis nesting (bounded by maxGroupDepth)
	opts  Options
	// presetTok is the name token of the most recently parsed date-preset
	// function — preset-misuse errors address it, not the closing ')'
	presetTok token
}

// Parse parses a compact filter string (SPEC §6.2.1) into the §6.2
// structured filters array, returned as canonical compact JSON: top-level
// nodes combine with an implicit AND, groups exist only for OR and nesting.
// Errors are always *Error (offset-addressed).
func Parse(input string, opts Options) (json.RawMessage, error) {
	if strings.TrimSpace(input) == "" {
		return nil, &Error{Offset: 0, Message: "empty filter", Hint: `a filter is one or more conditions, e.g. done = false AND due_date < currentWeek()`}
	}
	if len(input) > maxFilterLength {
		return nil, &Error{Offset: maxFilterLength, Token: "",
			Message: fmt.Sprintf("filter string is %d bytes — the maximum is %d", len(input), maxFilterLength),
			Hint:    "split the query: narrow with type or run several searches"}
	}
	toks, lexErr := lex(input)
	if lexErr != nil {
		return nil, lexErr
	}
	p := &parser{toks: toks, opts: opts}
	root, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if tok := p.peek(); tok.kind != tokEOF {
		return nil, &Error{Offset: tok.offset, Token: tok.raw,
			Message: "expected AND, OR or end of input",
			Hint:    "conditions combine with AND and OR; parentheses group"}
	}
	return emit(root), nil
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }

// keyword reports whether tok is the given keyword (case-insensitive).
func keyword(tok token, kw string) bool {
	return tok.kind == tokIdent && strings.EqualFold(tok.text, kw)
}

func (p *parser) parseOr() (*node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	children := []*node{left}
	for keyword(p.peek(), "or") {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		children = append(children, right)
	}
	if len(children) == 1 {
		return left, nil
	}
	return &node{op: "or", children: flatten("or", children)}, nil
}

func (p *parser) parseAnd() (*node, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	children := []*node{left}
	for keyword(p.peek(), "and") {
		p.next()
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		children = append(children, right)
	}
	if len(children) == 1 {
		return left, nil
	}
	return &node{op: "and", children: flatten("and", children)}, nil
}

// flatten merges same-operator children into their parent (canonical form:
// parentheses that do not change semantics leave no trace).
func flatten(op string, children []*node) []*node {
	out := make([]*node, 0, len(children))
	for _, c := range children {
		if c.op == op {
			out = append(out, c.children...)
			continue
		}
		out = append(out, c)
	}
	return out
}

func (p *parser) parsePrimary() (*node, error) {
	tok := p.peek()
	if tok.kind == tokLParen {
		p.next()
		p.depth++
		if p.depth > maxGroupDepth {
			return nil, &Error{Offset: tok.offset, Token: tok.raw,
				Message: fmt.Sprintf("filter groups nest at most %d deep", maxGroupDepth),
				Hint:    "flatten the grouping — AND/OR chains do not need parentheses per condition"}
		}
		inner, err := p.parseOr()
		p.depth--
		if err != nil {
			return nil, err
		}
		if closing := p.next(); closing.kind != tokRParen {
			return nil, &Error{Offset: closing.offset, Token: closing.raw,
				Message: fmt.Sprintf("expected ) to close the group opened at offset %d", tok.offset)}
		}
		return inner, nil
	}
	return p.parseLeaf()
}

func (p *parser) parseLeaf() (*node, error) {
	tok := p.next()
	if tok.kind != tokIdent {
		return nil, &Error{Offset: tok.offset, Token: tok.raw,
			Message: "expected a property key",
			Hint:    "a condition starts with a bare property key, e.g. status IN (\"Done\")"}
	}
	if reservedWords[strings.ToLower(tok.text)] {
		return nil, &Error{Offset: tok.offset, Token: tok.raw,
			Message: fmt.Sprintf("%q is a reserved word, not a property key", tok.text),
			Hint:    "a property key that collides with a keyword cannot be written here — express this condition with the structured filters array instead"}
	}
	key := tok.text
	if err := p.checkKey(tok); err != nil {
		return nil, err
	}

	op := p.next()
	switch {
	case op.kind == tokOp:
		return p.parseComparison(key, op)
	case keyword(op, "contains"):
		return p.parseValueLeaf(key, "contains")
	case keyword(op, "in"):
		return p.parseListLeaf(key, "in")
	case keyword(op, "has"):
		return p.parseHasAll(key, op, "allIn")
	case keyword(op, "is"):
		return p.parseIs(key)
	case keyword(op, "exists"):
		return &node{property: key, condition: "exists"}, nil
	case keyword(op, "not"):
		after := p.next()
		switch {
		case keyword(after, "contains"):
			return p.parseValueLeaf(key, "notContains")
		case keyword(after, "in"):
			return p.parseListLeaf(key, "notIn")
		case keyword(after, "has"):
			return p.parseHasAll(key, after, "notAllIn")
		default:
			return nil, &Error{Offset: after.offset, Token: after.raw,
				Message: "expected CONTAINS, IN or HAS ALL after NOT",
				Hint:    "negations: != · NOT CONTAINS · NOT IN (…) · NOT HAS ALL (…) · IS NOT EMPTY"}
		}
	default:
		return nil, &Error{Offset: op.offset, Token: op.raw,
			Message: fmt.Sprintf("expected a condition after property %q", key),
			Hint:    "conditions: = != > < >= <= · CONTAINS · IN (…) · HAS ALL (…) · IS EMPTY · IS NOT EMPTY · EXISTS"}
	}
}

// parseComparison handles = != > < >= <=. An = / != followed by a
// parenthesized list is the set-literal form (exactIn / notExactIn).
func (p *parser) parseComparison(key string, op token) (*node, error) {
	if p.peek().kind == tokLParen {
		switch op.text {
		case "=":
			return p.parseListLeaf(key, "exactIn")
		case "!=":
			return p.parseListLeaf(key, "notExactIn")
		default:
			return nil, &Error{Offset: p.peek().offset, Token: "(",
				Message: fmt.Sprintf("a value list is only allowed after = or != (set literal), not after %s", op.text)}
		}
	}
	condition := map[string]string{
		"=": "equal", "!=": "notEqual",
		">": "greater", "<": "less", ">=": "greaterOrEqual", "<=": "lessOrEqual",
	}[op.text]
	return p.parseValueLeaf(key, condition)
}

// parseValueLeaf parses one value and builds the leaf, folding date-preset
// functions into datePreset.
func (p *parser) parseValueLeaf(key, condition string) (*node, error) {
	value, preset, err := p.parseValue(key)
	if err != nil {
		return nil, err
	}
	n := &node{property: key, condition: condition, datePreset: preset}
	if preset == "" || value != nil {
		// counting presets keep their operand as value; plain presets are
		// valueless (§6.2)
		n.value, n.hasValue = value, true
	}
	// the engine transforms a preset into a day range only for these
	// conditions (pkg/lib/database.transformDateFilter); anything else —
	// including != — would silently drop the preset and answer a different
	// question, so the parser rejects it up front
	if preset != "" && condition != "equal" &&
		condition != "greater" && condition != "less" &&
		condition != "greaterOrEqual" && condition != "lessOrEqual" {
		return nil, &Error{Offset: p.presetTok.offset, Token: p.presetTok.raw,
			Message: fmt.Sprintf("a date preset cannot be used with %s", condition),
			Hint:    `presets work with = > < >= <=; negate by range instead, e.g. due_date < today() OR due_date > today()`}
	}
	return n, nil
}

// parseListLeaf parses ( value, value, … ) and builds an in/notIn/allIn/
// notAllIn/exactIn/notExactIn leaf.
func (p *parser) parseListLeaf(key, condition string) (*node, error) {
	open := p.next()
	if open.kind != tokLParen {
		return nil, &Error{Offset: open.offset, Token: open.raw,
			Message: fmt.Sprintf("expected ( to start the %s value list", condition)}
	}
	var values []any
	for {
		tok := p.peek()
		if tok.kind == tokRParen && len(values) == 0 {
			return nil, &Error{Offset: tok.offset, Token: tok.raw,
				Message: "a value list needs at least one value"}
		}
		value, preset, err := p.parseValue(key)
		if err != nil {
			return nil, err
		}
		if preset != "" {
			return nil, &Error{Offset: p.presetTok.offset, Token: p.presetTok.raw,
				Message: "date presets cannot appear inside a value list"}
		}
		values = append(values, value)
		sep := p.next()
		if sep.kind == tokComma {
			continue
		}
		if sep.kind == tokRParen {
			return &node{property: key, condition: condition, value: values, hasValue: true}, nil
		}
		return nil, &Error{Offset: sep.offset, Token: sep.raw,
			Message: "expected , or ) in the value list"}
	}
}

// parseHasAll parses HAS ALL ( … ) / NOT HAS ALL ( … ).
func (p *parser) parseHasAll(key string, hasTok token, condition string) (*node, error) {
	all := p.next()
	if !keyword(all, "all") {
		return nil, &Error{Offset: all.offset, Token: all.raw,
			Message: "expected ALL after HAS",
			Hint:    `the contains-all condition is HAS ALL ("a", "b")`}
	}
	return p.parseListLeaf(key, condition)
}

// parseIs parses IS EMPTY / IS NOT EMPTY.
func (p *parser) parseIs(key string) (*node, error) {
	tok := p.next()
	if keyword(tok, "empty") {
		return &node{property: key, condition: "empty"}, nil
	}
	if keyword(tok, "not") {
		after := p.next()
		if keyword(after, "empty") {
			return &node{property: key, condition: "notEmpty"}, nil
		}
		return nil, &Error{Offset: after.offset, Token: after.raw,
			Message: "expected EMPTY after IS NOT"}
	}
	return nil, &Error{Offset: tok.offset, Token: tok.raw,
		Message: "expected EMPTY or NOT EMPTY after IS"}
}

// parseValue parses one value: string, number, true/false, or a date-preset
// function. It returns (value, presetName): a plain preset returns ("",
// preset) with a nil value; a counting preset returns its operand as value.
func (p *parser) parseValue(key string) (any, string, error) {
	tok := p.next()
	switch tok.kind {
	case tokString:
		return p.stringValue(key, tok)
	case tokNumber:
		f, err := strconv.ParseFloat(tok.text, 64)
		if err != nil {
			return nil, "", &Error{Offset: tok.offset, Token: tok.raw, Message: "invalid number"}
		}
		return f, "", nil
	case tokIdent:
		if strings.EqualFold(tok.text, "true") {
			return true, "", nil
		}
		if strings.EqualFold(tok.text, "false") {
			return false, "", nil
		}
		if p.peek().kind == tokLParen {
			return p.parsePresetCall(key, tok)
		}
		return nil, "", &Error{Offset: tok.offset, Token: tok.raw,
			Message: fmt.Sprintf("unexpected bare word %q in value position", tok.text),
			Hint:    `values are double-quoted strings, numbers, true/false, RFC 3339 dates in quotes, or date-preset functions like currentWeek()`}
	default:
		return nil, "", &Error{Offset: tok.offset, Token: tok.raw,
			Message: "expected a value",
			Hint:    `values are double-quoted strings, numbers, true/false, RFC 3339 dates in quotes, or date-preset functions like currentWeek()`}
	}
}

// parsePresetCall parses name( [n] ) — the date-preset functions.
func (p *parser) parsePresetCall(key string, nameTok token) (any, string, error) {
	p.presetTok = nameTok
	preset, known := datePresets[nameTok.text]
	if !known {
		return nil, "", &Error{Offset: nameTok.offset, Token: nameTok.raw,
			Message: fmt.Sprintf("unknown function %q", nameTok.text),
			Hint:    didYouMeanHint(nameTok.text, presetFunctionNames(), "the date-preset functions are "+presetFunctionList)}
	}
	if format, ok := p.resolveFormat(key); ok && format != "date" {
		return nil, "", &Error{Offset: nameTok.offset, Token: nameTok.raw,
			Message: fmt.Sprintf("%s() is a date preset, but property %q has format %q", nameTok.text, key, format)}
	}
	p.next() // consume (
	if countingPresets[nameTok.text] {
		operand := p.next()
		if operand.kind != tokNumber {
			return nil, "", &Error{Offset: operand.offset, Token: operand.raw,
				Message: fmt.Sprintf("%s takes a day count, e.g. %s(7)", nameTok.text, nameTok.text)}
		}
		count, err := strconv.ParseFloat(operand.text, 64)
		if err != nil || count != float64(int64(count)) || count < 0 || count > maxDayCount {
			return nil, "", &Error{Offset: operand.offset, Token: operand.raw,
				Message: fmt.Sprintf("%s takes a whole day count between 0 and %d", nameTok.text, maxDayCount)}
		}
		if closing := p.next(); closing.kind != tokRParen {
			return nil, "", &Error{Offset: closing.offset, Token: closing.raw,
				Message: fmt.Sprintf("expected ) to close %s(", nameTok.text)}
		}
		return count, preset, nil
	}
	if closing := p.next(); closing.kind != tokRParen {
		return nil, "", &Error{Offset: closing.offset, Token: closing.raw,
			Message: fmt.Sprintf("%s takes no arguments", nameTok.text)}
	}
	return nil, preset, nil
}

// stringValue applies the §6.2.1 value mapping to a quoted string: on a
// date-formatted property it must be an RFC 3339 date and converts to the
// structured form's unix number; on a select-shaped property it is an option
// NAME and is validated read-only against the space's options.
func (p *parser) stringValue(key string, tok token) (any, string, error) {
	format, formatKnown := p.resolveFormat(key)
	if formatKnown && format == "date" {
		sec, ok := parseDate(tok.text)
		if !ok {
			return nil, "", &Error{Offset: tok.offset, Token: tok.raw,
				Message: fmt.Sprintf("property %q is a date — %q is not an RFC 3339 date", key, tok.text),
				Hint:    `write dates as "2026-08-01" / "2026-08-01T15:00:00Z" or use a preset function like currentWeek()`}
		}
		return float64(sec), "", nil
	}
	if formatKnown && (format == "select" || format == "multiSelect") && p.opts.KnownOptions != nil {
		if names, ok := p.opts.KnownOptions(key); ok {
			if !containsString(names, tok.text) {
				return nil, "", &Error{Offset: tok.offset, Token: tok.raw,
					Message: fmt.Sprintf("property %q has no option named %q — a query never creates options", key, tok.text),
					Hint:    didYouMeanHint(tok.text, names, fmt.Sprintf("list them with GET /v2/spaces/{spaceId}/properties/%s/options", key)),
				}
			}
		}
	}
	return tok.text, "", nil
}

func (p *parser) resolveFormat(key string) (string, bool) {
	if p.opts.ResolveFormat == nil {
		return "", false
	}
	return p.opts.ResolveFormat(key)
}

// checkKey validates a property key against the reference set (when wired).
func (p *parser) checkKey(tok token) error {
	if p.opts.KnownKeys == nil {
		return nil
	}
	if containsString(p.opts.KnownKeys, tok.text) {
		return nil
	}
	known := append([]string(nil), p.opts.KnownKeys...)
	sort.Strings(known)
	const maxListed = 15
	listed := known
	suffix := ""
	if len(listed) > maxListed {
		suffix = fmt.Sprintf(", … (%d total)", len(known))
		listed = listed[:maxListed]
	}
	hint := didYouMeanHint(tok.text, known, "")
	// a known key the compact syntax cannot spell (hyphen, space, keyword
	// collision) needs explicit steering — repairing the string cannot work
	for _, suggestion := range closest(tok.text, known, 3) {
		if !bareWritable(suggestion) {
			steer := fmt.Sprintf("property key %q cannot be written in the compact filter string — use the structured filters array", suggestion)
			if hint == "" {
				hint = steer
			} else {
				hint += " — " + steer
			}
			break
		}
	}
	return &Error{Offset: tok.offset, Token: tok.raw,
		Message: fmt.Sprintf("unknown property key %q — known property keys: %s%s", tok.text, strings.Join(listed, ", "), suffix),
		Hint:    hint,
	}
}

// bareWritable reports whether a property key can be written as a bare
// identifier in the compact syntax: identifier characters only and not a
// reserved word.
func bareWritable(key string) bool {
	if key == "" || reservedWords[strings.ToLower(key)] {
		return false
	}
	for i, r := range key {
		if i == 0 && !isIdentStart(r) {
			return false
		}
		if i > 0 && !isIdentPart(r) {
			return false
		}
	}
	return true
}

//
// ---- emission ----
//

// emitNode is the §6.2 filter-node JSON shape, in canonical field order.
type emitNode struct {
	Operator   string     `json:"operator,omitempty"`
	Filters    []emitNode `json:"filters,omitempty"`
	Property   string     `json:"property,omitempty"`
	Condition  string     `json:"condition,omitempty"`
	Value      *any       `json:"value,omitempty"`
	DatePreset string     `json:"datePreset,omitempty"`
}

func emitOne(n *node) emitNode {
	if n.op != "" {
		out := emitNode{Operator: n.op, Filters: make([]emitNode, 0, len(n.children))}
		for _, c := range n.children {
			out.Filters = append(out.Filters, emitOne(c))
		}
		return out
	}
	out := emitNode{Property: n.property, Condition: n.condition, DatePreset: n.datePreset}
	if n.hasValue {
		v := n.value
		out.Value = &v
	}
	return out
}

// emit renders the root as the §6.2 top-level filters array: a top-level AND
// spreads into bare siblings (implicit AND); anything else is one node.
func emit(root *node) json.RawMessage {
	var nodes []emitNode
	if root.op == "and" {
		for _, c := range root.children {
			nodes = append(nodes, emitOne(c))
		}
	} else {
		nodes = []emitNode{emitOne(root)}
	}
	data, err := json.Marshal(nodes)
	if err != nil {
		// the emit structs are marshal-safe by construction
		panic(fmt.Sprintf("filterstring: emit: %v", err))
	}
	return data
}

//
// ---- helpers ----
//

// ParseDate exposes the parser's date parsing: RFC 3339 (with offsets and
// fractional seconds) and date-only strings (UTC midnight) → unix seconds.
// Consumers validating the STRUCTURED filter form reuse it so both request
// forms agree on what a date string is.
func ParseDate(s string) (int64, bool) { return parseDate(s) }

// parseDate mirrors the parent package's §3 date parsing: RFC 3339 (with
// offsets and fractional seconds) and date-only strings (UTC midnight).
func parseDate(s string) (int64, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix(), true
		}
	}
	return 0, false
}

func containsString(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

func presetFunctionNames() []string {
	out := make([]string, 0, len(datePresets))
	for name := range datePresets {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// didYouMeanHint picks the closest known names; fallback (possibly empty)
// when nothing is close.
func didYouMeanHint(input string, known []string, fallback string) string {
	suggestions := closest(input, known, 3)
	if len(suggestions) == 0 {
		return fallback
	}
	return "did you mean " + strings.Join(suggestions, ", ") + "?"
}

// closest ranks known names by similarity to input: case-insensitive
// equality, then prefix, then containment, then edit distance ≤ 2.
// Deterministic (rank, then alphabetical).
func closest(input string, known []string, max int) []string {
	in := strings.ToLower(input)
	type scored struct {
		name string
		rank int
	}
	var out []scored
	for _, k := range known {
		lk := strings.ToLower(k)
		switch {
		case lk == in:
			out = append(out, scored{k, 0})
		case strings.HasPrefix(lk, in) || strings.HasPrefix(in, lk):
			out = append(out, scored{k, 1})
		case strings.Contains(lk, in) || strings.Contains(in, lk):
			out = append(out, scored{k, 2})
		case editDistanceAtMost(lk, in, 2):
			out = append(out, scored{k, 3})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].rank != out[j].rank {
			return out[i].rank < out[j].rank
		}
		return out[i].name < out[j].name
	})
	if len(out) > max {
		out = out[:max]
	}
	names := make([]string, len(out))
	for i, sc := range out {
		names[i] = sc.name
	}
	return names
}

// editDistanceAtMost reports whether the Levenshtein distance between a and
// b is ≤ bound; long inputs never match (cost guard).
func editDistanceAtMost(a, b string, bound int) bool {
	ra, rb := []rune(a), []rune(b)
	if len(ra) > 64 || len(rb) > 64 {
		return false
	}
	diff := len(ra) - len(rb)
	if diff < 0 {
		diff = -diff
	}
	if diff > bound {
		return false
	}
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			m := prev[j] + 1
			if curr[j-1]+1 < m {
				m = curr[j-1] + 1
			}
			if prev[j-1]+cost < m {
				m = prev[j-1] + cost
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)] <= bound
}
