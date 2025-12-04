package database

import (
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type OrderMap struct {
	store          ObjectStore
	collator       *collate.Collator
	collatorBuffer *collate.Buffer
	sortKeys       []domain.RelationKey
	data           map[string]*domain.Details // objectId -> { orderId + name }
	idsBuffer      []string
}

func BuildOrderMap(store ObjectStore, key domain.RelationKey, format model.RelationFormat, collatorBuffer *collate.Buffer) *OrderMap {
	om := &OrderMap{
		store:          store,
		collator:       collate.New(language.Und, collate.IgnoreCase),
		collatorBuffer: collatorBuffer,
		idsBuffer:      make([]string, 0),
	}

	switch format {
	case model.RelationFormat_object, model.RelationFormat_file:
		om.collectObjectNames(key)
		om.sortKeys = []domain.RelationKey{bundle.RelationKeyName}
	case model.RelationFormat_tag, model.RelationFormat_status:
		om.collectOptionOrders(key)
		om.sortKeys = []domain.RelationKey{bundle.RelationKeyOrderId, bundle.RelationKeyName}
	}

	return om
}

func (m *OrderMap) BuildOrder(buf []byte, ids ...string) []byte {
	if m == nil || len(m.data) == 0 {
		return buf[:0]
	}

	m.setOrders(ids...)
	buf = buf[:0]

	for _, key := range m.sortKeys {
		for _, id := range ids {
			if details, ok := m.data[id]; ok {
				buf = append(buf, []byte(details.GetString(key))...)
			}
		}
	}

	return buf
}

// Update updates orders only for objects that exist in OrderMap
func (m *OrderMap) Update(details []*domain.Details) (anyUpdated bool) {
	if m == nil || len(m.data) == 0 {
		return false
	}
	for _, det := range details {
		id := det.GetString(bundle.RelationKeyId)
		updated := false
		existingDetails, found := m.data[id]
		if !found {
			continue
		}

		orderId := det.GetString(bundle.RelationKeyOrderId)
		if existingDetails.GetString(bundle.RelationKeyOrderId) != orderId {
			updated = true
			existingDetails.SetString(bundle.RelationKeyOrderId, orderId)
		}

		name := m.getName(det)
		if existingDetails.GetString(bundle.RelationKeyName) != name {
			updated = true
			existingDetails.SetString(bundle.RelationKeyName, name)
		}

		if updated {
			anyUpdated = true
		}
	}
	return anyUpdated
}

func (m *OrderMap) setOrders(ids ...string) {
	if m.data == nil {
		m.data = make(map[string]*domain.Details, len(ids))
	}

	m.idsBuffer = m.idsBuffer[:0]
	for _, id := range ids {
		if _, found := m.data[id]; !found && id != "" {
			m.idsBuffer = append(m.idsBuffer, id)
		}
	}

	if len(m.idsBuffer) == 0 {
		return
	}

	records, err := m.store.Query(Query{Filters: []FilterRequest{{
		RelationKey: bundle.RelationKeyId,
		Condition:   model.BlockContentDataviewFilter_In,
		Value:       domain.StringList(m.idsBuffer),
	}}})
	if err != nil {
		return
	}

	for _, record := range records {
		m.data[record.Details.GetString(bundle.RelationKeyId)] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String(m.getName(record.Details)),
			bundle.RelationKeyOrderId: record.Details.Get(bundle.RelationKeyOrderId),
		})
	}
}

func (m *OrderMap) collectObjectNames(key domain.RelationKey) {
	targetIdsMap := make(map[string]struct{})

	err := m.store.QueryIterate(Query{Filters: []FilterRequest{{
		RelationKey: key,
		Condition:   model.BlockContentDataviewFilter_NotEmpty,
	}}}, func(details *domain.Details) {
		for _, id := range details.GetStringList(key) {
			targetIdsMap[id] = struct{}{}
		}
	})

	if err != nil {
		log.Warnf("failed to get objects from store: %v", err)
		return
	}

	targetIds := make([]string, 0, len(targetIdsMap))
	for id := range targetIdsMap {
		targetIds = append(targetIds, id)
	}

	if m.data == nil {
		m.data = make(map[string]*domain.Details, len(targetIds))
	}

	err = m.store.QueryIterate(Query{Filters: []FilterRequest{{
		RelationKey: bundle.RelationKeyId,
		Condition:   model.BlockContentDataviewFilter_In,
		Value:       domain.StringList(targetIds),
	}}}, func(details *domain.Details) {
		m.data[details.GetString(bundle.RelationKeyId)] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String(m.getName(details)),
		})
	})

	if err != nil {
		log.Warnf("failed to iterate over objects in store: %v", err)
	}
}

func (m *OrderMap) collectOptionOrders(key domain.RelationKey) {
	if m.data == nil {
		m.data = make(map[string]*domain.Details)
	}
	options, err := m.store.ListRelationOptions(key)
	if err != nil {
		log.Warnf("failed to get relation options from store: %v", err)
		return
	}
	for _, opt := range options {
		m.data[opt.Id] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String(m.collate(opt.Text)),
			bundle.RelationKeyOrderId: domain.String(opt.OrderId),
		})
	}
}

func (m *OrderMap) getName(details *domain.Details) string {
	name := details.GetString(bundle.RelationKeyName)
	// nolint:gosec
	if name == "" && model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout)) == model.ObjectType_note {
		name = details.GetString(bundle.RelationKeySnippet)
	}
	return m.collate(name)
}

func (m *OrderMap) collate(str string) string {
	defer m.collatorBuffer.Reset()
	return string(m.collator.KeyFromString(m.collatorBuffer, str))
}
