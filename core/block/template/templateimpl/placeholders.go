package templateimpl

import (
	"fmt"

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
	placeholderToday       = "_today"
	placeholderCurrentUser = "_current_user"
)

func placeholderTypeToString(t model.PlaceholderType) string {
	switch t {
	case model.Placeholder_PlaceholderToday:
		return placeholderToday
	case model.Placeholder_PlaceholderCurrentUser:
		return placeholderCurrentUser
	default:
		return ""
	}
}

func stringToPlaceholderType(s string) model.PlaceholderType {
	switch s {
	case placeholderToday:
		return model.Placeholder_PlaceholderToday
	case placeholderCurrentUser:
		return model.Placeholder_PlaceholderCurrentUser
	default:
		return model.Placeholder_PlaceholderValue
	}
}

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
		for relKey, val := range mapVal.Iterate() {
			raw := val.String()
			pType := stringToPlaceholderType(raw)
			if pType == model.Placeholder_PlaceholderValue {
				continue
			}
			placeholders = append(placeholders, &model.Placeholder{
				RelationKey: relKey,
				Values: []*model.PlaceholderValue{
					{Type: pType},
				},
			})
		}
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
			if len(p.Values) == 0 || p.Values[0].Type == model.Placeholder_PlaceholderValue {
				delete(existing, p.RelationKey)
			} else {
				existing[p.RelationKey] = domain.String(placeholderTypeToString(p.Values[0].Type))
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
