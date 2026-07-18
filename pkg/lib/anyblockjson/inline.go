package anyblockjson

// inline.go implements the §8 inline-markup codec: canonical rendering of
// text marks into the Markdown subset, and the inverse parser. The parser is
// the exact inverse of the renderer: it resolves emphasis with a
// deterministic delimiter stack (not CommonMark's delimiter-run algorithm),
// which is what makes Export ∘ Import byte-stable over arbitrarily
// overlapping mark ranges. All offsets are UTF-16 code units (§8.3).

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/text"
)

const objectLinkPrefix = "anytype://object?objectId="

// markNesting is the fixed outermost→innermost nesting order (§8.3 step 5).
var markNesting = []model.BlockContentTextMarkType{
	model.BlockContentTextMark_Mention,
	model.BlockContentTextMark_Object,
	model.BlockContentTextMark_Link,
	model.BlockContentTextMark_TextColor,
	model.BlockContentTextMark_BackgroundColor,
	model.BlockContentTextMark_Underscored,
	model.BlockContentTextMark_Strikethrough,
	model.BlockContentTextMark_Bold,
	model.BlockContentTextMark_Italic,
	model.BlockContentTextMark_Keyboard,
}

var markPriority = func() map[model.BlockContentTextMarkType]int {
	m := make(map[model.BlockContentTextMarkType]int, len(markNesting))
	for i, t := range markNesting {
		m[t] = i
	}
	return m
}()

func markNeedsParam(t model.BlockContentTextMarkType) bool {
	switch t {
	case model.BlockContentTextMark_Link,
		model.BlockContentTextMark_TextColor,
		model.BlockContentTextMark_BackgroundColor,
		model.BlockContentTextMark_Mention,
		model.BlockContentTextMark_Object,
		model.BlockContentTextMark_Emoji:
		return true
	}
	return false
}

// span is a mark over UTF-16 code-unit offsets.
type span struct {
	typ      model.BlockContentTextMarkType
	param    string
	from, to int
}

func sortSpans(s []span) {
	sort.SliceStable(s, func(i, j int) bool {
		if s[i].from != s[j].from {
			return s[i].from < s[j].from
		}
		if s[i].to != s[j].to {
			return s[i].to > s[j].to
		}
		return s[i].param < s[j].param
	})
}

// RenderInline serializes text and its marks into §8 inline Markdown.
func RenderInline(txt string, marks []*model.BlockContentTextMark) string {
	u16 := text.StrToUTF16(txt)
	spans := sanitizeSpans(u16, marks)
	u16, spans = materializeEmoji(u16, spans)
	spans = shrinkWhitespaceBoundaries(u16, spans)
	spans = resolveSameTypeOverlaps(spans)
	spans = splitEmphasisAtBoundaryWhitespace(u16, spans)
	return emitSegments(u16, spans)
}

// sanitizeSpans drops nil, zero-length, out-of-bounds and surrogate-splitting
// ranges, unknown mark types and empty params on param-carrying marks (§8.3
// step 1).
func sanitizeSpans(u16 []uint16, marks []*model.BlockContentTextMark) []span {
	spans := make([]span, 0, len(marks))
	for _, m := range marks {
		if m == nil || m.Range == nil {
			continue
		}
		from, to := int(m.Range.From), int(m.Range.To)
		if from < 0 || to > len(u16) || from >= to {
			continue
		}
		if splitsSurrogatePair(u16, from) || splitsSurrogatePair(u16, to) {
			continue
		}
		if _, known := markPriority[m.Type]; !known && m.Type != model.BlockContentTextMark_Emoji {
			continue
		}
		param := m.Param
		if markNeedsParam(m.Type) {
			if param == "" {
				continue
			}
		} else {
			// a param on a param-less mark type is noise; normalizing it to
			// empty lets equal-range marks merge (§8.3)
			param = ""
		}
		spans = append(spans, span{typ: m.Type, param: param, from: from, to: to})
	}
	return spans
}

func splitsSurrogatePair(u16 []uint16, i int) bool {
	if i <= 0 || i >= len(u16) {
		return false
	}
	return isHighSurrogate(u16[i-1]) && isLowSurrogate(u16[i])
}

func isHighSurrogate(u uint16) bool { return u >= 0xD800 && u <= 0xDBFF }
func isLowSurrogate(u uint16) bool  { return u >= 0xDC00 && u <= 0xDFFF }

// materializeEmoji splices each Emoji mark's emoji over its covered text
// (§8.1), adjusting the remaining marks' offsets. Overlapping emoji marks are
// truncated earlier-start-wins first (§8.3 step 3 semantics).
func materializeEmoji(u16 []uint16, spans []span) ([]uint16, []span) {
	var emoji, rest []span
	for _, s := range spans {
		if s.typ == model.BlockContentTextMark_Emoji {
			emoji = append(emoji, s)
		} else {
			rest = append(rest, s)
		}
	}
	if len(emoji) == 0 {
		return u16, rest
	}
	sortSpans(emoji)
	var acc []span
	for _, e := range emoji {
		for i := range acc {
			if e.from < acc[i].to {
				e.from = acc[i].to
			}
		}
		if e.from < e.to && !splitsSurrogatePair(u16, e.from) {
			acc = append(acc, e)
		}
	}
	for i := len(acc) - 1; i >= 0; i-- {
		e := acc[i]
		rep := text.StrToUTF16(e.param)
		nu := make([]uint16, 0, len(u16)+len(rep)-(e.to-e.from))
		nu = append(nu, u16[:e.from]...)
		nu = append(nu, rep...)
		nu = append(nu, u16[e.to:]...)
		u16 = nu
		for j := range rest {
			rest[j].from = adjustSplicePoint(rest[j].from, e.from, e.to, len(rep), true)
			rest[j].to = adjustSplicePoint(rest[j].to, e.from, e.to, len(rep), false)
		}
	}
	out := rest[:0]
	for _, s := range rest {
		if s.from < s.to {
			out = append(out, s)
		}
	}
	return u16, out
}

// adjustSplicePoint maps offset p across a splice of [f,t) by a replacement
// of length l. Points strictly inside the replaced range retract to exclude
// the replacement: starts land after it, ends before it.
func adjustSplicePoint(p, f, t, l int, isStart bool) int {
	switch {
	case p <= f:
		return p
	case p >= t:
		return p + l - (t - f)
	case isStart:
		return f + l
	default:
		return f
	}
}

func isMarkdownDelimited(t model.BlockContentTextMarkType) bool {
	switch t {
	case model.BlockContentTextMark_Bold,
		model.BlockContentTextMark_Italic,
		model.BlockContentTextMark_Strikethrough,
		model.BlockContentTextMark_Keyboard:
		return true
	}
	return false
}

// shrinkWhitespaceBoundaries shrinks Markdown-delimited marks past
// leading/trailing whitespace (§8.3 step 2).
func shrinkWhitespaceBoundaries(u16 []uint16, spans []span) []span {
	out := spans[:0]
	for _, s := range spans {
		if isMarkdownDelimited(s.typ) {
			for s.from < s.to && isWSUnit(u16, s.from) {
				s.from++
			}
			for s.to > s.from && isWSUnit(u16, s.to-1) {
				s.to--
			}
			if s.from >= s.to {
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

func isWSUnit(u16 []uint16, i int) bool {
	u := u16[i]
	if isHighSurrogate(u) || isLowSurrogate(u) {
		return false
	}
	return unicode.IsSpace(rune(u))
}

// resolveSameTypeOverlaps applies §8.3 step 3: same-type marks with equal
// params merge when overlapping or adjacent; with different params the
// earlier-starting mark wins and the later is truncated to start where the
// earlier ends. At equal starts the longer range wins (sort order).
func resolveSameTypeOverlaps(spans []span) []span {
	byType := make(map[model.BlockContentTextMarkType][]span)
	for _, s := range spans {
		byType[s.typ] = append(byType[s.typ], s)
	}
	var out []span
	for _, t := range markNesting {
		group := byType[t]
		if len(group) == 0 {
			continue
		}
		sortSpans(group)
		var acc []span
		for _, m := range group {
			dropped := false
			for i := range acc {
				a := &acc[i]
				if m.from > a.to {
					continue
				}
				if m.param == a.param {
					if m.to > a.to {
						a.to = m.to
					}
					dropped = true
					break
				}
				if m.from < a.to {
					m.from = a.to
					if m.from >= m.to {
						dropped = true
						break
					}
				}
			}
			if !dropped {
				acc = append(acc, m)
			}
		}
		out = append(out, acc...)
	}
	sortSpans(out)
	return out
}

func isEmphasisFamily(t model.BlockContentTextMarkType) bool {
	switch t {
	case model.BlockContentTextMark_Bold,
		model.BlockContentTextMark_Italic,
		model.BlockContentTextMark_Strikethrough:
		return true
	}
	return false
}

// splitEmphasisAtBoundaryWhitespace removes from emphasis-family marks every
// whitespace run that a stack-outer mark's endpoint touches. Such an endpoint
// forces the emphasis delimiter to close/reopen inside the run, and an
// emphasis delimiter emitted against whitespace cannot re-parse (flanking).
// Whitespace carries no visible styling for these types, so splitting the
// span around the run is a rendering no-op. Endpoints of inner marks are
// harmless: the shared-prefix emission keeps the outer emphasis open across
// them. Runs to a fixpoint because each split introduces new endpoints.
func splitEmphasisAtBoundaryWhitespace(u16 []uint16, spans []span) []span {
	type sortKey struct {
		prio  int
		param string
	}
	keyOf := func(s span) sortKey { return sortKey{markPriority[s.typ], s.param} }
	outerBefore := func(a, b sortKey) bool {
		if a.prio != b.prio {
			return a.prio < b.prio
		}
		return a.param < b.param
	}
	for {
		changed := false
		out := make([]span, 0, len(spans))
		for _, s := range spans {
			if !isEmphasisFamily(s.typ) {
				out = append(out, s)
				continue
			}
			sk := keyOf(s)
			cutsRun := func(i, j int) bool {
				for _, t := range spans {
					if !outerBefore(keyOf(t), sk) {
						continue
					}
					if (t.from >= i && t.from <= j) || (t.to >= i && t.to <= j) {
						return true
					}
				}
				return false
			}
			cur := s.from
			for i := s.from; i < s.to; {
				if !isWSUnit(u16, i) {
					i++
					continue
				}
				j := i
				for j < s.to && isWSUnit(u16, j) {
					j++
				}
				if cutsRun(i, j) {
					if cur < i {
						out = append(out, span{typ: s.typ, param: s.param, from: cur, to: i})
					}
					cur = j
					changed = true
				}
				i = j
			}
			// spans are already whitespace-shrunk, so cur < s.to always holds
			out = append(out, span{typ: s.typ, param: s.param, from: cur, to: s.to})
		}
		spans = out
		if !changed {
			sortSpans(spans)
			return spans
		}
	}
}

// stackItem identifies one active mark on a segment.
type stackItem struct {
	typ   model.BlockContentTextMarkType
	param string
}

// treeNode / treeKid form the well-nested render tree built from segment
// stacks (§8.3 steps 4–5).
type treeNode struct {
	item stackItem
	root bool
	kids []*treeKid
}

type treeKid struct {
	node     *treeNode // nil for a text segment
	from, to int
	atBOL    bool
	atEOL    bool
}

func emitSegments(u16 []uint16, spans []span) string {
	root := &treeNode{root: true}
	if len(u16) > 0 {
		bounds := collectBounds(len(u16), spans)
		path := []*treeNode{root}
		for i := 0; i+1 < len(bounds); i++ {
			segFrom, segTo := bounds[i], bounds[i+1]
			target := activeItems(spans, segFrom, segTo)
			l := 0
			for l < len(target) && l+1 < len(path) && path[l+1].item == target[l] {
				l++
			}
			path = path[:l+1]
			for _, it := range target[l:] {
				n := &treeNode{item: it}
				top := path[len(path)-1]
				top.kids = append(top.kids, &treeKid{node: n})
				path = append(path, n)
			}
			top := path[len(path)-1]
			top.kids = append(top.kids, &treeKid{
				from:  segFrom,
				to:    segTo,
				atBOL: segFrom == 0 && len(target) == 0,
				atEOL: segTo == len(u16) && len(target) == 0,
			})
		}
	}
	var b strings.Builder
	renderNode(&b, u16, root, false)
	return b.String()
}

func collectBounds(length int, spans []span) []int {
	set := map[int]struct{}{0: {}, length: {}}
	for _, s := range spans {
		set[s.from] = struct{}{}
		set[s.to] = struct{}{}
	}
	bounds := make([]int, 0, len(set))
	for b := range set {
		bounds = append(bounds, b)
	}
	sort.Ints(bounds)
	return bounds
}

func activeItems(spans []span, from, to int) []stackItem {
	var items []stackItem
	for _, s := range spans {
		if s.from <= from && s.to >= to {
			items = append(items, stackItem{typ: s.typ, param: s.param})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		pi, pj := markPriority[items[i].typ], markPriority[items[j].typ]
		if pi != pj {
			return pi < pj
		}
		return items[i].param < items[j].param
	})
	return items
}

func renderNode(b *strings.Builder, u16 []uint16, n *treeNode, inLabel bool) {
	renderKids := func(label bool) {
		for _, k := range n.kids {
			if k.node != nil {
				renderNode(b, u16, k.node, label)
			} else {
				b.WriteString(escapeProse(text.UTF16ToStr(u16[k.from:k.to]), k.atBOL, k.atEOL, label))
			}
		}
	}
	if n.root {
		renderKids(inLabel)
		return
	}
	switch n.item.typ {
	case model.BlockContentTextMark_Mention:
		b.WriteString(`<mention objectId="` + escapeAttr(n.item.param) + `">`)
		renderKids(inLabel)
		b.WriteString(`</mention>`)
	case model.BlockContentTextMark_Object:
		b.WriteByte('[')
		renderKids(true)
		b.WriteString("](" + escapeDest(objectLinkPrefix+n.item.param) + ")")
	case model.BlockContentTextMark_Link:
		b.WriteByte('[')
		renderKids(true)
		b.WriteString("](" + escapeDest(n.item.param) + ")")
	case model.BlockContentTextMark_TextColor:
		// Coincident color+background ranges combine into one tag (§8.1):
		// in the tree that is a TextColor node whose sole child is a
		// BackgroundColor node.
		if len(n.kids) == 1 && n.kids[0].node != nil &&
			n.kids[0].node.item.typ == model.BlockContentTextMark_BackgroundColor {
			bg := n.kids[0].node
			b.WriteString(`<font color="` + escapeAttr(n.item.param) + `" background="` + escapeAttr(bg.item.param) + `">`)
			renderNode(b, u16, &treeNode{root: true, kids: bg.kids}, inLabel)
			b.WriteString(`</font>`)
			return
		}
		b.WriteString(`<font color="` + escapeAttr(n.item.param) + `">`)
		renderKids(inLabel)
		b.WriteString(`</font>`)
	case model.BlockContentTextMark_BackgroundColor:
		b.WriteString(`<font background="` + escapeAttr(n.item.param) + `">`)
		renderKids(inLabel)
		b.WriteString(`</font>`)
	case model.BlockContentTextMark_Underscored:
		b.WriteString(`<u>`)
		renderKids(inLabel)
		b.WriteString(`</u>`)
	case model.BlockContentTextMark_Strikethrough:
		b.WriteString(`~~`)
		renderKids(inLabel)
		b.WriteString(`~~`)
	case model.BlockContentTextMark_Bold:
		b.WriteString(`**`)
		renderKids(inLabel)
		b.WriteString(`**`)
	case model.BlockContentTextMark_Italic:
		b.WriteString(`*`)
		renderKids(inLabel)
		b.WriteString(`*`)
	case model.BlockContentTextMark_Keyboard:
		// Keyboard is innermost: its content is always a single text segment.
		var content strings.Builder
		for _, k := range n.kids {
			if k.node == nil {
				content.WriteString(text.UTF16ToStr(u16[k.from:k.to]))
			}
		}
		writeCodeSpan(b, content.String())
	}
}

// writeCodeSpan emits a CommonMark code span: the delimiter is the shortest
// backtick run absent from the content, space-padded when the content starts
// or ends with a backtick or would trigger the strip rule (§8.2).
func writeCodeSpan(b *strings.Builder, content string) {
	runs := map[int]bool{}
	run := 0
	for _, r := range content {
		if r == '`' {
			run++
		} else if run > 0 {
			runs[run] = true
			run = 0
		}
	}
	if run > 0 {
		runs[run] = true
	}
	n := 1
	for runs[n] {
		n++
	}
	delim := strings.Repeat("`", n)
	pad := strings.HasPrefix(content, "`") || strings.HasSuffix(content, "`")
	if !pad && len(content) > 0 {
		first, last := content[0], content[len(content)-1]
		startsWS := first == ' ' || first == '\n'
		endsWS := last == ' ' || last == '\n'
		if startsWS && endsWS && strings.Trim(content, " \n") != "" {
			pad = true
		}
	}
	b.WriteString(delim)
	if pad {
		b.WriteByte(' ')
	}
	b.WriteString(content)
	if pad {
		b.WriteByte(' ')
	}
	b.WriteString(delim)
}

const (
	edgeWS = iota
	edgePunct
	edgeWord
)

func classifyRune(r rune) int {
	if unicode.IsSpace(r) {
		return edgeWS
	}
	if isPunctRune(r) {
		return edgePunct
	}
	return edgeWord
}

// escapeProse writes a text segment with canonical minimal escaping (§8.2).
// atBOL/atEOL report whether the segment starts/ends the whole rendered
// string; at internal segment edges the neighbor is a delimiter and is
// treated as punctuation.
func escapeProse(s string, atBOL, atEOL, inLabel bool) string {
	rs := []rune(s)
	var b strings.Builder
	kindAt := func(i int) int {
		if i < 0 {
			if atBOL {
				return edgeWS
			}
			return edgePunct
		}
		if i >= len(rs) {
			if atEOL {
				return edgeWS
			}
			return edgePunct
		}
		return classifyRune(rs[i])
	}
	for i, r := range rs {
		prev, next := kindAt(i-1), kindAt(i+1)
		switch r {
		case '\\':
			if i+1 < len(rs) {
				if isASCIIPunct(rs[i+1]) {
					b.WriteString(`\\`)
				} else {
					b.WriteByte('\\')
				}
			} else if atEOL {
				b.WriteByte('\\')
			} else {
				// a trailing backslash would escape the following delimiter
				b.WriteString(`\\`)
			}
		case '`':
			b.WriteString("\\`")
		case '*':
			if prev == edgeWS && next == edgeWS {
				b.WriteByte('*')
			} else {
				b.WriteString(`\*`)
			}
		case '_':
			canOpen := next != edgeWS && prev != edgeWord
			canClose := prev != edgeWS && next != edgeWord
			if canOpen || canClose {
				b.WriteString(`\_`)
			} else {
				b.WriteByte('_')
			}
		case '~':
			adjacent := (i > 0 && rs[i-1] == '~') || (i+1 < len(rs) && rs[i+1] == '~') ||
				(i == 0 && !atBOL) || (i == len(rs)-1 && !atEOL)
			if adjacent {
				b.WriteString(`\~`)
			} else {
				b.WriteByte('~')
			}
		case '[':
			// always escaped: a bare '[' could assemble a false link with
			// text from later segments, which no local lookahead can rule out
			b.WriteString(`\[`)
		case ']':
			if inLabel {
				b.WriteString(`\]`)
			} else {
				b.WriteByte(']')
			}
		case '<':
			if tagLikeAhead(rs[i:]) {
				b.WriteString(`\<`)
			} else {
				b.WriteByte('<')
			}
		case '&':
			if entityAhead(rs[i:]) {
				b.WriteString(`\&`)
			} else {
				b.WriteByte('&')
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// tagLikeAhead reports whether rs starts a whitelisted tag prefix — the only
// case where a literal '<' needs escaping.
func tagLikeAhead(rs []rune) bool {
	j := 1
	if j < len(rs) && rs[j] == '/' {
		j++
	}
	start := j
	for j < len(rs) && isASCIILetter(rs[j]) {
		j++
	}
	name := string(rs[start:j])
	if name != "u" && name != "font" && name != "mention" {
		return false
	}
	if j >= len(rs) {
		return false
	}
	return rs[j] == '>' || rs[j] == '/' || unicode.IsSpace(rs[j])
}

func entityAhead(rs []rune) bool {
	_, _, ok := parseEntity(rs)
	return ok
}

func escapeAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// escapeDest renders a link destination: bare with \-escaped specials, or
// angle-wrapped when it contains whitespace.
func escapeDest(dest string) string {
	needsAngle := false
	for _, r := range dest {
		if unicode.IsSpace(r) {
			needsAngle = true
			break
		}
	}
	var b strings.Builder
	if needsAngle {
		b.WriteByte('<')
		for _, r := range dest {
			switch r {
			case '\\', '<', '>', '&':
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
		b.WriteByte('>')
		return b.String()
	}
	for _, r := range dest {
		switch r {
		case '\\', '(', ')', '&':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

//
// ---- parsing ----
//

// InlineError is a grammar error in a text string's inline markup (§12).
type InlineError struct {
	Msg     string
	Snippet string
}

func (e *InlineError) Error() string {
	if e.Snippet == "" {
		return e.Msg
	}
	return fmt.Sprintf("%s near %q", e.Msg, e.Snippet)
}

func inlineErr(rs []rune, pos int, msg string) error {
	end := pos + 24
	if end > len(rs) {
		end = len(rs)
	}
	start := pos
	if start > len(rs) {
		start = len(rs)
	}
	return &InlineError{Msg: msg, Snippet: string(rs[start:end])}
}

// ParseInline parses §8 inline Markdown back into plain text and marks with
// UTF-16 code-unit ranges.
func ParseInline(md string) (string, []*model.BlockContentTextMark, error) {
	rs := []rune(md)
	toks, err := tokenizeInline(rs)
	if err != nil {
		return "", nil, err
	}
	ib := &inlineBuilder{}
	if err := resolveTokens(toks, ib); err != nil {
		return "", nil, err
	}
	marks := canonicalizeMarks(ib.marks)
	return text.UTF16ToStr(ib.out), marks, nil
}

type tokenKind int

const (
	tokText tokenKind = iota
	tokDelim
	tokCode
	tokTag
	tokLink
)

type token struct {
	kind              tokenKind
	txt               string // tokText: literal (decoded) text
	ch                rune   // tokDelim: delimiter char
	n                 int    // tokDelim: run length
	canOpen, canClose bool
	content           string            // tokCode
	tagName           string            // tokTag
	closing           bool              // tokTag
	attrs             map[string]string // tokTag
	label             []token           // tokLink
	dest              string            // tokLink
}

func tokenizeInline(rs []rune) ([]token, error) {
	var toks []token
	appendText := func(s string) {
		if s == "" {
			return
		}
		if len(toks) > 0 && toks[len(toks)-1].kind == tokText {
			toks[len(toks)-1].txt += s
		} else {
			toks = append(toks, token{kind: tokText, txt: s})
		}
	}
	i := 0
	for i < len(rs) {
		switch r := rs[i]; r {
		case '\\':
			if i+1 < len(rs) && isASCIIPunct(rs[i+1]) {
				appendText(string(rs[i+1]))
				i += 2
			} else {
				appendText(`\`)
				i++
			}
		case '&':
			if dec, size, ok := parseEntity(rs[i:]); ok {
				appendText(dec)
				i += size
			} else {
				appendText("&")
				i++
			}
		case '`':
			n := runLen(rs, i, '`')
			if end, ok := findBacktickClose(rs, i+n, n); ok {
				toks = append(toks, token{kind: tokCode, content: stripCodePadding(string(rs[i+n : end]))})
				i = end + n
			} else {
				appendText(strings.Repeat("`", n))
				i += n
			}
		case '<':
			tok, size, isTag, err := parseTag(rs, i)
			if err != nil {
				return nil, err
			}
			if !isTag {
				appendText("<")
				i++
				break
			}
			if tok != nil {
				toks = append(toks, *tok)
			}
			i += size
		case '[':
			tok, size, ok, err := parseLink(rs, i)
			if err != nil {
				return nil, err
			}
			if ok {
				toks = append(toks, *tok)
				i += size
			} else {
				appendText("[")
				i++
			}
		case '*', '_':
			n := runLen(rs, i, r)
			canOpen, canClose := delimFlanking(rs, i, n, r)
			if canOpen || canClose {
				toks = append(toks, token{kind: tokDelim, ch: r, n: n, canOpen: canOpen, canClose: canClose})
			} else {
				appendText(strings.Repeat(string(r), n))
			}
			i += n
		case '~':
			n := runLen(rs, i, '~')
			if n == 2 {
				canOpen, canClose := delimFlanking(rs, i, n, '~')
				if canOpen || canClose {
					toks = append(toks, token{kind: tokDelim, ch: '~', n: n, canOpen: canOpen, canClose: canClose})
				} else {
					appendText("~~")
				}
			} else {
				appendText(strings.Repeat("~", n))
			}
			i += n
		default:
			appendText(string(r))
			i++
		}
	}
	return toks, nil
}

func runLen(rs []rune, i int, ch rune) int {
	n := 0
	for i+n < len(rs) && rs[i+n] == ch {
		n++
	}
	return n
}

// delimFlanking computes flanking-lite open/close capability for a delimiter
// run: '*'/'~' need a non-space neighbor on the inside; '_' additionally must
// not sit inside a word (intraword underscores stay literal).
func delimFlanking(rs []rune, i, n int, ch rune) (canOpen, canClose bool) {
	prev, next := edgeWS, edgeWS
	if i > 0 {
		prev = classifyRune(rs[i-1])
	}
	if i+n < len(rs) {
		next = classifyRune(rs[i+n])
	}
	canOpen = next != edgeWS
	canClose = prev != edgeWS
	if ch == '_' {
		canOpen = canOpen && prev != edgeWord
		canClose = canClose && next != edgeWord
	}
	return canOpen, canClose
}

func findBacktickClose(rs []rune, from, n int) (int, bool) {
	j := from
	for j < len(rs) {
		if rs[j] == '`' {
			m := runLen(rs, j, '`')
			if m == n {
				return j, true
			}
			j += m
		} else {
			j++
		}
	}
	return 0, false
}

func stripCodePadding(content string) string {
	if len(content) < 2 {
		return content
	}
	first, last := content[0], content[len(content)-1]
	startsWS := first == ' ' || first == '\n'
	endsWS := last == ' ' || last == '\n'
	if startsWS && endsWS && strings.Trim(content, " \n") != "" {
		return content[1 : len(content)-1]
	}
	return content
}

var namedEntities = map[string]string{
	"lt": "<", "gt": ">", "amp": "&", "quot": `"`, "apos": "'", "nbsp": " ",
}

// parseEntity decodes an HTML entity at the start of rs, returning the
// decoded text and consumed length.
func parseEntity(rs []rune) (string, int, bool) {
	if len(rs) < 3 || rs[0] != '&' {
		return "", 0, false
	}
	if rs[1] == '#' {
		j := 2
		hex := false
		if j < len(rs) && (rs[j] == 'x' || rs[j] == 'X') {
			hex = true
			j++
		}
		start := j
		var v int64
		for j < len(rs) && j-start < 7 {
			d := digitVal(rs[j], hex)
			if d < 0 {
				break
			}
			v = v*int64(base(hex)) + int64(d)
			j++
		}
		if j == start || j >= len(rs) || rs[j] != ';' {
			return "", 0, false
		}
		if v == 0 || v > 0x10FFFF || (v >= 0xD800 && v <= 0xDFFF) {
			return "", 0, false
		}
		return string(rune(v)), j + 1, true
	}
	j := 1
	for j < len(rs) && j < 12 && isASCIILetter(rs[j]) {
		j++
	}
	if j >= len(rs) || rs[j] != ';' {
		return "", 0, false
	}
	dec, ok := namedEntities[string(rs[1:j])]
	if !ok {
		return "", 0, false
	}
	return dec, j + 1, true
}

func digitVal(r rune, hex bool) int {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0')
	case hex && r >= 'a' && r <= 'f':
		return int(r-'a') + 10
	case hex && r >= 'A' && r <= 'F':
		return int(r-'A') + 10
	}
	return -1
}

func base(hex bool) int {
	if hex {
		return 16
	}
	return 10
}

func decodeEntities(s string) string {
	if !strings.ContainsRune(s, '&') {
		return s
	}
	rs := []rune(s)
	var b strings.Builder
	for i := 0; i < len(rs); {
		if rs[i] == '&' {
			if dec, size, ok := parseEntity(rs[i:]); ok {
				b.WriteString(dec)
				i += size
				continue
			}
		}
		b.WriteRune(rs[i])
		i++
	}
	return b.String()
}

// parseTag parses a whitelisted inline tag at rs[i]. Returns isTag=false when
// the '<' does not start a whitelisted tag name (the '<' is then literal);
// once a whitelisted name is recognized, malformed syntax is an error (§12).
// A self-closing tag is zero-length and returns a nil token (dropped, §8.1).
func parseTag(rs []rune, i int) (*token, int, bool, error) {
	j := i + 1
	closing := false
	if j < len(rs) && rs[j] == '/' {
		closing = true
		j++
	}
	nameStart := j
	for j < len(rs) && isASCIILetter(rs[j]) {
		j++
	}
	name := string(rs[nameStart:j])
	if name != "u" && name != "font" && name != "mention" {
		return nil, 0, false, nil
	}
	if j >= len(rs) || !(rs[j] == '>' || rs[j] == '/' || unicode.IsSpace(rs[j])) {
		return nil, 0, false, nil
	}
	attrs := map[string]string{}
	selfClose := false
	for {
		for j < len(rs) && unicode.IsSpace(rs[j]) {
			j++
		}
		if j >= len(rs) {
			return nil, 0, false, inlineErr(rs, i, fmt.Sprintf("unterminated <%s> tag", name))
		}
		if rs[j] == '/' {
			j++
			if j >= len(rs) || rs[j] != '>' {
				return nil, 0, false, inlineErr(rs, i, fmt.Sprintf("malformed <%s> tag", name))
			}
			selfClose = true
			j++
			break
		}
		if rs[j] == '>' {
			j++
			break
		}
		if closing {
			return nil, 0, false, inlineErr(rs, i, fmt.Sprintf("closing </%s> tag with attributes", name))
		}
		attrStart := j
		for j < len(rs) && isASCIILetter(rs[j]) {
			j++
		}
		attrName := string(rs[attrStart:j])
		if attrName == "" {
			return nil, 0, false, inlineErr(rs, i, fmt.Sprintf("malformed <%s> tag", name))
		}
		for j < len(rs) && unicode.IsSpace(rs[j]) {
			j++
		}
		if j >= len(rs) || rs[j] != '=' {
			return nil, 0, false, inlineErr(rs, i, fmt.Sprintf("attribute %q in <%s> tag needs a quoted value", attrName, name))
		}
		j++
		for j < len(rs) && unicode.IsSpace(rs[j]) {
			j++
		}
		if j >= len(rs) || (rs[j] != '"' && rs[j] != '\'') {
			return nil, 0, false, inlineErr(rs, i, fmt.Sprintf("attribute %q in <%s> tag needs a quoted value", attrName, name))
		}
		quote := rs[j]
		j++
		valStart := j
		for j < len(rs) && rs[j] != quote {
			j++
		}
		if j >= len(rs) {
			return nil, 0, false, inlineErr(rs, i, fmt.Sprintf("unterminated attribute value in <%s> tag", name))
		}
		if _, dup := attrs[attrName]; dup {
			return nil, 0, false, inlineErr(rs, i, fmt.Sprintf("duplicate attribute %q in <%s> tag", attrName, name))
		}
		attrs[attrName] = decodeEntities(string(rs[valStart:j]))
		j++
	}
	if !closing {
		switch name {
		case "u":
			if len(attrs) > 0 {
				return nil, 0, false, inlineErr(rs, i, "unexpected attribute on <u> tag")
			}
		case "font":
			for k := range attrs {
				if k != "color" && k != "background" {
					return nil, 0, false, inlineErr(rs, i, fmt.Sprintf("unknown attribute %q on <font> tag", k))
				}
			}
			if len(attrs) == 0 {
				return nil, 0, false, inlineErr(rs, i, "<font> tag needs a color or background attribute")
			}
		case "mention":
			for k := range attrs {
				if k != "objectId" {
					return nil, 0, false, inlineErr(rs, i, fmt.Sprintf("unknown attribute %q on <mention> tag", k))
				}
			}
			if _, ok := attrs["objectId"]; !ok {
				return nil, 0, false, inlineErr(rs, i, "<mention> tag needs an objectId attribute")
			}
		}
	} else if selfClose {
		return nil, 0, false, inlineErr(rs, i, fmt.Sprintf("malformed </%s> tag", name))
	}
	if selfClose {
		// zero-length tag: dropped
		return nil, j - i, true, nil
	}
	return &token{kind: tokTag, tagName: name, closing: closing, attrs: attrs}, j - i, true, nil
}

// scanLink matches the [label](dest) pattern at rs[i] without tokenizing the
// label. Shared by the parser and the renderer's escape decision.
func scanLink(rs []rune, i int) (labelEnd int, dest string, size int, ok bool) {
	j := i + 1
	depth := 0
	labelEnd = -1
	for j < len(rs) {
		c := rs[j]
		if c == '\\' && j+1 < len(rs) && isASCIIPunct(rs[j+1]) {
			j += 2
			continue
		}
		if c == '`' {
			n := runLen(rs, j, '`')
			if end, found := findBacktickClose(rs, j+n, n); found {
				j = end + n
			} else {
				j += n
			}
			continue
		}
		if c == '[' {
			depth++
		} else if c == ']' {
			if depth == 0 {
				labelEnd = j
				break
			}
			depth--
		}
		j++
	}
	if labelEnd < 0 {
		return 0, "", 0, false
	}
	k := labelEnd + 1
	if k >= len(rs) || rs[k] != '(' {
		return 0, "", 0, false
	}
	k++
	for k < len(rs) && unicode.IsSpace(rs[k]) {
		k++
	}
	if k >= len(rs) {
		return 0, "", 0, false
	}
	var db strings.Builder
	if rs[k] == '<' {
		k++
		for {
			if k >= len(rs) {
				return 0, "", 0, false
			}
			c := rs[k]
			if c == '>' {
				k++
				break
			}
			if c == '\\' && k+1 < len(rs) && isASCIIPunct(rs[k+1]) {
				db.WriteRune(rs[k+1])
				k += 2
				continue
			}
			if c == '&' {
				if dec, esize, eok := parseEntity(rs[k:]); eok {
					db.WriteString(dec)
					k += esize
					continue
				}
			}
			db.WriteRune(c)
			k++
		}
	} else {
		parens := 0
		for {
			if k >= len(rs) {
				return 0, "", 0, false
			}
			c := rs[k]
			if unicode.IsSpace(c) {
				break
			}
			if c == ')' {
				if parens == 0 {
					break
				}
				parens--
			} else if c == '(' {
				parens++
			}
			if c == '\\' && k+1 < len(rs) && isASCIIPunct(rs[k+1]) {
				db.WriteRune(rs[k+1])
				k += 2
				continue
			}
			if c == '&' {
				if dec, esize, eok := parseEntity(rs[k:]); eok {
					db.WriteString(dec)
					k += esize
					continue
				}
			}
			db.WriteRune(c)
			k++
		}
	}
	for k < len(rs) && unicode.IsSpace(rs[k]) {
		k++
	}
	if k >= len(rs) || rs[k] != ')' {
		return 0, "", 0, false
	}
	return labelEnd, db.String(), k + 1 - i, true
}

func parseLink(rs []rune, i int) (*token, int, bool, error) {
	labelEnd, dest, size, ok := scanLink(rs, i)
	if !ok {
		return nil, 0, false, nil
	}
	labelToks, err := tokenizeInline(rs[i+1 : labelEnd])
	if err != nil {
		return nil, 0, false, err
	}
	return &token{kind: tokLink, label: labelToks, dest: dest}, size, true, nil
}

//
// ---- resolution ----
//

type entryKind int

const (
	entryEmph entryKind = iota
	entryTag
)

type openEntry struct {
	kind     entryKind
	ch       rune
	width    int
	markType model.BlockContentTextMarkType
	tagName  string
	attrs    map[string]string
	start16  int
}

func (e *openEntry) rawDelim() string {
	return strings.Repeat(string(e.ch), e.width)
}

type inlineBuilder struct {
	out   []uint16
	marks []*model.BlockContentTextMark
}

func (ib *inlineBuilder) appendString(s string) {
	ib.out = append(ib.out, text.StrToUTF16(s)...)
}

// insertLiteral splices literal text at pos (used when an unmatched opening
// delimiter is demoted back to text), shifting affected mark offsets.
func (ib *inlineBuilder) insertLiteral(pos int, s string) {
	ins := text.StrToUTF16(s)
	out := make([]uint16, 0, len(ib.out)+len(ins))
	out = append(out, ib.out[:pos]...)
	out = append(out, ins...)
	out = append(out, ib.out[pos:]...)
	ib.out = out
	for _, m := range ib.marks {
		if int(m.Range.From) >= pos {
			m.Range.From += int32(len(ins))
		}
		if int(m.Range.To) > pos {
			m.Range.To += int32(len(ins))
		}
	}
}

func (ib *inlineBuilder) addMark(typ model.BlockContentTextMarkType, param string, from, to int) {
	if from >= to {
		return
	}
	if markNeedsParam(typ) && param == "" {
		return
	}
	ib.marks = append(ib.marks, &model.BlockContentTextMark{
		Range: &model.Range{From: int32(from), To: int32(to)},
		Type:  typ,
		Param: param,
	})
}

// resolveTokens turns a token stream into text and marks with a delimiter
// stack. Unmatched emphasis delimiters demote to literal text; unmatched or
// misnested whitelisted tags are errors.
func resolveTokens(toks []token, ib *inlineBuilder) error {
	var stack []openEntry
	for ti := range toks {
		t := &toks[ti]
		switch t.kind {
		case tokText:
			ib.appendString(t.txt)
		case tokCode:
			start := len(ib.out)
			ib.appendString(t.content)
			ib.addMark(model.BlockContentTextMark_Keyboard, "", start, len(ib.out))
		case tokDelim:
			resolveDelimRun(&stack, ib, t)
		case tokTag:
			if !t.closing {
				stack = append(stack, openEntry{kind: entryTag, tagName: t.tagName, attrs: t.attrs, start16: len(ib.out)})
				break
			}
			idx := -1
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].kind == entryTag {
					if stack[i].tagName != t.tagName {
						return &InlineError{Msg: fmt.Sprintf("misnested tags: </%s> closes across <%s>", t.tagName, stack[i].tagName)}
					}
					idx = i
					break
				}
			}
			if idx < 0 {
				return &InlineError{Msg: fmt.Sprintf("unmatched closing </%s> tag", t.tagName)}
			}
			for i := len(stack) - 1; i > idx; i-- {
				ib.insertLiteral(stack[i].start16, stack[i].rawDelim())
			}
			e := stack[idx]
			stack = stack[:idx]
			emitTagMarks(ib, e)
		case tokLink:
			start := len(ib.out)
			if err := resolveTokens(t.label, ib); err != nil {
				return err
			}
			if strings.HasPrefix(t.dest, objectLinkPrefix) {
				ib.addMark(model.BlockContentTextMark_Object, t.dest[len(objectLinkPrefix):], start, len(ib.out))
			} else {
				ib.addMark(model.BlockContentTextMark_Link, t.dest, start, len(ib.out))
			}
		}
	}
	for i := len(stack) - 1; i >= 0; i-- {
		e := stack[i]
		if e.kind == entryTag {
			return &InlineError{Msg: fmt.Sprintf("unclosed <%s> tag", e.tagName)}
		}
		ib.insertLiteral(e.start16, e.rawDelim())
	}
	return nil
}

func emitTagMarks(ib *inlineBuilder, e openEntry) {
	end := len(ib.out)
	switch e.tagName {
	case "u":
		ib.addMark(model.BlockContentTextMark_Underscored, "", e.start16, end)
	case "font":
		ib.addMark(model.BlockContentTextMark_TextColor, e.attrs["color"], e.start16, end)
		ib.addMark(model.BlockContentTextMark_BackgroundColor, e.attrs["background"], e.start16, end)
	case "mention":
		ib.addMark(model.BlockContentTextMark_Mention, e.attrs["objectId"], e.start16, end)
	}
}

// resolveDelimRun consumes an emphasis/strikethrough delimiter run against
// the stack: close the top entry while it matches, open with the remainder,
// demote what can do neither to literal text.
func resolveDelimRun(stack *[]openEntry, ib *inlineBuilder, t *token) {
	n := t.n
	for n > 0 {
		if t.canClose && len(*stack) > 0 {
			top := &(*stack)[len(*stack)-1]
			if top.kind == entryEmph && top.ch == t.ch && top.width <= n {
				ib.addMark(top.markType, "", top.start16, len(ib.out))
				n -= top.width
				*stack = (*stack)[:len(*stack)-1]
				continue
			}
		}
		if t.canOpen {
			e := openEntry{kind: entryEmph, ch: t.ch, start16: len(ib.out)}
			switch {
			case t.ch == '~':
				e.width, e.markType = 2, model.BlockContentTextMark_Strikethrough
			case n >= 2:
				e.width, e.markType = 2, model.BlockContentTextMark_Bold
			default:
				e.width, e.markType = 1, model.BlockContentTextMark_Italic
			}
			*stack = append(*stack, e)
			n -= e.width
			continue
		}
		ib.appendString(strings.Repeat(string(t.ch), n))
		n = 0
	}
}

// canonicalizeMarks merges same-type same-param overlapping/adjacent ranges
// and sorts marks into the canonical order (from asc, to desc, nesting
// priority, param).
func canonicalizeMarks(marks []*model.BlockContentTextMark) []*model.BlockContentTextMark {
	if len(marks) == 0 {
		return nil
	}
	type key struct {
		typ   model.BlockContentTextMarkType
		param string
	}
	groups := make(map[key][]*model.BlockContentTextMark)
	var order []key
	for _, m := range marks {
		k := key{m.Type, m.Param}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], m)
	}
	var out []*model.BlockContentTextMark
	for _, k := range order {
		g := groups[k]
		sort.SliceStable(g, func(i, j int) bool { return g[i].Range.From < g[j].Range.From })
		merged := []*model.BlockContentTextMark{g[0]}
		for _, m := range g[1:] {
			last := merged[len(merged)-1]
			if m.Range.From <= last.Range.To {
				if m.Range.To > last.Range.To {
					last.Range.To = m.Range.To
				}
			} else {
				merged = append(merged, m)
			}
		}
		out = append(out, merged...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Range.From != b.Range.From {
			return a.Range.From < b.Range.From
		}
		if a.Range.To != b.Range.To {
			return a.Range.To > b.Range.To
		}
		if markPriority[a.Type] != markPriority[b.Type] {
			return markPriority[a.Type] < markPriority[b.Type]
		}
		return a.Param < b.Param
	})
	return out
}

func isASCIILetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isASCIIPunct(r rune) bool {
	return strings.ContainsRune("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", r)
}

func isPunctRune(r rune) bool {
	return isASCIIPunct(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
}
