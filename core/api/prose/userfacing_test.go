package prose

// userfacing_test.go guards the strings the v2 API hands a caller: the JSON
// Schemas served from /v2/schemas, the hints attached to a validation issue,
// and the error messages beside them.
//
// core/api/openapiprose_test.go already holds the generated OpenAPI document
// to this rule. It reads the document, so it cannot see the schemas and hints
// that are built in Go and served separately — this reads the source instead,
// and covers them.
//
// Comments are exempt by design: a rule's reasoning is worth keeping next to
// the code. Moving a citation out of a served string and into the comment
// above it is the fix.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var messageRules = []struct {
	name    string
	pattern *regexp.Regexp
	fix     string
}{
	{
		name:    "section mark",
		pattern: regexp.MustCompile(`§`),
		fix:     "state the rule in plain words; the sections are internal documents",
	},
	{
		name:    "spec reference",
		pattern: regexp.MustCompile(`\bSPEC\b`),
		fix:     "state the rule in plain words; a caller cannot open the spec",
	},
	{
		name:    "numbered constraint",
		pattern: regexp.MustCompile(`\b[CD]\d+\b|D′\d`),
		fix:     "state the rule in plain words; a constraint number names nothing a caller can look up",
	},
	{
		name:    "internal filename",
		pattern: regexp.MustCompile(`\b[\w/]+\.md\b`),
		fix:     "a caller cannot open a file in this repository; say what the file says",
	},
	{
		name:    "corpus evidence",
		pattern: regexp.MustCompile(`(?i)\bcorpus\b`),
		fix:     "the evidence behind a rule is not the rule; keep it in a comment",
	},
}

func TestServedStringsCarryNothingOnlyThisRepositoryCanResolve(t *testing.T) {
	root, err := filepath.Abs("..")
	require.NoError(t, err)

	var checked int
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return err
		}
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			checked++
			for _, rule := range messageRules {
				if found := rule.pattern.FindString(lit.Value); found != "" {
					t.Errorf("core/api/%s:%d carries %s %q in a string a caller can read\n  %s\n  %s",
						rel, fset.Position(lit.Pos()).Line, rule.name, found,
						strings.TrimSpace(lit.Value), rule.fix)
				}
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
	require.NotZero(t, checked, "no string literals were read at all")
}
