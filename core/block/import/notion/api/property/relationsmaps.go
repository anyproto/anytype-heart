package property

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/import/common"
)

type UniqueKey string

func MakeUniqueKey(name string, format int64) UniqueKey {
	return UniqueKey(fmt.Sprintf("%s_%d", name, format))
}

type PropertiesStore struct {
	PropertyIdsToSnapshots    map[string]*common.StateSnapshot
	RelationsIdsToOptions     map[string][]*common.StateSnapshot
	uniquePropertyToSnapshots map[UniqueKey]*common.StateSnapshot
}

func NewPropertiesStore() *PropertiesStore {
	return &PropertiesStore{
		PropertyIdsToSnapshots:    make(map[string]*common.StateSnapshot, 0),
		RelationsIdsToOptions:     make(map[string][]*common.StateSnapshot, 0),
		uniquePropertyToSnapshots: make(map[UniqueKey]*common.StateSnapshot, 0),
	}
}

func (m *PropertiesStore) GetSnapshotByNameAndFormat(name string, format int64) *common.StateSnapshot {
	uk := MakeUniqueKey(name, format)
	if snapshot, ok := m.uniquePropertyToSnapshots[uk]; ok {
		return snapshot
	}
	return nil
}

func (m *PropertiesStore) AddSnapshotByNameAndFormat(name string, format int64, sn *common.StateSnapshot) {
	uk := MakeUniqueKey(name, format)
	if _, ok := m.uniquePropertyToSnapshots[uk]; !ok {
		m.uniquePropertyToSnapshots[uk] = sn
	}
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
