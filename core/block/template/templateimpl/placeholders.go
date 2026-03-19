package templateimpl

import (
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	keyType  = "type"
	keyValue = "value"
)

func (s *service) GetTemplatePlaceholders(templateId string) ([]*model.Placeholder, error) {
	var placeholders []*model.Placeholder
	err := cache.Do(s.picker, templateId, func(sb smartblock.SmartBlock) error {
		if !lo.Contains(sb.ObjectTypeKeys(), bundle.TypeKeyTemplate) {
			return fmt.Errorf("object is not a template")
		}
		mapVal, ok := sb.Details().TryMapValue(bundle.RelationKeyTemplatePlaceholders)
		if !ok {
			return nil
		}
		placeholders = storageToPlaceholders(mapVal)
		return nil
	})
	return placeholders, err
}

func (s *service) SetTemplatePlaceholders(ctx session.Context, templateId string, placeholders []*model.Placeholder) error {
	return cache.DoStateCtx(s.picker, ctx, templateId, func(st *state.State, sb smartblock.SmartBlock) error {
		if !lo.Contains(sb.ObjectTypeKeys(), bundle.TypeKeyTemplate) {
			return fmt.Errorf("object is not a template")
		}
		existing := st.Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders).Copy().ToMap()
		if existing == nil {
			existing = make(map[string]domain.Value, len(placeholders))
		}

		for _, p := range placeholders {
			if shouldRemovePlaceholder(p.Values) {
				delete(existing, p.RelationKey)
			} else {
				existing[p.RelationKey] = placeholderValuesToStorage(p.Values)
			}
		}

		if len(existing) == 0 {
			st.RemoveDetail(bundle.RelationKeyTemplatePlaceholders)
		} else {
			st.SetDetail(bundle.RelationKeyTemplatePlaceholders, domain.NewValueMap(existing))
		}
		return nil
	})
}

// shouldRemovePlaceholder returns true if the placeholder entry should be removed:
// either no values, or only PlaceholderValue entries with nil concrete values.
func shouldRemovePlaceholder(values []*model.PlaceholderValue) bool {
	if len(values) == 0 {
		return true
	}
	for _, v := range values {
		if v.Type != model.Placeholder_PlaceholderValue || v.Value != nil {
			return false
		}
	}
	return true
}

// placeholderValuesToStorage converts PlaceholderValue entries to domain.Value.
//
// Storage format per relation key:
//   - Single entry:   {"type": <PlaceholderType>}  or  {"type": 0, "value": <val>}
//   - Multiple entries: [{"type": 0, "value": "obj1"}, {"type": 2}]
//
// Each PlaceholderValue maps 1:1 to a struct with "type" and optional "value" keys.
func placeholderValuesToStorage(values []*model.PlaceholderValue) domain.Value {
	if len(values) == 1 {
		return placeholderValueToMap(values[0])
	}
	maps := make([]domain.ValueMap, 0, len(values))
	for _, v := range values {
		maps = append(maps, placeholderValueToMap(v).MapValue())
	}
	return domain.MapList(maps)
}

func placeholderValueToMap(v *model.PlaceholderValue) domain.Value {
	inner := make(map[string]domain.Value, 2)
	inner[keyType] = domain.Float64(float64(v.Type))
	if v.Value != nil {
		inner[keyValue] = domain.ValueFromProto(v.Value)
	}
	return domain.NewValueMap(inner)
}

// storageToPlaceholders converts a stored MapValue to a list of model.Placeholder.
// Each relation key maps to either:
//   - a MapValue (single entry with "type" and optional "value")
//   - a MapList (list of such entries)
func storageToPlaceholders(mapVal domain.ValueMap) []*model.Placeholder {
	var result []*model.Placeholder
	for relKey, val := range mapVal.Iterate() {
		var values []*model.PlaceholderValue

		if maps, ok := val.TryMapList(); ok {
			for _, m := range maps {
				if pv := mapToPlaceholderValue(m); pv != nil {
					values = append(values, pv)
				}
			}
		} else if m, ok := val.TryMapValue(); ok {
			if pv := mapToPlaceholderValue(m); pv != nil {
				values = append(values, pv)
			}
		}

		if len(values) > 0 {
			result = append(result, &model.Placeholder{
				RelationKey: relKey,
				Values:      values,
			})
		}
	}
	return result
}

func mapToPlaceholderValue(m domain.ValueMap) *model.PlaceholderValue {
	pv := &model.PlaceholderValue{
		Type: model.PlaceholderType(m.GetInt64(keyType)),
	}
	if concreteVal, has := m.TryGet(keyValue); has && concreteVal.Ok() {
		pv.Value = concreteVal.ToProto()
	}
	return pv
}

// resolveTemplatePlaceholders reads the templatePlaceholders map from the template state,
// resolves each placeholder to its actual value, and removes the templatePlaceholders detail.
// Storage format per relation: {"type": <PlaceholderType>, "value": <concrete_value>}
func (s *service) resolveTemplatePlaceholders(st *state.State, spaceId string) {
	placeholders, ok := st.Details().TryMapValue(bundle.RelationKeyTemplatePlaceholders)
	if !ok {
		return
	}

	for relKey, val := range placeholders.Iterate() {
		var entries []domain.ValueMap
		if maps, ok := val.TryMapList(); ok {
			entries = maps
		} else if m, ok := val.TryMapValue(); ok {
			entries = []domain.ValueMap{m}
		} else {
			continue
		}

		var strings []string
		var scalar domain.Value

		for _, entry := range entries {
			placeholderType := model.PlaceholderType(entry.GetInt64(keyType))

			switch placeholderType {
			case model.Placeholder_PlaceholderValue:
				if concreteVal, has := entry.TryGet(keyValue); has && concreteVal.Ok() {
					if list := concreteVal.WrapToStringList(); len(list) > 0 {
						strings = append(strings, list...)
					} else {
						scalar = concreteVal
					}
				}
			case model.Placeholder_PlaceholderToday:
				ts := s.resolveToday(spaceId, domain.RelationKey(relKey))
				scalar = domain.Float64(float64(ts))
			case model.Placeholder_PlaceholderCurrentUser:
				if s.accountService != nil {
					participantId := domain.NewParticipantId(spaceId, s.accountService.AccountID())
					strings = append(strings, participantId)
				}
			}
		}

		if len(strings) > 0 {
			st.SetDetail(domain.RelationKey(relKey), domain.StringList(strings))
		} else if scalar.Ok() {
			st.SetDetail(domain.RelationKey(relKey), scalar)
		}
	}

	st.RemoveDetail(bundle.RelationKeyTemplatePlaceholders)
}

func (s *service) resolveToday(spaceId string, relKey domain.RelationKey) int64 {
	now := time.Now()
	includeTime := false
	if spaceId != "" {
		rel, err := s.store.SpaceIndex(spaceId).FetchRelationByKey(relKey.String())
		if err == nil {
			includeTime = rel.GetIncludeTime()
		}
	}
	if !includeTime {
		now = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}
	return now.Unix()
}
