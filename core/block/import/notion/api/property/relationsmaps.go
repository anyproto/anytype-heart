package property

import (
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type UniqueKey string

func MakeUniqueKey(name string, format int64) UniqueKey {
	return UniqueKey(fmt.Sprintf("%s_%d", name, format))
}

type PropertiesStore struct {
	PropertyIdsToSnapshots    map[string]*model.SmartBlockSnapshotBase
	RelationsIdsToOptions     map[string][]*model.SmartBlockSnapshotBase
	uniquePropertyToSnapshots map[UniqueKey]*model.SmartBlockSnapshotBase
}

func NewPropertiesStore() *PropertiesStore {
	return &PropertiesStore{
		PropertyIdsToSnapshots:    make(map[string]*model.SmartBlockSnapshotBase, 0),
		RelationsIdsToOptions:     make(map[string][]*model.SmartBlockSnapshotBase, 0),
		uniquePropertyToSnapshots: make(map[UniqueKey]*model.SmartBlockSnapshotBase, 0),
	}
}

func (m *PropertiesStore) GetSnapshotByNameAndFormat(name string, format int64) *model.SmartBlockSnapshotBase {
	uk := MakeUniqueKey(name, format)
	if snapshot, ok := m.uniquePropertyToSnapshots[uk]; ok {
		return snapshot
	}
	return nil
}

func (m *PropertiesStore) AddSnapshotByNameAndFormat(name string, format int64, sn *model.SmartBlockSnapshotBase) {
	uk := MakeUniqueKey(name, format)
	if _, ok := m.uniquePropertyToSnapshots[uk]; !ok {
		m.uniquePropertyToSnapshots[uk] = sn
	}
}

func (m *PropertiesStore) ReadRelationsMap(key string) *model.SmartBlockSnapshotBase {
	if snapshot, ok := m.PropertyIdsToSnapshots[key]; ok {
		return snapshot
	}
	return nil
}

func (m *PropertiesStore) WriteToRelationsMap(key string, relation *model.SmartBlockSnapshotBase) {
	m.PropertyIdsToSnapshots[key] = relation
}

func (m *PropertiesStore) ReadRelationsOptionsMap(key string) []*model.SmartBlockSnapshotBase {
	if snapshot, ok := m.RelationsIdsToOptions[key]; ok {
		return snapshot
	}
	return nil
}

func (m *PropertiesStore) WriteToRelationsOptionsMap(key string, relationOptions []*model.SmartBlockSnapshotBase) {
	m.RelationsIdsToOptions[key] = append(m.RelationsIdsToOptions[key], relationOptions...)
}
