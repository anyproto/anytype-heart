//go:build (!linux && !darwin) || android || ios || windows || nographviz
// +build !linux,!darwin android ios windows nographviz

package debug

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
)

func CreateSvg(block core.SmartBlock, svgFilename string) (err error) {
	return fmt.Errorf("graphviz is not supported on the current platform")
}
