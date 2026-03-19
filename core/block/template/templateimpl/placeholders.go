package templateimpl

import (
	"fmt"
	"strconv"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
		existing := st.Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders).ToMap()
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

// placeholderValuesToStorage converts PlaceholderValue entries to a domain.Value (MapValue).
// Keys are stringified PlaceholderType numbers, values are:
//   - For PlaceholderValue (type 0): the concrete value converted via domain.ValueFromProto
//   - For shortcuts (Today=1, CurrentUser=2): domain.Bool(true) as a marker
func placeholderValuesToStorage(values []*model.PlaceholderValue) domain.Value {
	inner := make(map[string]domain.Value, len(values))
	for _, v := range values {
		key := strconv.Itoa(int(v.Type))
		if v.Type == model.Placeholder_PlaceholderValue && v.Value != nil {
			inner[key] = domain.ValueFromProto(v.Value)
		} else if v.Type != model.Placeholder_PlaceholderValue {
			inner[key] = domain.Bool(true)
		}
	}
	return domain.NewValueMap(inner)
}

// storageToPlaceholders converts a stored MapValue to a list of model.Placeholder.
// The outer map keys are relation keys, inner maps have stringified PlaceholderType as keys.
func storageToPlaceholders(mapVal domain.ValueMap) []*model.Placeholder {
	var result []*model.Placeholder
	for relKey, val := range mapVal.Iterate() {
		innerMap, ok := val.TryMapValue()
		if !ok {
			continue
		}
		var values []*model.PlaceholderValue
		for typeStr, v := range innerMap.Iterate() {
			typeNum, err := strconv.Atoi(typeStr)
			if err != nil {
				continue
			}
			pv := &model.PlaceholderValue{
				Type: model.PlaceholderType(typeNum),
			}
			if pv.Type == model.Placeholder_PlaceholderValue {
				pv.Value = v.ToProto()
			}
			values = append(values, pv)
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
