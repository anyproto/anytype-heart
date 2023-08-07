package hash

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"golang.org/x/exp/slices"
)

func HeadsHash(heads []string) string {
	if len(heads) == 0 {
		return ""
	}
	slices.Sort(heads)

	sum := sha256.Sum256([]byte(strings.Join(heads, ",")))
	return fmt.Sprintf("%x", sum)
}
