// +build !linux,!darwin android ios
// +build !amd64

package change

import "fmt"

func (tr *Tree) Graphviz() (data string, err error) {
	return "", fmt.Errorf("not supported")
}
