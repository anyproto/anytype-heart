package apiv2

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

// TestServedRoutesComeFromTheOperationTable guards the typed-hint contract
// (model/ref.go): a repair that names one of this API's operations, or a
// query parameter to resend with, spells it through a Ref, so the reference
// is on the wire as data (see_also) and a caller without routes can re-spell
// it. A route written straight into a served string is exactly the
// affordance the MCP benchmark showed callers cannot use, and it is
// invisible to every wrapper's lookup.
//
// The schema documents are the exception: their `endpoint` lines and field
// descriptions document the REST surface for a reader who asked for it, and
// they are not repairs.
func TestServedRoutesComeFromTheOperationTable(t *testing.T) {
	// a method before a route or an ellipsis, a versioned path, or a bare
	// `?name=` query-parameter mention — every spelling a repair has used
	routeShaped := regexp.MustCompile(`(?:GET|POST|PATCH|PUT|DELETE|HEAD) (?:/v[0-9]|…|\.\.\./)|/v2/(?:spaces|schemas|search|auth|validate)\b|\?[a-z_]+=`)
	documentation := map[string]bool{
		"service/schemas.go":     true,
		"service/schemas_ops.go": true,
		"service/apiv2schema.go": true,
		"authz.go":               true, // the route table itself
	}

	var checked int
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || documentation[filepath.ToSlash(path)] ||
			strings.HasPrefix(path, "model") {
			return err
		}
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			checked++
			if found := routeShaped.FindString(lit.Value); found != "" {
				t.Errorf("%s:%d spells a route in a served string (%q) — name the operation with a v2model.Ref so see_also carries it\n  %s",
					path, fset.Position(lit.Pos()).Line, found, strings.TrimSpace(lit.Value))
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
	require.NotZero(t, checked)
}
