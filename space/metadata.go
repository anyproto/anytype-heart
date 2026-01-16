package space

import (
	"fmt"
	"strconv"
	"strings"
)

func getRepKey(spaceId string) (uint64, error) {
	sepIdx := strings.Index(spaceId, ".")
	if sepIdx == -1 {
		return 0, fmt.Errorf("space id is incorrect")
	}
	return strconv.ParseUint(spaceId[sepIdx+1:], 36, 64)
}
