package full

import (
	"regexp"
	"sort"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// routeSpeller re-spells the REST routes the document's prose names as the
// tools that serve them. The document is written for HTTP callers, so a
// parameter or body description points at an operation by its route (`Space
// id, from GET /v2/spaces`); a tool caller has no routes, only tools, so the
// same text reaches it as `list_spaces`. The routes come from the document
// itself, so a renamed path or operationId needs no second table.
type routeSpeller struct {
	routes []route
}

// route is one served operation's route template, compiled.
type route struct {
	op     string
	method string
	re     *regexp.Regexp // the path, anchored, one group per placeholder
	params []string
	// literal is the template's length without placeholders: of two
	// templates matching one path, the more literal one names it
	literal int
}

// routeToken is one route-shaped span of prose: a method and a /v<n> path,
// with an optional ?query. A sentence's closing dot, comma or parenthesis is
// not part of it.
var routeToken = regexp.MustCompile(`\b(GET|POST|PATCH|PUT|DELETE) (/v[0-9]+[^\s,;)]*[^\s,;).])`)

// placeholderSegment matches one `{name}` in a route template.
var placeholderSegment = regexp.MustCompile(`\{([a-z_]+)\}`)

func newRouteSpeller() *routeSpeller {
	return &routeSpeller{}
}

// add registers a served tool's operation.
func (s *routeSpeller) add(op, method, path string) {
	var b strings.Builder
	b.WriteString("^")
	var params []string
	last := 0
	literal := 0
	for _, m := range placeholderSegment.FindAllStringSubmatchIndex(path, -1) {
		b.WriteString(regexp.QuoteMeta(path[last:m[0]]))
		literal += m[0] - last
		// a bound value or the placeholder itself, never a path separator
		b.WriteString(`([^/?\s]+)`)
		params = append(params, path[m[2]:m[3]])
		last = m[1]
	}
	b.WriteString(regexp.QuoteMeta(path[last:]))
	literal += len(path) - last
	b.WriteString("$")
	s.routes = append(s.routes, route{op: op, method: strings.ToUpper(method), re: regexp.MustCompile(b.String()), params: params, literal: literal})
}

// respell rewrites every route in s that names a served operation; a route
// naming none is left as written (the derivation test refuses it).
func (s *routeSpeller) respell(text string) string {
	if !strings.Contains(text, " /v") {
		return text
	}
	return routeToken.ReplaceAllStringFunc(text, func(token string) string {
		m := routeToken.FindStringSubmatch(token)
		method, target := m[1], m[2]
		path, rawQuery, _ := strings.Cut(target, "?")
		ref, ok := s.ref(method, path)
		if !ok {
			return token
		}
		if rawQuery != "" {
			ref.Query = map[string]string{}
			for _, pair := range strings.Split(rawQuery, "&") {
				name, value, _ := strings.Cut(pair, "=")
				ref.Query[name] = value
			}
		}
		return Spelling(ref)
	})
}

// refs is every route in text that names a served operation, in order.
func (s *routeSpeller) refs(text string) []v2model.Ref {
	var out []v2model.Ref
	for _, m := range routeToken.FindAllStringSubmatch(text, -1) {
		path, _, _ := strings.Cut(m[2], "?")
		if ref, ok := s.ref(m[1], path); ok {
			out = append(out, ref)
		}
	}
	return out
}

// ref finds the operation a concrete route names: the most literal template
// that matches, its placeholders bound to the values in the path (a
// placeholder written as itself stays unbound).
func (s *routeSpeller) ref(method, path string) (v2model.Ref, bool) {
	best := -1
	var groups []string
	for i, r := range s.routes {
		if r.method != method {
			continue
		}
		g := r.re.FindStringSubmatch(path)
		if g == nil {
			continue
		}
		if best < 0 || r.literal > s.routes[best].literal {
			best, groups = i, g
		}
	}
	if best < 0 {
		return v2model.Ref{}, false
	}
	r := s.routes[best]
	ref := v2model.Ref{Op: r.op}
	for i, name := range r.params {
		if value := groups[i+1]; value != "{"+name+"}" {
			if ref.Params == nil {
				ref.Params = map[string]string{}
			}
			ref.Params[name] = value
		}
	}
	return ref, true
}

// respellSchema rewrites the routes in every description of a JSON Schema
// tree, in place. Values a schema states as data — a default, an example, an
// enum or a const — are content, not prose, and are left alone.
func (s *routeSpeller) respellSchema(node any) {
	switch n := node.(type) {
	case map[string]any:
		keys := make([]string, 0, len(n))
		for k := range n {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch k {
			case "default", "example", "examples", "enum", "const":
				continue
			case "description":
				if text, ok := n[k].(string); ok {
					n[k] = s.respell(text)
					continue
				}
			}
			s.respellSchema(n[k])
		}
	case []any:
		for _, item := range n {
			s.respellSchema(item)
		}
	}
}
