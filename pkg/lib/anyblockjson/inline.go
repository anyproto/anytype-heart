package anyblockjson

// inline.go implements the §8 inline-markup codec: canonical rendering of
// text marks into the Markdown subset, and the inverse parser. The parser is
// the exact inverse of the renderer: it resolves emphasis with a
// deterministic delimiter stack (not CommonMark's delimiter-run algorithm),
// which is what makes Export ∘ Import byte-stable over arbitrarily
// overlapping mark ranges. All offsets are UTF-16 code units (§8.3).

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/text"
)

// An Object mark's target renders as Anytype's deep link. The form is exact:
// scheme "anytype", host "object", and a single "object_id" query parameter —
// nothing else. Matching it by string prefix instead would take everything
// after "object_id=" as the id, so the platform's own two-parameter link
// (core/block/export/writer.go) would yield the id "<id>&spaceId=<space>",
// and an id could smuggle query parameters into a link other tools resolve.
const (
	objectLinkScheme = "anytype"
	objectLinkHost   = "object"
	objectLinkParam  = "objectId"
)

// objectLinkDest renders an object id as the canonical deep link, percent-
// encoding the id so it cannot introduce a second parameter. Ordinary ids are
// CIDs, so in practice nothing is escaped and the bytes are unchanged.
func objectLinkDest(id string) string {
	return objectLinkScheme + "://" + objectLinkHost + "?" +
		url.Values{objectLinkParam: {id}}.Encode()
}

// parseObjectLink reports the object id a destination names, and whether the
// destination is exactly that canonical link. Anything else — extra query
// parameters, another host, another scheme, a path — is not an object
// reference: it stays a plain Link, preserved verbatim, which is lossless.
// Accepting a superset and dropping the extras would not be (§8.1).
func parseObjectLink(dest string) (string, bool) {
	if !strings.HasPrefix(dest, objectLinkScheme+"://"+objectLinkHost+"?") {
		return "", false
	}
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != objectLinkScheme || u.Host != objectLinkHost ||
		u.Path != "" || u.Fragment != "" || u.User != nil {
		return "", false
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q) != 1 || len(q[objectLinkParam]) != 1 {
		return "", false
	}
	id := q[objectLinkParam][0]
	if id == "" {
		return "", false
	}
	return id, true
}

// isObjectLink reports whether a Link mark's target is the canonical object
// deep link, and so renders as an Object mark (§8.3).
func isObjectLink(dest string) bool {
	_, ok := parseObjectLink(dest)
	return ok
}

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

// renderInline serializes text and its marks into §8 inline Markdown.
func renderInline(txt string, marks []*model.BlockContentTextMark) string {
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
		typ := m.Type
		param := m.Param
		// a Link whose target is an object deep-link renders identically to
		// an Object mark, so it normalizes to one — otherwise the parse-back
		// type flip would break same-type overlap resolution (§8.3)
		if typ == model.BlockContentTextMark_Link {
			if id, ok := parseObjectLink(param); ok {
				typ = model.BlockContentTextMark_Object
				param = id
			}
		}
		if markNeedsParam(typ) {
			if param == "" {
				continue
			}
		} else {
			// a param on a param-less mark type is noise; normalizing it to
			// empty lets equal-range marks merge (§8.3)
			param = ""
		}
		// params beyond the §8 resource bounds are invalid: the parser will
		// not recognize them, so rendering them would not round-trip
		switch typ {
		case model.BlockContentTextMark_Link:
			if text.UTF16RuneCountString(param) > maxLinkDestLen {
				continue
			}
		case model.BlockContentTextMark_Object:
			if text.UTF16RuneCountString(objectLinkDest(param)) > maxLinkDestLen {
				continue
			}
		case model.BlockContentTextMark_Emoji:
			if text.UTF16RuneCountString(param) > maxEmojiParamLen {
				continue
			}
		}
		spans = append(spans, span{typ: typ, param: param, from: from, to: to})
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
	// splice all replacements in one pass; acc is ascending and disjoint
	reps := make([][]uint16, len(acc))
	total := 0
	for i, e := range acc {
		reps[i] = text.StrToUTF16(e.param)
		total += len(reps[i]) - (e.to - e.from)
	}
	nu := make([]uint16, 0, len(u16)+total)
	prev := 0
	for i, e := range acc {
		nu = append(nu, u16[prev:e.from]...)
		nu = append(nu, reps[i]...)
		prev = e.to
	}
	nu = append(nu, u16[prev:]...)
	u16 = nu
	for j := range rest {
		rest[j].from = adjustAcrossSplices(rest[j].from, acc, reps, true)
		rest[j].to = adjustAcrossSplices(rest[j].to, acc, reps, false)
	}
	out := rest[:0]
	for _, s := range rest {
		if s.from < s.to {
			out = append(out, s)
		}
	}
	return u16, out
}

// adjustAcrossSplices maps offset p across the ordered disjoint splices.
// Points strictly inside a replaced range retract to exclude the
// replacement: starts land after it, ends before it.
func adjustAcrossSplices(p int, acc []span, reps [][]uint16, isStart bool) int {
	delta := 0
	for k, e := range acc {
		if p <= e.from {
			break
		}
		if p >= e.to {
			delta += len(reps[k]) - (e.to - e.from)
			continue
		}
		if isStart {
			return e.from + delta + len(reps[k])
		}
		return e.from + delta
	}
	return p + delta
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
// earlier ends. At equal starts the longer range wins (sort order). A merge
// that extends an accepted range can create a fresh overlap with a
// later-accepted range, so each group re-runs until stable — resolution must
// be idempotent for §11 byte-stability.
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
		for {
			sortSpans(group)
			var acc []span
			extended := false
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
							extended = true
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
			group = acc
			// every extending pass consumed at least one span, so this
			// terminates
			if !extended {
				break
			}
		}
		out = append(out, group...)
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
		b.WriteString(`<mention object_id="` + escapeAttr(n.item.param) + `">`)
		renderKids(inLabel)
		b.WriteString(`</mention>`)
	case model.BlockContentTextMark_Object:
		b.WriteByte('[')
		renderKids(true)
		b.WriteString("](" + escapeDest(objectLinkDest(n.item.param)) + ")")
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
			if tagShaped(rs[i:]) {
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

// tagShaped reports whether rs starts a tag-shaped sequence: '<', an optional
// '/', then at least one ASCII letter. This is the whole reserved syntax space
// of the tag namespace, not just the three names version 1 knows (§8.2).
//
// Anchoring the escape on the whitelist instead would leave literal
// `<sub>x</sub>` bytes in canonical output, and the day a version adds `sub`
// those bytes become markup — with nothing in the text string to say which
// version wrote them, and a stricter reading (§8.3 makes a malformed instance
// of a *known* tag an error) turning old valid documents invalid. Escaping the
// shape costs a backslash on text that looks like markup and buys a tag
// namespace a later version can extend without a text-rewriting migration.
//
// It is deliberately the exact complement of the parser's leniency: import
// keeps an unrecognized tag-shaped sequence literal and warns (§10), and
// export escapes exactly what import warns about.
func tagShaped(rs []rune) bool {
	_, ok := tagShapedName(rs)
	return ok
}

// tagShapedName returns the name of the tag-shaped sequence at rs[0] and
// whether rs is tag-shaped at all.
func tagShapedName(rs []rune) (string, bool) {
	j := 1
	if j < len(rs) && rs[j] == '/' {
		j++
	}
	start := j
	if j >= len(rs) || !isASCIILetter(rs[j]) {
		return "", false // a tag name starts with a letter, always
	}
	for j < len(rs) && (isASCIILetter(rs[j]) || rs[j] == '_') {
		j++
	}
	return string(rs[start:j]), true
}

func entityAhead(rs []rune) bool {
	_, _, ok := parseEntity(rs)
	return ok
}

// escapeAttr entity-encodes attribute values. Brackets and backticks are
// encoded too: raw ones would derail the link-label scan when the tag sits
// inside a link label (the scan runs before tag parsing).
func escapeAttr(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;",
		"[", "&#91;", "]", "&#93;", "`", "&#96;",
	)
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
	// brackets and backticks are escaped in both forms: a raw ']' (or a
	// backtick pairing with one in prose) inside a destination would derail
	// the enclosing label scan when the link nests in another label
	if needsAngle {
		b.WriteByte('<')
		for _, r := range dest {
			switch r {
			case '\\', '<', '>', '&', '[', ']', '`':
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
		b.WriteByte('>')
		return b.String()
	}
	// '<' is escaped too: a bare destination starting with '<' would
	// otherwise be misread as the angle-wrapped form
	for _, r := range dest {
		switch r {
		case '\\', '(', ')', '&', '<', '[', ']', '`':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

//
// ---- parsing ----
//

// inlineError is a grammar error in a text string's inline markup (§12).
type inlineError struct {
	Msg     string
	Snippet string
}

func (e *inlineError) Error() string {
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
	return &inlineError{Msg: msg, Snippet: string(rs[start:end])}
}

// inlineNotes collects what a caller may want to report as warnings about a
// text string it parsed successfully (§12). Nothing here makes a document
// invalid, so the parser records instead of failing, and a nil sink is the
// no-op case for callers that do not report.
type inlineNotes struct {
	// unknownTags names the tag-shaped sequences the grammar does not
	// recognize (§10), deduplicated in first-seen order: one occurrence of
	// `<sub>x</sub>` is one fact about the text, not two.
	unknownTags []string
}

func (n *inlineNotes) unknownTag(name string) {
	if n == nil {
		return
	}
	for _, seen := range n.unknownTags {
		if seen == name {
			return
		}
	}
	n.unknownTags = append(n.unknownTags, name)
}

// parseInline parses §8 inline Markdown back into plain text and marks with
// UTF-16 code-unit ranges.
func parseInline(md string) (string, []*model.BlockContentTextMark, error) {
	txt, marks, _, err := parseInlineNotes(md)
	return txt, marks, err
}

// parseInlineNotes is parseInline plus the notes worth surfacing as warnings.
func parseInlineNotes(md string) (string, []*model.BlockContentTextMark, *inlineNotes, error) {
	rs := []rune(md)
	notes := &inlineNotes{}
	toks, err := tokenizeInline(rs, 0, notes)
	if err != nil {
		return "", nil, nil, err
	}
	ib := &inlineBuilder{}
	if err := resolveTokens(toks, ib); err != nil {
		return "", nil, nil, err
	}
	ib.applyInserts()
	marks := canonicalizeMarks(ib.marks)
	return text.UTF16ToStr(ib.out), marks, notes, nil
}

// Resource bounds (deterministic local rules, recorded in SPEC §8): they keep
// parsing linear on the untrusted-document boundary.
const (
	// maxLinkDestLen bounds a link destination; longer candidates are not
	// links (the '[' stays literal). Export drops Link/Object marks whose
	// param exceeds it, keeping the round trip stable.
	maxLinkDestLen = 2048
	// maxLinkDestWS bounds the whitespace tolerated around a destination.
	maxLinkDestWS = 32
	// maxLinkNesting bounds link-label nesting (CommonMark caps labels
	// similarly); deeper '[' stay literal.
	maxLinkNesting = 32
	// maxEmojiParamLen bounds an emoji mark's replacement text; longer
	// params are invalid and dropped (§8.3 step 1).
	maxEmojiParamLen = 64
)

// inlineScanCtx holds per-text precomputed scan tables so link and code-span
// lookups are O(log n) instead of rescanning the tail per candidate.
type inlineScanCtx struct {
	btRuns   map[int][]int // backtick run length -> ascending start positions
	brackets map[int]int   // '[' position -> matching ']' position
}

func newInlineScanCtx(rs []rune) *inlineScanCtx {
	ctx := &inlineScanCtx{btRuns: map[int][]int{}, brackets: map[int]int{}}
	for i := 0; i < len(rs); {
		if rs[i] == '`' {
			n := runLen(rs, i, '`')
			ctx.btRuns[n] = append(ctx.btRuns[n], i)
			i += n
		} else {
			i++
		}
	}
	// bracket pairing with the tokenizer's exact skip rules (escapes, code
	// spans); LIFO pairing equals the per-'[' depth scan it replaces
	var stack []int
	for i := 0; i < len(rs); {
		c := rs[i]
		if c == '\\' && i+1 < len(rs) && isASCIIPunct(rs[i+1]) {
			i += 2
			continue
		}
		if c == '`' {
			n := runLen(rs, i, '`')
			if end, ok := ctx.backtickClose(i+n, n); ok {
				i = end + n
			} else {
				i += n
			}
			continue
		}
		switch c {
		case '[':
			stack = append(stack, i)
		case ']':
			if len(stack) > 0 {
				ctx.brackets[stack[len(stack)-1]] = i
				stack = stack[:len(stack)-1]
			}
		}
		i++
	}
	return ctx
}

// backtickClose finds the next maximal backtick run of exactly n starting at
// or after from.
func (ctx *inlineScanCtx) backtickClose(from, n int) (int, bool) {
	runs := ctx.btRuns[n]
	idx := sort.SearchInts(runs, from)
	if idx < len(runs) {
		return runs[idx], true
	}
	return 0, false
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

func tokenizeInline(rs []rune, depth int, notes *inlineNotes) ([]token, error) {
	ctx := newInlineScanCtx(rs)
	var toks []token
	var pending strings.Builder
	flushText := func() {
		if pending.Len() > 0 {
			toks = append(toks, token{kind: tokText, txt: pending.String()})
			pending.Reset()
		}
	}
	appendText := func(s string) {
		pending.WriteString(s)
	}
	emit := func(t token) {
		flushText()
		toks = append(toks, t)
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
			if end, ok := ctx.backtickClose(i+n, n); ok {
				emit(token{kind: tokCode, content: stripCodePadding(string(rs[i+n : end]))})
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
				// tag-shaped but not a tag this version parses: literal text,
				// never an error (§10) — recorded so a caller can say so,
				// because canonical output would have escaped these bytes
				if name, shaped := tagShapedName(rs[i:]); shaped {
					notes.unknownTag(name)
				}
				appendText("<")
				i++
				break
			}
			if tok != nil {
				emit(*tok)
			}
			i += size
		case '[':
			tok, size, ok, err := parseLink(rs, i, ctx, depth, notes)
			if err != nil {
				return nil, err
			}
			if ok {
				emit(*tok)
				i += size
			} else {
				appendText("[")
				i++
			}
		case '*', '_':
			n := runLen(rs, i, r)
			canOpen, canClose := delimFlanking(rs, i, n, r)
			if canOpen || canClose {
				emit(token{kind: tokDelim, ch: r, n: n, canOpen: canOpen, canClose: canClose})
			} else {
				appendText(strings.Repeat(string(r), n))
			}
			i += n
		case '~':
			n := runLen(rs, i, '~')
			if n == 2 {
				canOpen, canClose := delimFlanking(rs, i, n, '~')
				if canOpen || canClose {
					emit(token{kind: tokDelim, ch: '~', n: n, canOpen: canOpen, canClose: canClose})
				} else {
					appendText("~~")
				}
			} else {
				appendText(strings.Repeat("~", n))
			}
			i += n
		default:
			// batch the plain run up to the next special character
			j := i + 1
			for j < len(rs) && !isInlineSpecial(rs[j]) {
				j++
			}
			appendText(string(rs[i:j]))
			i = j
		}
	}
	flushText()
	return toks, nil
}

func isInlineSpecial(r rune) bool {
	switch r {
	case '\\', '&', '`', '<', '[', '*', '_', '~':
		return true
	}
	return false
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
	if j >= len(rs) || (rs[j] != '>' && rs[j] != '/' && !unicode.IsSpace(rs[j])) {
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
		// attribute names are snake_case like every other identifier the
		// format defines (§8.1), so '_' is part of the name, not a terminator
		for j < len(rs) && (isASCIILetter(rs[j]) || rs[j] == '_') {
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
	if closing && selfClose {
		return nil, 0, false, inlineErr(rs, i, fmt.Sprintf("malformed </%s> tag", name))
	}
	if !closing {
		if err := validateTagAttrs(name, attrs); err != "" {
			return nil, 0, false, inlineErr(rs, i, err)
		}
	}
	if selfClose {
		// zero-length tag: dropped
		return nil, j - i, true, nil
	}
	return &token{kind: tokTag, tagName: name, closing: closing, attrs: attrs}, j - i, true, nil
}

// validateTagAttrs enforces the per-tag attribute rules (§8.1); returns an
// error message or "".
func validateTagAttrs(name string, attrs map[string]string) string {
	switch name {
	case "u":
		if len(attrs) > 0 {
			return "unexpected attribute on <u> tag"
		}
	case "font":
		for k := range attrs {
			if k != "color" && k != "background" {
				return fmt.Sprintf("unknown attribute %q on <font> tag", k)
			}
		}
		if len(attrs) == 0 {
			return "<font> tag needs a color or background attribute"
		}
	case "mention":
		for k := range attrs {
			if k != "object_id" {
				return fmt.Sprintf("unknown attribute %q on <mention> tag", k)
			}
		}
		if _, ok := attrs["object_id"]; !ok {
			return "<mention> tag needs an object_id attribute"
		}
	}
	return ""
}

// scanLink matches the [label](dest) pattern at rs[i] without tokenizing the
// label, using the precomputed bracket table.
func scanLink(rs []rune, i int, ctx *inlineScanCtx) (labelEnd int, dest string, size int, ok bool) {
	labelEnd, ok = ctx.brackets[i]
	if !ok {
		return 0, "", 0, false
	}
	k := labelEnd + 1
	if k >= len(rs) || rs[k] != '(' {
		return 0, "", 0, false
	}
	k++
	ws := 0
	for k < len(rs) && unicode.IsSpace(rs[k]) {
		k++
		ws++
		if ws > maxLinkDestWS {
			return 0, "", 0, false
		}
	}
	if k >= len(rs) {
		return 0, "", 0, false
	}
	destLimit := k + maxLinkDestLen
	var destOk bool
	if rs[k] == '<' {
		dest, k, destOk = scanAngleDest(rs, k+1, destLimit)
	} else {
		dest, k, destOk = scanBareDest(rs, k, destLimit)
	}
	if !destOk {
		return 0, "", 0, false
	}
	ws = 0
	for k < len(rs) && unicode.IsSpace(rs[k]) {
		k++
		ws++
		if ws > maxLinkDestWS {
			return 0, "", 0, false
		}
	}
	if k >= len(rs) || rs[k] != ')' {
		return 0, "", 0, false
	}
	return labelEnd, dest, k + 1 - i, true
}

// scanAngleDest reads an angle-wrapped destination up to the closing '>',
// decoding escapes and entities; limit bounds the scan (§8 resource bounds).
func scanAngleDest(rs []rune, k, limit int) (string, int, bool) {
	var db strings.Builder
	for {
		if k >= len(rs) || k > limit {
			return "", 0, false
		}
		c := rs[k]
		if c == '>' {
			return db.String(), k + 1, true
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

// scanBareDest reads a bare destination up to whitespace or the closing
// unbalanced ')', decoding escapes and entities; limit bounds the scan.
func scanBareDest(rs []rune, k, limit int) (string, int, bool) {
	var db strings.Builder
	parens := 0
	for {
		if k >= len(rs) || k > limit {
			return "", 0, false
		}
		c := rs[k]
		if unicode.IsSpace(c) {
			return db.String(), k, true
		}
		switch c {
		case ')':
			if parens == 0 {
				return db.String(), k, true
			}
			parens--
		case '(':
			parens++
		case '\\':
			if k+1 < len(rs) && isASCIIPunct(rs[k+1]) {
				db.WriteRune(rs[k+1])
				k += 2
				continue
			}
		case '&':
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

func parseLink(rs []rune, i int, ctx *inlineScanCtx, depth int, notes *inlineNotes) (*token, int, bool, error) {
	if depth >= maxLinkNesting {
		return nil, 0, false, nil
	}
	labelEnd, dest, size, ok := scanLink(rs, i, ctx)
	if !ok {
		return nil, 0, false, nil
	}
	labelToks, err := tokenizeInline(rs[i+1:labelEnd], depth+1, notes)
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
	out     []uint16
	marks   []*model.BlockContentTextMark
	inserts []pendingInsert
}

type pendingInsert struct {
	pos int // in pre-insertion coordinates
	seq int
	s   string
}

func (ib *inlineBuilder) appendString(s string) {
	ib.out = append(ib.out, text.StrToUTF16(s)...)
}

// insertLiteral records literal text to splice at pos (an unmatched opening
// delimiter demoted back to text). Splices are deferred and applied in one
// batch by applyInserts — per-demotion slice rebuilds would be quadratic.
// All recorded positions (marks, open entries, inserts) share the
// pre-insertion coordinate space, so deferral is exact.
func (ib *inlineBuilder) insertLiteral(pos int, s string) {
	ib.inserts = append(ib.inserts, pendingInsert{pos: pos, seq: len(ib.inserts), s: s})
}

// applyInserts splices all pending literals in one pass and shifts mark
// offsets. At equal positions, later-recorded inserts land first — a later
// insert at pos pushes earlier-inserted text right, matching the sequential
// semantics the deferral replaces.
func (ib *inlineBuilder) applyInserts() {
	if len(ib.inserts) == 0 {
		return
	}
	ins := ib.inserts
	sort.SliceStable(ins, func(i, j int) bool {
		if ins[i].pos != ins[j].pos {
			return ins[i].pos < ins[j].pos
		}
		return ins[i].seq > ins[j].seq
	})
	encoded := make([][]uint16, len(ins))
	total := 0
	for i := range ins {
		encoded[i] = text.StrToUTF16(ins[i].s)
		total += len(encoded[i])
	}
	out := make([]uint16, 0, len(ib.out)+total)
	prev := 0
	for i := range ins {
		out = append(out, ib.out[prev:ins[i].pos]...)
		out = append(out, encoded[i]...)
		prev = ins[i].pos
	}
	out = append(out, ib.out[prev:]...)
	ib.out = out

	// prefix sums over insert lengths for O(log n) shift lookups
	positions := make([]int, len(ins))
	cum := make([]int, len(ins)+1)
	for i := range ins {
		positions[i] = ins[i].pos
		cum[i+1] = cum[i] + len(encoded[i])
	}
	shift := func(p int, inclusive bool) int32 {
		// sum of insert lengths with pos < p (or <= p when inclusive)
		idx := sort.SearchInts(positions, p)
		if inclusive {
			for idx < len(positions) && positions[idx] == p {
				idx++
			}
		}
		return int32(cum[idx]) //nolint:gosec // UTF-16 offsets are bounded by the text length
	}
	for _, m := range ib.marks {
		from, to := int(m.Range.From), int(m.Range.To)
		m.Range.From += shift(from, true)
		m.Range.To += shift(to, false)
	}
	ib.inserts = nil
}

func (ib *inlineBuilder) addMark(typ model.BlockContentTextMarkType, param string, from, to int) {
	if from >= to {
		return
	}
	if markNeedsParam(typ) && param == "" {
		return
	}
	ib.marks = append(ib.marks, &model.BlockContentTextMark{
		Range: &model.Range{From: int32(from), To: int32(to)}, //nolint:gosec // UTF-16 offsets are bounded by the text length
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
						return &inlineError{Msg: fmt.Sprintf("misnested tags: </%s> closes across <%s>", t.tagName, stack[i].tagName)}
					}
					idx = i
					break
				}
			}
			if idx < 0 {
				return &inlineError{Msg: fmt.Sprintf("unmatched closing </%s> tag", t.tagName)}
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
			if id, ok := parseObjectLink(t.dest); ok {
				ib.addMark(model.BlockContentTextMark_Object, id, start, len(ib.out))
			} else {
				ib.addMark(model.BlockContentTextMark_Link, t.dest, start, len(ib.out))
			}
		}
	}
	for i := len(stack) - 1; i >= 0; i-- {
		e := stack[i]
		if e.kind == entryTag {
			return &inlineError{Msg: fmt.Sprintf("unclosed <%s> tag", e.tagName)}
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
		ib.addMark(model.BlockContentTextMark_Mention, e.attrs["object_id"], e.start16, end)
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
		typ   model.BlockContentTextMarkType //nolint:unused // map-key equality is the use
		param string                         //nolint:unused // map-key equality is the use
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
