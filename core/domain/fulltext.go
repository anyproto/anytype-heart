package domain

import "strings"

func ExtractFromFullTextId(fullTextId string) (objectId, blockId, relationKey string) {
	parts := strings.Split(fullTextId, "-")
	objectId = parts[0]
	if len(parts) > 1 {
		if strings.HasPrefix(parts[1], "r_") {
			relationKey = parts[1][2:]
		} else {
			blockId = parts[1]
		}
	}
	return
}
