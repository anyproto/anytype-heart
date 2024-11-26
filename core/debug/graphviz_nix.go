//go:build (linux || darwin) && !android && !ios && !nographviz && (amd64 || arm64) && cgo
// +build linux darwin
// +build !android
// +build !ios
// +build !nographviz
// +build amd64 arm64
// +build cgo

package debug

import (
	"context"
	"os"

	"github.com/goccy/go-graphviz"
)

func GraphvizSvg(gv, svgFilename string) (err error) {
	gvo, err := graphviz.ParseBytes([]byte(gv))
	if err != nil {
		log.Fatal("can't open graphviz data:", err)
		return err
	}

	f, err := os.Create(svgFilename)
	if err != nil {
		log.Fatal("can't create SVG file:", err)
		return err
	}
	defer f.Close()

	ctx := context.Background()
	g, err := graphviz.New(ctx)
	if err != nil {
		return err
	}
	return g.Render(ctx, gvo, graphviz.SVG, f)
}
