package property

import (
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

const (
	TagNameProperty          = "Tag"
	TagNamePropertyToReplace = "Tags"
)

func IsPropertyMatchTagRelation(tags string, hasTag bool) bool {
	return (tags == TagNamePropertyToReplace && !hasTag) || strings.EqualFold(strings.TrimSpace(tags), bundle.RelationKeyTag.String())
}
