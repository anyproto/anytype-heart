// +build linux darwin
// +build !android,!ios,!nographviz
// +build amd64 arm64

package debug

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/goccy/go-graphviz"
	"os"
)

// This will create SVG image of the SmartBlock (i.e a DAG)
func CreateSvg(block core.SmartBlock, svgFilename string)(err error){
	t, _, err := change.BuildTree(block)
	if err != nil {
		logger.Fatal("build tree error:", err)
		return err
	}

	gv, err := t.Graphviz()
	if err != nil {
		logger.Fatal("can't make graphviz data:", err)
		return err
	}

	gvo, err := graphviz.ParseBytes([]byte(gv))
	if err != nil {
		logger.Fatal("can't open graphviz data:", err)
		return err
	}

	f, err := os.Create(svgFilename)
	if err != nil {
		logger.Fatal("can't create SVG file:", err)
		return err
	}
	defer f.Close()

	g := graphviz.New()
	return g.Render(gvo, graphviz.SVG, f)
}

