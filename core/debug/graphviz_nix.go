//go:build (linux || darwin) && !android && !ios && !nographviz && (amd64 || arm64) && cgo
// +build linux darwin
// +build !android
// +build !ios
// +build !nographviz
// +build amd64 arm64
// +build cgo

package debug

import (
	"os"

	"github.com/goccy/go-graphviz"
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
