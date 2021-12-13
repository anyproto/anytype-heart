// +build !linux,!darwin android ios nographviz
// +build !amd64

package change

import "fmt"

func (t *Tree) Graphviz() (data string, err error) {
	return "", fmt.Errorf("not supported")
}
