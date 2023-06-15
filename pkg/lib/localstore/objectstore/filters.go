package objectstore

import (
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
	"github.com/anyproto/anytype-heart/space/typeprovider"
)

func newIdsFilter(ids []string) idsFilter {
	f := make(idsFilter)
	for i, id := range ids {
		f[id] = i
	}
	return f
}

type idsFilter map[string]int

func (f idsFilter) FilterObject(getter filter.Getter) bool {
	id := getter.Get(bundle.RelationKeyId.String()).GetStringValue()
	_, ok := f[id]
	return ok
}

func (f idsFilter) Compare(a, b filter.Getter) int {
	idA := a.Get(bundle.RelationKeyId.String()).GetStringValue()
	idB := b.Get(bundle.RelationKeyId.String()).GetStringValue()
	aIndex := f[idA]
	bIndex := f[idB]
	if aIndex == bIndex {
		return 0
	} else if aIndex < bIndex {
		return -1
	} else {
		return 1
	}
}

func (f idsFilter) String() string {
	return fmt.Sprintf("idsFilter")
}

type filterSmartblockTypes struct {
	smartBlockTypes []smartblock.SmartBlockType
	not             bool
	sbtProvider     typeprovider.SmartBlockTypeProvider
}

func newSmartblockTypesFilter(sbtProvider typeprovider.SmartBlockTypeProvider, not bool, smartBlockTypes []smartblock.SmartBlockType) *filterSmartblockTypes {
	return &filterSmartblockTypes{
		smartBlockTypes: smartBlockTypes,
		not:             not,
		sbtProvider:     sbtProvider,
	}
}

func (m *filterSmartblockTypes) FilterObject(getter filter.Getter) bool {
	id := getter.Get(bundle.RelationKeyId.String()).GetStringValue()
	t, err := m.sbtProvider.Type(id)
	if err != nil {
		log.Debugf("failed to detect smartblock type for %s: %s", id, err.Error())
		return false
	}
	for _, ot := range m.smartBlockTypes {
		if t == ot {
			return !m.not
		}
	}
	return m.not
}

func (m *filterSmartblockTypes) String() string {
	return fmt.Sprintf("filterSmartblockTypes %v", m.smartBlockTypes)
}
