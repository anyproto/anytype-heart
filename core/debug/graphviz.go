//go:build (!linux && !darwin) || android || ios || nographviz || wwindows
// +build !linux,!darwin android ios nographviz wwindows

package debug

import "fmt"

func GraphvizSvg(gv, svgFilename string) (err error) {
	return fmt.Errorf("graphviz is not supported on the current platform")
}
