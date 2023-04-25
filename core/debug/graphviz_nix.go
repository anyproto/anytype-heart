//go:build (linux || darwin) && !android && !ios && !nographviz && (amd64 || arm64)
// +build linux darwin
// +build !android
// +build !ios
// +build !nographviz
// +build amd64 arm64

package debug

import (
	"github.com/goccy/go-graphviz"
	"os"
)

func GraphvizSvg(gv, svgFilename string) (err error) {
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
