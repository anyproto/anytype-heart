package objectstore

import (
	"strings"

	"github.com/ipfs/go-datastore/query"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
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

func (f idsFilter) Filter(e query.Entry) bool {
	_, ok := f[extractIdFromKey(e.Key)]
	return ok
}

func (f idsFilter) Compare(a, b query.Entry) int {
	aIndex := f[extractIdFromKey(a.Key)]
	bIndex := f[extractIdFromKey(b.Key)]
	if aIndex == bIndex {
		return 0
	} else if aIndex < bIndex {
		return -1
	} else {
		return 1
	}
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

func (m *filterSmartblockTypes) Filter(e query.Entry) bool {
	keyParts := strings.Split(e.Key, "/")
	id := keyParts[len(keyParts)-1]

	t, err := m.sbtProvider.Type(id)
	if err != nil {
		log.Errorf("failed to detect smartblock type for %s: %s", id, err.Error())
		return false
	}

	for _, ot := range m.smartBlockTypes {
		if t == ot {
			return !m.not
		}
	}
	return m.not
}

func (m *dsObjectStore) objectTypeFilter(ots ...string) query.Filter {
	var sbTypes []smartblock.SmartBlockType
	for _, otUrl := range ots {
		if ot, err := bundle.GetTypeByUrl(otUrl); err == nil {
			for _, sbt := range ot.Types {
				sbTypes = append(sbTypes, smartblock.SmartBlockType(sbt))
			}
			continue
		}
		if sbt, err := m.sbtProvider.Type(otUrl); err == nil {
			sbTypes = append(sbTypes, sbt)
		}
	}
	return newSmartblockTypesFilter(m.sbtProvider, false, sbTypes)
}
