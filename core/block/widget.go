package block

import (
	"context"
	"fmt"
	"slices"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

var skippedTypesForAutoWidget = []domain.TypeKey{
	bundle.TypeKeyTemplate,
	bundle.TypeKeyObjectType,
	bundle.TypeKeyDate,
	bundle.TypeKeyRelation,
	bundle.TypeKeyRelationOption,
	bundle.TypeKeyDashboard,
	bundle.TypeKeyChatDerived,
}

func (s *Service) SetWidgetBlockTargetId(ctx session.Context, req *pb.RpcBlockWidgetSetTargetIdRequest) error {
	return cache.Do(s, req.ContextId, func(b smartblock.SmartBlock) error {
		st := b.NewStateCtx(ctx)
		root := st.Get(req.BlockId)
		if root == nil {
			return fmt.Errorf("failed to find block '%s' in widget object", req.BlockId)
		}
		if len(root.Model().ChildrenIds) == 0 {
			return fmt.Errorf("failed to get child block of widget block '%s' as ChildrenIds is empty", req.BlockId)
		}
		link := st.Get(root.Model().ChildrenIds[0])
		if lc, ok := link.Model().Content.(*model.BlockContentOfLink); ok {
			lc.Link.TargetBlockId = req.TargetId
			return b.Apply(st)
		}
		return fmt.Errorf("failed to update target block of widget block '%s' as its child is not link", req.BlockId)
	})
}

func (s *Service) SetWidgetBlockLayout(ctx session.Context, req *pb.RpcBlockWidgetSetLayoutRequest) error {
	return cache.Do(s, req.ContextId, func(b basic.Updatable) error {
		return b.Update(ctx, func(b simple.Block) error {
			if wc, ok := b.Model().Content.(*model.BlockContentOfWidget); ok {
				wc.Widget.Layout = req.Layout
			}
			return nil
		}, req.BlockId)
	})
}

func (s *Service) SetWidgetBlockLimit(ctx session.Context, req *pb.RpcBlockWidgetSetLimitRequest) error {
	return cache.Do(s, req.ContextId, func(b basic.Updatable) error {
		return b.Update(ctx, func(b simple.Block) error {
			if wc, ok := b.Model().Content.(*model.BlockContentOfWidget); ok {
				wc.Widget.Limit = req.Limit
			}
			return nil
		}, req.BlockId)
	})
}

func (s *Service) SetWidgetBlockViewId(ctx session.Context, req *pb.RpcBlockWidgetSetViewIdRequest) error {
	return cache.Do(s, req.ContextId, func(b basic.Updatable) error {
		return b.Update(ctx, func(b simple.Block) error {
			if wc, ok := b.Model().Content.(*model.BlockContentOfWidget); ok {
				wc.Widget.ViewId = req.ViewId
			}
			return nil
		}, req.BlockId)
	})
}

func (s *Service) CreateTypeWidgetIfMissing(ctx context.Context, spaceId string, key domain.TypeKey) error {
	return s.CreateTypeWidgetsIfMissing(ctx, spaceId, []domain.TypeKey{key}, true)
}

func (s *Service) CreateTypeWidgetsIfMissing(ctx context.Context, spaceId string, keys []domain.TypeKey, checkFirstObject bool) error {
	if len(keys) == 0 {
		return nil
	}

	space, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return err
	}

	widgetObjectId := space.DerivedIDs().Widgets
	spaceIndex := s.objectStore.SpaceIndex(space.Id())
	widgetDetails, err := spaceIndex.GetDetails(widgetObjectId)
	if err == nil && widgetDetails.GetBool(bundle.RelationKeyAutoWidgetDisabled) {
		return nil
	}

	// Get existing widget targets
	var existingTargets []string
	if err == nil {
		existingTargets = widgetDetails.Get(bundle.RelationKeyAutoWidgetTargets).StringList()
	}

	// Filter out skipped types and already existing widgets
	var typesToCreate []struct {
		key    domain.TypeKey
		typeId string
	}

	for _, key := range keys {
		if slices.Contains(skippedTypesForAutoWidget, key) {
			continue
		}

		typeId, err := space.GetTypeIdByKey(ctx, key)
		if err != nil {
			continue
		}

		if slices.Contains(existingTargets, typeId) {
			// widget was created before
			continue
		}

		// Check if this is the first object of this type (if enabled)
		if checkFirstObject {
			records, err := spaceIndex.QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
				database.FilterEq{
					Key:   bundle.RelationKeyType,
					Cond:  model.BlockContentDataviewFilter_Equal,
					Value: domain.String(typeId),
				},
				database.FilterEq{
					Key:   bundle.RelationKeyIsHiddenDiscovery,
					Cond:  model.BlockContentDataviewFilter_NotEqual,
					Value: domain.Bool(true),
				},
				database.FilterEq{
					Key:   bundle.RelationKeyOrigin,
					Cond:  model.BlockContentDataviewFilter_NotEqual,
					Value: domain.Int64(model.ObjectOrigin_usecase),
				},
			}}, 1, 1)
			if err != nil {
				log.Warnf("failed to query records for type '%s' in space '%s': %v", key, spaceId, err)
			}
			if len(records) > 0 {
				// only create widget if this was the first object of this type created
				continue
			}
		}

		typesToCreate = append(typesToCreate, struct {
			key    domain.TypeKey
			typeId string
		}{key: key, typeId: typeId})
	}

	if len(typesToCreate) == 0 {
		return nil
	}

	// Create all widgets in a single transaction
	return cache.DoState(s, widgetObjectId, func(st *state.State, w widget.Widget) error {
		for _, t := range typesToCreate {
			var targetName string
			typeDetails, err := spaceIndex.GetDetails(t.typeId)
			if err == nil {
				targetName = typeDetails.Get(bundle.RelationKeyPluralName).String()
				if targetName == "" {
					targetName = typeDetails.Get(bundle.RelationKeyName).String()
				}
			}

			if err := w.AddAutoWidget(st, t.typeId, t.key.String(), addr.ObjectTypeAllViewId, model.BlockContentWidget_View, targetName); err != nil {
				log.Warnf("failed to add widget for type '%s': %v", t.key, err)
				// Continue with other widgets even if one fails
			}
		}
		return nil
	})
}

// autoInstallSpaceChatWidget automatically installs the chat widget in the space if it is not already installed.
func (s *Service) autoInstallSpaceChatWidget(ctx context.Context, spc clientspace.Space) error {
	widgetObjectId := spc.DerivedIDs().Widgets
	widgetDetails, err := s.objectStore.SpaceIndex(spc.Id()).GetDetails(widgetObjectId)
	if err != nil {
		return err
	}
	keys := widgetDetails.Get(bundle.RelationKeyAutoWidgetTargets).StringList()
	if slices.Contains(keys, widget.DefaultWidgetChat) {
		return nil
	}

	info, err := s.accountService.GetSpaceInfo(ctx, spc.Id())
	if err != nil {
		return fmt.Errorf("get space info: %w", err)
	}

	var createWidget bool
	err = cache.Do(s, info.SpaceViewId, func(sb smartblock.SmartBlock) error {
		uxType := model.SpaceUxType(sb.Details().GetInt64(bundle.RelationKeySpaceUxType))
		if uxType == model.SpaceUxType_Chat {
			createWidget = true
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("get space view: %w", err)
	}
	// Do not create widget in the current release
	// Disable this logic in GO-6089
	if !createWidget {
		return nil
	}

	err = spc.DoCtx(ctx, widgetObjectId, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		if w, ok := sb.(widget.Widget); ok {
			// We rely on AddAutoWidget to check if the widget was already installed/removed before
			err = w.AddAutoWidget(st, widget.DefaultWidgetChat, widget.DefaultWidgetChat, "", model.BlockContentWidget_Link, "")
			if err != nil {
				return err
			}
		}
		return sb.Apply(st)
	})
	if err != nil {
		return err
	}
	return nil
}
