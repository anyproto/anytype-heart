package property

import (
	"github.com/anyproto/anytype-heart/core/block/import/common"
)

type PropertiesStore struct {
	PropertyIdsToSnapshots map[string]*common.StateSnapshot
	RelationsIdsToOptions  map[string][]*common.StateSnapshot
}

func (m *PropertiesStore) ReadRelationsMap(key string) *common.StateSnapshot {
	if snapshot, ok := m.PropertyIdsToSnapshots[key]; ok {
		return snapshot
	}
	return nil
}

func (m *PropertiesStore) WriteToRelationsMap(key string, relation *common.StateSnapshot) {
	m.PropertyIdsToSnapshots[key] = relation
}

func (m *PropertiesStore) ReadRelationsOptionsMap(key string) []*common.StateSnapshot {
	if snapshot, ok := m.RelationsIdsToOptions[key]; ok {
		return snapshot
	}
	return nil
}

func (m *PropertiesStore) WriteToRelationsOptionsMap(key string, relationOptions []*common.StateSnapshot) {
	m.RelationsIdsToOptions[key] = append(m.RelationsIdsToOptions[key], relationOptions...)
}
