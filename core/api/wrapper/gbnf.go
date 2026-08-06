package wrapper

// gbnf.go renders each tool's GBNF grammar (llama.cpp / Ollama constrained
// decoding) from the same Arg table the schema comes from — §7.3 item 2:
// on-device function-calling needs the TOOL schemas grammar-emittable, and
// C13 applies to these small flat argument objects instead of the recursive
// block tree. The generated grammar constrains the arguments JSON object:
// required args first in declared order, then each optional arg as an
// independently omittable ("," pair)? group — a deterministic key order the
// docs state, which is exactly what constrained decoding wants.
//
// The filter-string grammar is served as a SEPARATE artifact
// (Manifest.FilterGrammar) rather than composed into find's `filter` value:
// composing a DSL into a JSON string production would need every DSL quote
// re-escaped through the JSON string encoding — a transformation neither
// GBNF nor the EBNF source expresses. Runners that support grammar
// switching apply it to the unescaped value; everyone else keeps `filter`
// as a plain JSON string (the schema bound still applies).

import (
	"fmt"
	"regexp"
	"strings"
)

// gbnfCommon holds the shared productions every tool grammar appends:
// llama.cpp-style JSON string/number/boolean plus the bounded flat object
// used by ArgObject args (scalar/array values only — no nested objects).
const gbnfCommon = `ws ::= [ \t\n]*
string ::= "\"" schar* "\""
schar ::= [^"\\] | "\\" (["\\/bfnrt] | "u" hex hex hex hex)
hex ::= [0-9a-fA-F]
number ::= "-"? [0-9]+ ("." [0-9]+)?
boolean ::= "true" | "false"
scalar ::= string | number | boolean | "null"
scalarlist ::= "[" ws (scalar (ws "," ws scalar)*)? ws "]"
flatvalue ::= scalar | scalarlist
flatobject ::= "{" ws (string ws ":" ws flatvalue (ws "," ws string ws ":" ws flatvalue)*)? ws "}"`

// toolGBNF renders one tool's grammar over its argument object.
func toolGBNF(t Tool) string {
	var required, optional []Arg
	for _, a := range t.Args {
		if a.Required {
			required = append(required, a)
		} else {
			optional = append(optional, a)
		}
	}
	var b strings.Builder
	b.WriteString("root ::= \"{\" ws ")
	for i, a := range required {
		if i > 0 {
			b.WriteString(" \",\" ws ")
		}
		b.WriteString(pairRule(a.Name))
	}
	for _, a := range optional {
		fmt.Fprintf(&b, " (\",\" ws %s)?", pairRule(a.Name))
	}
	b.WriteString(" ws \"}\"\n")
	for _, a := range t.Args {
		fmt.Fprintf(&b, "%s ::= \"\\\"%s\\\"\" ws \":\" ws %s ws\n", pairRule(a.Name), a.Name, valueRule(a))
	}
	for _, a := range t.Args {
		if len(a.Enum) > 0 {
			alts := make([]string, len(a.Enum))
			for i, v := range a.Enum {
				alts[i] = fmt.Sprintf("\"\\\"%s\\\"\"", v)
			}
			fmt.Fprintf(&b, "%s ::= %s\n", enumRule(a.Name), strings.Join(alts, " | "))
		}
	}
	b.WriteString(gbnfCommon)
	return b.String()
}

func pairRule(name string) string {
	return "p-" + strings.ReplaceAll(name, "_", "-")
}

func enumRule(name string) string {
	return "e-" + strings.ReplaceAll(name, "_", "-")
}

func valueRule(a Arg) string {
	switch a.Type {
	case ArgString:
		if len(a.Enum) > 0 {
			return enumRule(a.Name)
		}
		return "string"
	case ArgInteger:
		return "number"
	case ArgBoolean:
		return "boolean"
	case ArgObject:
		return "flatobject"
	default:
		return "string"
	}
}

// filterStringGBNF is the GBNF transcription of the pinned filter-string
// EBNF (filterstring.EBNF, SPEC §6.2.1). It constrains to the canonical
// surface — uppercase keywords, camelCase preset names, ASCII identifiers —
// which is what constrained decoding should produce; the parser itself is
// case-insensitive and Unicode-lenient.
const filterStringGBNF = `root ::= fws orexpr fws
orexpr ::= andexpr (rws "OR" rws andexpr)*
andexpr ::= primary (rws "AND" rws primary)*
primary ::= "(" fws orexpr fws ")" | leaf
leaf ::= key fws cond
cond ::= compare fws value
       | eqop fws valuelist
       | ("NOT" rws)? "CONTAINS" rws value
       | ("NOT" rws)? "IN" fws valuelist
       | ("NOT" rws)? "HAS" rws "ALL" fws valuelist
       | "IS" rws ("NOT" rws)? "EMPTY"
       | "EXISTS"
compare ::= ">=" | "<=" | "!=" | "=" | ">" | "<"
eqop ::= "=" | "!="
valuelist ::= "(" fws fvalue (fws "," fws fvalue)* fws ")"
value ::= fvalue
fvalue ::= fstring | fnumber | "true" | "false" | preset
preset ::= presetname fws "(" fws ")" | countname fws "(" fws fnumber fws ")"
presetname ::= "yesterday" | "today" | "tomorrow" | "lastWeek" | "currentWeek" | "nextWeek" | "lastMonth" | "currentMonth" | "nextMonth" | "lastYear" | "currentYear" | "nextYear"
countname ::= "daysAgo" | "daysFromNow"
key ::= [a-zA-Z_] [a-zA-Z0-9_]*
fstring ::= "\"" fchar* "\""
fchar ::= [^"\\] | "\\" ["\\nt]
fnumber ::= "-"? [0-9]+ ("." [0-9]*)?
fws ::= [ \t]*
rws ::= [ \t]+`

//
// ---- well-formedness checking (the §7.4 convertibility assertion) ----
//

var gbnfRuleNameRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]*$`)

// checkGBNF verifies a grammar is well-formed GBNF: every line is
// `name ::= body` (or a continuation line), literals and character classes
// terminate, parentheses balance, and every referenced rule is defined.
// It is the test hook that keeps the served artifacts honest (§7.4).
func checkGBNF(grammar string) error {
	defined := map[string]bool{}
	referenced := map[string]bool{}
	var current string
	for lineNo, raw := range strings.Split(grammar, "\n") {
		line := strings.TrimRight(raw, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		body := line
		if name, rest, ok := splitGBNFRule(line); ok {
			if !gbnfRuleNameRe.MatchString(name) {
				return fmt.Errorf("line %d: invalid rule name %q", lineNo+1, name)
			}
			if defined[name] {
				return fmt.Errorf("line %d: rule %q defined twice", lineNo+1, name)
			}
			defined[name] = true
			current = name
			body = rest
		} else {
			if current == "" {
				return fmt.Errorf("line %d: continuation before any rule", lineNo+1)
			}
			body = strings.TrimSpace(body)
			if !strings.HasPrefix(body, "|") {
				return fmt.Errorf("line %d: continuation lines must start with |", lineNo+1)
			}
		}
		if err := scanGBNFBody(body, referenced); err != nil {
			return fmt.Errorf("line %d: %w", lineNo+1, err)
		}
	}
	if !defined["root"] {
		return fmt.Errorf("no root rule defined")
	}
	for name := range referenced {
		if !defined[name] {
			return fmt.Errorf("rule %q referenced but not defined", name)
		}
	}
	return nil
}

func splitGBNFRule(line string) (name, body string, ok bool) {
	idx := strings.Index(line, "::=")
	if idx < 0 {
		return "", "", false
	}
	name = strings.TrimSpace(line[:idx])
	if name == "" || strings.ContainsAny(name, "\"[|()") {
		return "", "", false
	}
	return name, line[idx+3:], true
}

// scanGBNFBody tokenizes one rule body: quoted literals, char classes,
// rule references, and the ( ) | * + ? operators.
func scanGBNFBody(body string, referenced map[string]bool) error {
	depth := 0
	i := 0
	for i < len(body) {
		c := body[i]
		switch {
		case c == '"':
			j := i + 1
			for j < len(body) {
				if body[j] == '\\' {
					j += 2
					continue
				}
				if body[j] == '"' {
					break
				}
				j++
			}
			if j >= len(body) {
				return fmt.Errorf("unterminated literal")
			}
			i = j + 1
		case c == '[':
			j := i + 1
			for j < len(body) {
				if body[j] == '\\' {
					j += 2
					continue
				}
				if body[j] == ']' {
					break
				}
				j++
			}
			if j >= len(body) {
				return fmt.Errorf("unterminated character class")
			}
			i = j + 1
		case c == '(':
			depth++
			i++
		case c == ')':
			depth--
			if depth < 0 {
				return fmt.Errorf("unbalanced parentheses")
			}
			i++
		case c == '|' || c == '*' || c == '+' || c == '?' || c == ' ' || c == '\t':
			i++
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			j := i
			for j < len(body) && (isGBNFNameChar(body[j])) {
				j++
			}
			referenced[body[i:j]] = true
			i = j
		default:
			return fmt.Errorf("unexpected character %q", c)
		}
	}
	if depth != 0 {
		return fmt.Errorf("unbalanced parentheses")
	}
	return nil
}

func isGBNFNameChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-'
}
