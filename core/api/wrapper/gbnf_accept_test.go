package wrapper

// gbnf_accept_test.go — a small backtracking GBNF matcher, test-only. It
// closes the loop §7.4 promised ("assert convertibility by test"): the
// served C12 example must be IN the language of the served grammar, and the
// filter grammar's language must track the pinned parser. Well-formedness
// (checkGBNF) alone cannot catch a grammar whose pinned key order
// contradicts the example shipped beside it.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
)

//
// ---- grammar AST ----
//

type gExpr interface{ gexpr() }

type gSeq struct{ items []gExpr }
type gAlt struct{ alts []gExpr }
type gLit struct{ s string }
type gRef struct{ name string }
type gRep struct {
	e  gExpr
	op byte // '*', '+', '?'
}
type gClass struct {
	neg     bool
	singles []rune
	ranges  [][2]rune
}

func (gSeq) gexpr()   {}
func (gAlt) gexpr()   {}
func (gLit) gexpr()   {}
func (gRef) gexpr()   {}
func (gRep) gexpr()   {}
func (gClass) gexpr() {}

type gGrammar struct {
	rules map[string]gExpr
	steps int
}

// parseGBNF parses a grammar into rule ASTs (same line discipline as
// checkGBNF: `name ::= body` plus `|`-led continuation lines).
func parseGBNF(grammar string) (*gGrammar, error) {
	bodies := map[string]string{}
	var current string
	for _, raw := range strings.Split(grammar, "\n") {
		line := strings.TrimRight(raw, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if name, rest, ok := splitGBNFRule(line); ok {
			bodies[name] = rest
			current = name
			continue
		}
		if current == "" {
			return nil, fmt.Errorf("continuation before any rule")
		}
		bodies[current] += " " + strings.TrimSpace(line)
	}
	g := &gGrammar{rules: map[string]gExpr{}}
	for name, body := range bodies {
		p := &gParser{src: body}
		e, err := p.parseAlt()
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", name, err)
		}
		p.skipSpace()
		if p.pos != len(p.src) {
			return nil, fmt.Errorf("rule %s: trailing input at %d", name, p.pos)
		}
		g.rules[name] = e
	}
	if _, ok := g.rules["root"]; !ok {
		return nil, fmt.Errorf("no root rule")
	}
	return g, nil
}

type gParser struct {
	src string
	pos int
}

func (p *gParser) skipSpace() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
}

func (p *gParser) parseAlt() (gExpr, error) {
	var alts []gExpr
	for {
		seq, err := p.parseSeq()
		if err != nil {
			return nil, err
		}
		alts = append(alts, seq)
		p.skipSpace()
		if p.pos < len(p.src) && p.src[p.pos] == '|' {
			p.pos++
			continue
		}
		break
	}
	if len(alts) == 1 {
		return alts[0], nil
	}
	return gAlt{alts: alts}, nil
}

func (p *gParser) parseSeq() (gExpr, error) {
	var items []gExpr
	for {
		p.skipSpace()
		if p.pos >= len(p.src) || p.src[p.pos] == '|' || p.src[p.pos] == ')' {
			break
		}
		term, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		items = append(items, term)
	}
	if len(items) == 1 {
		return items[0], nil
	}
	return gSeq{items: items}, nil
}

func (p *gParser) parseTerm() (gExpr, error) {
	var e gExpr
	c := p.src[p.pos]
	switch {
	case c == '"':
		lit, err := p.parseLiteral()
		if err != nil {
			return nil, err
		}
		e = lit
	case c == '[':
		class, err := p.parseClass()
		if err != nil {
			return nil, err
		}
		e = class
	case c == '(':
		p.pos++
		inner, err := p.parseAlt()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.pos >= len(p.src) || p.src[p.pos] != ')' {
			return nil, fmt.Errorf("missing ) at %d", p.pos)
		}
		p.pos++
		e = inner
	default:
		start := p.pos
		for p.pos < len(p.src) && isGBNFNameChar(p.src[p.pos]) {
			p.pos++
		}
		if p.pos == start {
			return nil, fmt.Errorf("unexpected %q at %d", c, p.pos)
		}
		e = gRef{name: p.src[start:p.pos]}
	}
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case '*', '+', '?':
			e = gRep{e: e, op: p.src[p.pos]}
			p.pos++
			continue
		}
		break
	}
	return e, nil
}

func (p *gParser) parseLiteral() (gExpr, error) {
	var b strings.Builder
	p.pos++ // opening quote
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == '"' {
			p.pos++
			return gLit{s: b.String()}, nil
		}
		if c == '\\' {
			p.pos++
			if p.pos >= len(p.src) {
				break
			}
			switch p.src[p.pos] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(p.src[p.pos])
			}
			p.pos++
			continue
		}
		b.WriteByte(c)
		p.pos++
	}
	return nil, fmt.Errorf("unterminated literal")
}

func (p *gParser) parseClass() (gExpr, error) {
	class := gClass{}
	p.pos++ // [
	if p.pos < len(p.src) && p.src[p.pos] == '^' {
		class.neg = true
		p.pos++
	}
	var items []rune
	for p.pos < len(p.src) && p.src[p.pos] != ']' {
		c := rune(p.src[p.pos])
		if c == '\\' {
			p.pos++
			if p.pos >= len(p.src) {
				return nil, fmt.Errorf("unterminated class escape")
			}
			switch p.src[p.pos] {
			case 'n':
				c = '\n'
			case 't':
				c = '\t'
			case 'r':
				c = '\r'
			default:
				c = rune(p.src[p.pos])
			}
		}
		items = append(items, c)
		p.pos++
	}
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("unterminated class")
	}
	p.pos++ // ]
	// fold "a-z" triples into ranges
	for i := 0; i < len(items); {
		if i+2 < len(items) && items[i+1] == '-' {
			class.ranges = append(class.ranges, [2]rune{items[i], items[i+2]})
			i += 3
			continue
		}
		class.singles = append(class.singles, items[i])
		i++
	}
	return class, nil
}

//
// ---- matcher ----
//

const gMaxSteps = 4_000_000

// match returns every end position reachable by matching e at pos.
func (g *gGrammar) match(e gExpr, input string, pos int, depth int) map[int]bool {
	g.steps++
	if g.steps > gMaxSteps || depth > 512 {
		panic("gbnf matcher budget exceeded — grammar too complex for the test matcher")
	}
	out := map[int]bool{}
	switch n := e.(type) {
	case gLit:
		if strings.HasPrefix(input[pos:], n.s) {
			out[pos+len(n.s)] = true
		}
	case gClass:
		if pos < len(input) {
			r := rune(input[pos])
			size := 1
			if r >= 0x80 {
				for _, rr := range input[pos:] {
					r = rr
					break
				}
				size = len(string(r))
			}
			if n.contains(r) {
				out[pos+size] = true
			}
		}
	case gRef:
		sub, ok := g.rules[n.name]
		if !ok {
			panic("undefined rule " + n.name)
		}
		return g.match(sub, input, pos, depth+1)
	case gSeq:
		positions := map[int]bool{pos: true}
		for _, item := range n.items {
			next := map[int]bool{}
			for p := range positions {
				for q := range g.match(item, input, p, depth+1) {
					next[q] = true
				}
			}
			positions = next
			if len(positions) == 0 {
				break
			}
		}
		return positions
	case gAlt:
		for _, alt := range n.alts {
			for q := range g.match(alt, input, pos, depth+1) {
				out[q] = true
			}
		}
	case gRep:
		switch n.op {
		case '?':
			out[pos] = true
			for q := range g.match(n.e, input, pos, depth+1) {
				out[q] = true
			}
		case '*', '+':
			frontier := map[int]bool{pos: true}
			if n.op == '*' {
				out[pos] = true
			}
			for len(frontier) > 0 {
				next := map[int]bool{}
				for p := range frontier {
					for q := range g.match(n.e, input, p, depth+1) {
						if !out[q] && q != pos {
							next[q] = true
						}
						out[q] = true
					}
				}
				frontier = next
			}
		}
	}
	return out
}

func (c gClass) contains(r rune) bool {
	found := false
	for _, s := range c.singles {
		if r == s {
			found = true
		}
	}
	for _, rg := range c.ranges {
		if r >= rg[0] && r <= rg[1] {
			found = true
		}
	}
	if c.neg {
		return !found
	}
	return found
}

func (g *gGrammar) accepts(input string) bool {
	g.steps = 0
	ends := g.match(gRef{name: "root"}, input, 0, 0)
	return ends[len(input)]
}

func mustParseGBNF(t *testing.T, grammar string) *gGrammar {
	t.Helper()
	g, err := parseGBNF(grammar)
	require.NoError(t, err)
	return g
}

//
// ---- the assertions ----
//

// TestExamplesAcceptedByOwnGBNF: every served example must be generatable
// under the grammar served beside it. Before the ordered-example fix, 9 of
// 11 examples serialized alphabetically while the grammar pinned
// required-then-optional declared order — a constrained decoder literally
// could not emit the one worked example the tool showed it.
func TestExamplesAcceptedByOwnGBNF(t *testing.T) {
	m, err := BuildManifest()
	require.NoError(t, err)
	for _, tool := range m.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			g := mustParseGBNF(t, tool.GBNF)
			assert.True(t, g.accepts(string(tool.Example)),
				"served example %s is not in the language of the served grammar:\n%s", tool.Example, tool.GBNF)
		})
	}
}

// TestMatcherIsHonest: the matcher itself must be able to reject, or the
// test above proves nothing.
func TestMatcherIsHonest(t *testing.T) {
	g := mustParseGBNF(t, `root ::= "a" [0-9]+`)
	assert.True(t, g.accepts("a17"))
	assert.False(t, g.accepts("a"))
	assert.False(t, g.accepts("b17"))
	assert.False(t, g.accepts("a17x"))

	// a real tool grammar rejects the alphabetical key order the old map
	// serialization produced
	tool, ok := ToolByName("check_item")
	require.True(t, ok)
	tg := mustParseGBNF(t, toolGBNF(tool))
	assert.True(t, tg.accepts(`{"object":"1","block":"ab3f2","checked":true}`))
	assert.False(t, tg.accepts(`{"block":"ab3f2","checked":true,"object":"1"}`),
		"alphabetical key order must be OUT of the grammar's language")
}

// TestFilterGrammarTracksParser: the served filter GBNF must not accept
// strings the pinned parser rejects (the pre-fix `titleEXISTS` class), and
// every served example must be in the grammar AND parse.
func TestFilterGrammarTracksParser(t *testing.T) {
	g := mustParseGBNF(t, filterStringGBNF)

	for _, ex := range filterstring.Examples {
		t.Run("example "+ex, func(t *testing.T) {
			assert.True(t, g.accepts(ex), "served example must be in the served grammar")
			_, err := filterstring.Parse(ex, filterstring.Options{})
			assert.NoError(t, err, "served example must parse")
		})
	}

	// pre-fix false positives: fws between a key and a word-led condition
	// let a constrained decoder emit these; the parser rejects all of them
	for _, bad := range []string{
		`titleEXISTS`,
		`nameCONTAINS "x"`,
		`tagsIN ("a")`,
		`done = false AND titleEXISTS`,
	} {
		t.Run("negative "+bad, func(t *testing.T) {
			assert.False(t, g.accepts(bad), "must be out of the grammar's language")
			_, err := filterstring.Parse(bad, filterstring.Options{})
			assert.Error(t, err, "and the parser agrees")
		})
	}

	// canonical positives stay in
	for _, good := range []string{
		`title EXISTS`,
		`name CONTAINS "x"`,
		`tags IN ("a")`,
		`done = false`,
		`dueDate < currentWeek() AND (status != "Done" OR assignee IS EMPTY)`,
	} {
		t.Run("positive "+good, func(t *testing.T) {
			assert.True(t, g.accepts(good))
			_, err := filterstring.Parse(good, filterstring.Options{})
			assert.NoError(t, err)
		})
	}
}
