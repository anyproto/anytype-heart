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
		spaceId := sb.SpaceID()

		for _, p := range placeholders {
			if len(p.Values) == 0 {
				continue
			}
			value, err := s.buildStorageValue(p.RelationKey, spaceId, p.Values)
			if err != nil {
				return fmt.Errorf("invalid placeholder value: %w", err)
			}
			existing[p.RelationKey] = value
		}

		if len(existing) != 0 {
			st.SetDetail(bundle.RelationKeyTemplatePlaceholders, domain.NewValueMap(existing))
		}
		return nil
	})
}

func (s *service) DeleteTemplatePlaceholders(ctx session.Context, templateId string, relationKeys []string) error {
	return cache.DoStateCtx(s.picker, ctx, templateId, func(st *state.State, sb smartblock.SmartBlock) error {
		if !lo.Contains(sb.ObjectTypeKeys(), bundle.TypeKeyTemplate) {
			return fmt.Errorf("object is not a template")
		}
		existing := st.Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders).Copy().ToMap()
		if existing == nil {
			return nil
		}

		for _, key := range relationKeys {
			delete(existing, key)
		}

		if len(existing) == 0 {
			st.RemoveDetail(bundle.RelationKeyTemplatePlaceholders)
		} else {
			st.SetDetail(bundle.RelationKeyTemplatePlaceholders, domain.NewValueMap(existing))
		}
		return nil
	})
}

// buildStorageValue converts PlaceholderValue entries to domain.Value and validates each entry.
// Storage format per relation key is a MapList: [{"type": 0, "value": "obj1"}, {"type": 2}]
// Each PlaceholderValue maps 1:1 to a struct with "type" and optional "value" keys.
func (s *service) buildStorageValue(relationKey, spaceId string, values []*model.PlaceholderValue) (domain.Value, error) {
	format, err := s.formatFetcher.GetRelationFormatByKey(spaceId, domain.RelationKey(relationKey))
	if err != nil {
		log.Warnf("failed to get relation format for key %s: %v", relationKey, err)
	}

	if len(values) > 1 && !isObjectFormat(format) {
		return domain.Value{}, fmt.Errorf("relation of format %s cannot handle multiple values", format.String())
	}

	maps := make([]domain.ValueMap, 0, len(values))
	for _, v := range values {
		mapValue, err := placeholderValueToMap(v, format)
		if err != nil {
			return domain.Value{}, err
		}
		maps = append(maps, mapValue.MapValue())
	}
	return domain.MapList(maps), nil
}

func placeholderValueToMap(v *model.PlaceholderValue, format model.RelationFormat) (domain.Value, error) {
	inner := make(map[string]domain.Value, 2)

	if v.Type == model.Placeholder_PlaceholderToday && format != model.RelationFormat_date {
		return domain.Value{}, fmt.Errorf("cannot use 'today' placeholder for detail of %s format: date is expected", format.String())
	}

	if v.Type == model.Placeholder_PlaceholderCurrentUser && format != model.RelationFormat_object {
		return domain.Value{}, fmt.Errorf("cannot use 'current user' placeholder for detail of %s format: object is expected", format.String())
	}

	inner[keyType] = domain.Int64(int64(v.Type))
	if v.Value != nil {
		inner[keyType] = domain.Int64(int64(0))
		value := domain.ValueFromProto(v.Value)
		switch format {
		case model.RelationFormat_object, model.RelationFormat_status, model.RelationFormat_tag,
			model.RelationFormat_file, model.RelationFormat_shorttext, model.RelationFormat_longtext,
			model.RelationFormat_url, model.RelationFormat_phone, model.RelationFormat_email, model.RelationFormat_emoji:
			if !value.IsString() {
				return domain.Value{}, fmt.Errorf("detail of format %s should be string", format.String())
			}
		case model.RelationFormat_number, model.RelationFormat_date:
			if !value.IsFloat64() {
				return domain.Value{}, fmt.Errorf("detail of format %s should be number", format.String())
			}
		case model.RelationFormat_checkbox:
			if !value.IsBool() {
				return domain.Value{}, fmt.Errorf("detail of format %s should be boolean", format.String())
			}
		default:
			return domain.Value{}, fmt.Errorf("relation format %s is not supported for placeholders", format.String())
		}
		inner[keyValue] = value
	}
	return domain.NewValueMap(inner), nil
}

func isObjectFormat(format model.RelationFormat) bool {
	switch format {
	case model.RelationFormat_object, model.RelationFormat_status, model.RelationFormat_tag, model.RelationFormat_file:
		return true
	}
	return false
}

// storageToPlaceholders converts a stored MapList to a list of model.Placeholder
// Each relation key maps to a MapList of entries with "type" and optional "value"
func storageToPlaceholders(mapVal domain.ValueMap) []*model.Placeholder {
	result := make([]*model.Placeholder, 0, mapVal.Len())
	for relKey, val := range mapVal.Iterate() {
		entries := val.MapListValue()
		values := make([]*model.PlaceholderValue, 0, len(entries))
		for _, m := range entries {
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
		Type: model.PlaceholderType(m.GetInt64(keyType)), // nolint:gosec
	}
	if concreteVal, has := m.TryGet(keyValue); has && concreteVal.Ok() {
		pv.Value = concreteVal.ToProto()
	}
	return pv
}

// resolveTemplatePlaceholders reads the templatePlaceholders map from the template state,
// resolves each placeholder to its actual value, and removes the templatePlaceholders detail
func (s *service) resolveTemplatePlaceholders(st *state.State, spaceId string) {
	placeholders, ok := st.Details().TryMapValue(bundle.RelationKeyTemplatePlaceholders)
	if !ok {
		return
	}

	for relKey, val := range placeholders.Iterate() {
		format, err := s.formatFetcher.GetRelationFormatByKey(spaceId, domain.RelationKey(relKey))
		if err != nil {
			log.Warnf("failed to get relation format for key %s: %v", relKey, err)
		}

		var (
			mapList = val.MapListValue()
			strings = make([]string, 0, len(mapList))
			value   = domain.Null() // fallback
			isList  = isObjectFormat(format)
		)

		for _, entry := range val.MapListValue() {
			placeholderType := model.PlaceholderType(entry.GetInt64(keyType)) // nolint:gosec

			switch placeholderType {
			case model.Placeholder_PlaceholderValue:
				if value = entry.Get(keyValue); value.Ok() {
					if isList {
						// placeholders support only string lists
						strings = append(strings, value.String())
					}
				}
			case model.Placeholder_PlaceholderToday:
				ts := s.resolveToday(spaceId, domain.RelationKey(relKey))
				value = domain.Float64(float64(ts))
			case model.Placeholder_PlaceholderCurrentUser:
				if s.accountService != nil {
					participantId := domain.NewParticipantId(spaceId, s.accountService.AccountID())
					strings = append(strings, participantId)
				}
			}
		}

		if len(strings) > 0 {
			value = domain.StringList(strings)
		}
		st.SetDetail(domain.RelationKey(relKey), value)
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
