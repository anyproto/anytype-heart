// +build !linux,!darwin android ios nographviz
// +build !amd64

package debug

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"fmt"
)

func CreateSvg(block core.SmartBlock, svgFilename string)(err error){
	return fmt.Errorf("graphviz is not supported on the current platform")
}
