package property

import "github.com/anyproto/anytype-heart/pkg/lib/pb/model"

type PropertiesStore struct {
	PropertyIdsToSnapshots map[string]*model.SmartBlockSnapshotBase
	RelationsIdsToOptions  map[string][]*model.SmartBlockSnapshotBase
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
