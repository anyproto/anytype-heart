package property

import (
	"strings"
)

const (
	TagNameProperty          = "Tag"
	TagNamePropertyToReplace = "Tags"
)

func IsPropertyMatchTagRelation(tags string, hasTag bool) bool {
	return (tags == TagNamePropertyToReplace && !hasTag) || strings.TrimSpace(tags) == TagNameProperty
}
