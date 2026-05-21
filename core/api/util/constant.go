package util

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var ObjectLayouts = []model.ObjectTypeLayout{
	model.ObjectType_basic,
	model.ObjectType_profile,
	model.ObjectType_todo,
	model.ObjectType_note,
	model.ObjectType_bookmark,
	model.ObjectType_set,
	model.ObjectType_collection,
}

var ObjectAndFileLayouts = append(append([]model.ObjectTypeLayout{}, ObjectLayouts...), domain.FileLayouts...)

var fileTypeUniqueKeySet = map[string]struct{}{
	bundle.TypeKeyFile.URL():  {},
	bundle.TypeKeyImage.URL(): {},
	bundle.TypeKeyVideo.URL(): {},
	bundle.TypeKeyAudio.URL(): {},
}

// IsFileTypeUniqueKey reports whether the given type unique key (e.g. "ot-image") belongs to a file-layout type.
func IsFileTypeUniqueKey(uniqueKey string) bool {
	_, ok := fileTypeUniqueKeySet[uniqueKey]
	return ok
}

var MemberLayouts = []model.ObjectTypeLayout{
	model.ObjectType_participant,
}

var TagLayouts = []model.ObjectTypeLayout{
	model.ObjectType_relationOption,
	model.ObjectType_tag,
}

var objectLayoutSet = func() map[model.ObjectTypeLayout]struct{} {
	m := make(map[model.ObjectTypeLayout]struct{}, len(ObjectLayouts))
	for _, l := range ObjectLayouts {
		m[l] = struct{}{}
	}
	return m
}()

var memberLayoutSet = func() map[model.ObjectTypeLayout]struct{} {
	m := make(map[model.ObjectTypeLayout]struct{}, len(MemberLayouts))
	for _, l := range MemberLayouts {
		m[l] = struct{}{}
	}
	return m
}()

var fileLayoutSet = func() map[model.ObjectTypeLayout]struct{} {
	m := make(map[model.ObjectTypeLayout]struct{}, len(domain.FileLayouts))
	for _, l := range domain.FileLayouts {
		m[l] = struct{}{}
	}
	return m
}()

var tagLayoutSet = func() map[model.ObjectTypeLayout]struct{} {
	m := make(map[model.ObjectTypeLayout]struct{}, len(TagLayouts))
	for _, l := range TagLayouts {
		m[l] = struct{}{}
	}
	return m
}()

func IsObjectLayout(layout model.ObjectTypeLayout) bool {
	_, ok := objectLayoutSet[layout]
	return ok
}

func IsMemberLayout(layout model.ObjectTypeLayout) bool {
	_, ok := memberLayoutSet[layout]
	return ok
}

func IsObjectOrMemberLayout(layout model.ObjectTypeLayout) bool {
	return IsObjectLayout(layout) || IsMemberLayout(layout)
}

func IsFileLayout(layout model.ObjectTypeLayout) bool {
	_, ok := fileLayoutSet[layout]
	return ok
}

func IsTagLayout(layout model.ObjectTypeLayout) bool {
	_, ok := tagLayoutSet[layout]
	return ok
}

func LayoutsToIntArgs(layouts []model.ObjectTypeLayout) []int {
	ints := make([]int, len(layouts))
	for i, l := range layouts {
		ints[i] = int(l)
	}
	return ints
}
