package hash

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
)

func HeadsHash(heads []string) string {
	if len(heads) == 0 {
		return ""
	}
	slices.Sort(heads)

	sum := sha256.Sum256([]byte(strings.Join(heads, ",")))
	return fmt.Sprintf("%x", sum)
}
