package property

import (
	"strings"
)

const (
	TagNameProperty               = "Tag"
	TagNamePropertyToReplace      = "Tags"
	TagNamePropertyToReplaceLower = "tags"
	UntitledProperty              = "Untitled"
)

func IsPropertyMatchTagRelation(tags string, hasTag bool) bool {
	return ((tags == TagNamePropertyToReplace || strings.TrimSpace(tags) == TagNamePropertyToReplaceLower) && !hasTag) ||
		(strings.TrimSpace(tags) == TagNameProperty)
}
