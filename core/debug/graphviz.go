//go:build (!linux && !darwin) || android || ios || nographviz || (!arm64 && !amd64)
// +build !linux,!darwin android ios nographviz !arm64,!amd64

package debug

func GraphvizSvg(gv, svgFilename string) (err error) {
	return fmt.Errorf("graphviz is not supported on the current platform")
}
