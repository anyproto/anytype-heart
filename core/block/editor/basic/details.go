package basic

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"
	"golang.org/x/exp/maps"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/uri"
)

var log = logging.Logger("anytype-mw-editor-basic")

type detailUpdate struct {
	key   string
	value *types.Value
}

func (bs *basic) SetDetails(ctx session.Context, details []*model.Detail, showEvent bool) (err error) {
	_, err = bs.setDetails(ctx, details, showEvent)
	return err
}

func (bs *basic) SetDetailsAndUpdateLastUsed(ctx session.Context, details []*model.Detail, showEvent bool) (err error) {
	var keys []domain.RelationKey
	keys, err = bs.setDetails(ctx, details, showEvent)
	if err != nil {
		return err
	}
	ts := time.Now().Unix()
	for _, key := range keys {
		bs.lastUsedUpdater.UpdateLastUsedDate(bs.SpaceID(), key, ts)
	}
	return nil
}

func (bs *basic) setDetails(ctx session.Context, details []*model.Detail, showEvent bool) (updatedKeys []domain.RelationKey, err error) {
	s := bs.NewStateCtx(ctx)

	// Collect updates handling special cases. These cases could update details themselves, so we
	// have to apply changes later
	updates, updatedKeys := bs.collectDetailUpdates(details, s)
	newDetails := applyDetailUpdates(s.CombinedDetails(), updates)
	s.SetDetails(newDetails)

	flags := internalflag.NewFromState(s.ParentState())
	flags.Remove(model.InternalFlag_editorDeleteEmpty)
	flags.AddToState(s)

	if err = bs.Apply(s, smartblock.NoRestrictions, smartblock.KeepInternalFlags); err != nil {
		return nil, err
	}

	bs.discardOwnSetDetailsEvent(ctx, showEvent)
	return updatedKeys, nil
}

func (bs *basic) UpdateDetails(update func(current *types.Struct) (*types.Struct, error)) (err error) {
	_, _, err = bs.updateDetails(update)
	return err
}

func (bs *basic) UpdateDetailsAndLastUsed(update func(current *types.Struct) (*types.Struct, error)) (err error) {
	var oldDetails, newDetails *types.Struct
	oldDetails, newDetails, err = bs.updateDetails(update)
	if err != nil {
		return err
	}

	diff := pbtypes.StructDiff(oldDetails, newDetails)
	if diff == nil || diff.Fields == nil {
		return nil
	}
	ts := time.Now().Unix()
	for key := range diff.Fields {
		bs.lastUsedUpdater.UpdateLastUsedDate(bs.SpaceID(), domain.RelationKey(key), ts)
	}
	return nil
}

func (bs *basic) updateDetails(update func(current *types.Struct) (*types.Struct, error)) (oldDetails, newDetails *types.Struct, err error) {
	if update == nil {
		return nil, nil, fmt.Errorf("update function is nil")
	}
	s := bs.NewState()

	oldDetails = s.CombinedDetails()
	oldDetailsCopy := pbtypes.CopyStruct(oldDetails, true)

	newDetails, err = update(oldDetailsCopy)
	if err != nil {
		return
	}
	s.SetDetails(newDetails)

	if err = bs.addRelationLinks(s, maps.Keys(newDetails.Fields)...); err != nil {
		return nil, nil, err
	}

	return oldDetails, newDetails, bs.Apply(s)
}

func (bs *basic) collectDetailUpdates(details []*model.Detail, s *state.State) ([]*detailUpdate, []domain.RelationKey) {
	updates := make([]*detailUpdate, 0, len(details))
	keys := make([]domain.RelationKey, 0, len(details))
	for _, detail := range details {
		update, err := bs.createDetailUpdate(s, detail)
		if err == nil {
			updates = append(updates, update)
			keys = append(keys, domain.RelationKey(update.key))
		} else {
			log.Errorf("can't set detail %s: %s", detail.Key, err)
		}
	}
	return updates, keys
}

func applyDetailUpdates(oldDetails *types.Struct, updates []*detailUpdate) *types.Struct {
	newDetails := pbtypes.CopyStruct(oldDetails, false)
	if newDetails == nil || newDetails.Fields == nil {
		newDetails = &types.Struct{
			Fields: make(map[string]*types.Value),
		}
	}
	for _, update := range updates {
		if update.value == nil {
			delete(newDetails.Fields, update.key)
		} else {
			newDetails.Fields[update.key] = update.value
		}
	}
	return newDetails
}

func (bs *basic) createDetailUpdate(st *state.State, detail *model.Detail) (*detailUpdate, error) {
	if detail.Value != nil {
		if err := pbtypes.ValidateValue(detail.Value); err != nil {
			return nil, fmt.Errorf("detail %s validation error: %w", detail.Key, err)
		}
		if err := bs.setDetailSpecialCases(st, detail); err != nil {
			return nil, fmt.Errorf("special case: %w", err)
		}
		if err := bs.addRelationLink(st, detail.Key); err != nil {
			return nil, err
		}
		if err := bs.validateDetailFormat(bs.SpaceID(), detail.Key, detail.Value); err != nil {
			return nil, fmt.Errorf("failed to validate relation: %w", err)
		}
	}
	return &detailUpdate{
		key:   detail.Key,
		value: detail.Value,
	}, nil
}

func (bs *basic) validateDetailFormat(spaceID string, key string, v *types.Value) error {
	r, err := bs.objectStore.FetchRelationByKey(key)
	if err != nil {
		return err
	}
	if _, isNull := v.Kind.(*types.Value_NullValue); isNull {
		// allow null value for any field
		return nil
	}

	switch r.Format {
	case model.RelationFormat_longtext, model.RelationFormat_shorttext:
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of string", v.Kind)
		}
		return nil
	case model.RelationFormat_number:
		if _, ok := v.Kind.(*types.Value_NumberValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of number", v.Kind)
		}
		return nil
	case model.RelationFormat_status:
		if _, ok := v.Kind.(*types.Value_StringValue); ok {

		} else if _, ok := v.Kind.(*types.Value_ListValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of list", v.Kind)
		}

		vals := pbtypes.GetStringListValue(v)
		if len(vals) > 1 {
			return fmt.Errorf("status should not contain more than one value")
		}
		return bs.validateOptions(r, vals)

	case model.RelationFormat_tag:
		if _, ok := v.Kind.(*types.Value_ListValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of list", v.Kind)
		}

		vals := pbtypes.GetStringListValue(v)
		if r.MaxCount > 0 && len(vals) > int(r.MaxCount) {
			return fmt.Errorf("maxCount exceeded")
		}

		return bs.validateOptions(r, vals)
	case model.RelationFormat_date:
		if _, ok := v.Kind.(*types.Value_NumberValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of number", v.Kind)
		}

		return nil
	case model.RelationFormat_file, model.RelationFormat_object:
		switch s := v.Kind.(type) {
		case *types.Value_StringValue:
			return nil
		case *types.Value_ListValue:
			if r.MaxCount > 0 && len(s.ListValue.Values) > int(r.MaxCount) {
				return fmt.Errorf("relation %s(%s) has maxCount exceeded", r.Key, r.Format.String())
			}

			for i, lv := range s.ListValue.Values {
				if optId, ok := lv.Kind.(*types.Value_StringValue); !ok {
					return fmt.Errorf("incorrect list item value at index %d: %T instead of string", i, lv.Kind)
				} else if optId.StringValue == "" {
					return fmt.Errorf("empty option at index %d", i)
				}
			}
			return nil
		default:
			return fmt.Errorf("incorrect type: %T instead of list/string", v.Kind)
		}
	case model.RelationFormat_checkbox:
		if _, ok := v.Kind.(*types.Value_BoolValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of bool", v.Kind)
		}

		return nil
	case model.RelationFormat_url:
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of string", v.Kind)
		}

		s := strings.TrimSpace(v.GetStringValue())
		if s != "" {
			err := uri.ValidateURI(strings.TrimSpace(v.GetStringValue()))
			if err != nil {
				return fmt.Errorf("failed to parse URL: %w", err)
			}
		}
		// todo: should we allow schemas other than http/https?
		// if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		//	return fmt.Errorf("url scheme %s not supported", u.Scheme)
		// }
		return nil
	case model.RelationFormat_email:
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of string", v.Kind)
		}
		// todo: revise regexp and reimplement
		/*valid := uri.ValidateEmail(v.GetStringValue())
		if !valid {
			return fmt.Errorf("failed to validate email")
		}*/
		return nil
	case model.RelationFormat_phone:
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of string", v.Kind)
		}

		// todo: revise regexp and reimplement
		/*valid := uri.ValidatePhone(v.GetStringValue())
		if !valid {
			return fmt.Errorf("failed to validate phone")
		}*/
		return nil
	case model.RelationFormat_emoji:
		if _, ok := v.Kind.(*types.Value_StringValue); !ok {
			return fmt.Errorf("incorrect type: %T instead of string", v.Kind)
		}

		// check if the symbol is emoji
		return nil
	default:
		return fmt.Errorf("unsupported rel format: %s", r.Format.String())
	}
}

func (bs *basic) validateOptions(rel *relationutils.Relation, v []string) error {
	// TODO:
	return nil
}

func (bs *basic) setDetailSpecialCases(st *state.State, detail *model.Detail) error {
	if detail.Key == bundle.RelationKeyType.String() {
		return fmt.Errorf("can't change object type directly: %w", domain.ErrValidationFailed)
	}
	if detail.Key == bundle.RelationKeyLayout.String() {
		// special case when client sets the layout detail directly instead of using SetLayoutInState command
		return bs.SetLayoutInState(st, model.ObjectTypeLayout(detail.Value.GetNumberValue()), false)
	}
	return nil
}

func (bs *basic) addRelationLink(st *state.State, relationKey string) error {
	relLink, err := bs.objectStore.GetRelationLink(relationKey)
	if err != nil || relLink == nil {
		return fmt.Errorf("failed to get relation: %w", err)
	}
	st.AddRelationLinks(relLink)
	return nil
}

// addRelationLinks is deprecated and will be removed in release 7
func (bs *basic) addRelationLinks(st *state.State, relationKeys ...string) error {
	if len(relationKeys) == 0 {
		return nil
	}
	// this code depends on the objectstore being indexed, but can be run on start with empty account
	// todo: remove this code in release because we will no longer need relationLinks
	relations, err := bs.objectStore.FetchRelationByKeys(relationKeys...)
	if err != nil || relations == nil {
		return fmt.Errorf("failed to get relations: %w", err)
	}
	st.AddRelationLinks(relations.RelationLinks()...)
	return nil
}

func (bs *basic) discardOwnSetDetailsEvent(ctx session.Context, showEvent bool) {
	if !showEvent && ctx != nil {
		var filtered []*pb.EventMessage
		msgs := ctx.GetMessages()
		var isFiltered bool
		for i, msg := range msgs {
			if sd := msg.GetObjectDetailsSet(); sd == nil || sd.Id != bs.Id() {
				filtered = append(filtered, msgs[i])
			} else {
				isFiltered = true
			}
		}
		if isFiltered {
			ctx.SetMessages(bs.Id(), filtered)
		}
	}
}

func (bs *basic) SetLayout(ctx session.Context, layout model.ObjectTypeLayout) (err error) {
	s := bs.NewStateCtx(ctx)
	if err = bs.SetLayoutInState(s, layout, false); err != nil {
		return
	}
	return bs.Apply(s, smartblock.NoRestrictions)
}

func (bs *basic) SetObjectTypes(ctx session.Context, objectTypeKeys []domain.TypeKey, ignoreRestrictions bool) (err error) {
	s := bs.NewStateCtx(ctx)
	if err = bs.SetObjectTypesInState(s, objectTypeKeys, ignoreRestrictions); err != nil {
		return
	}

	// KeepInternalFlags is set because we allow to choose template further
	return bs.Apply(s, smartblock.NoRestrictions, smartblock.KeepInternalFlags)
}

func (bs *basic) SetObjectTypesInState(s *state.State, objectTypeKeys []domain.TypeKey, ignoreRestrictions bool) (err error) {
	if len(objectTypeKeys) == 0 {
		return fmt.Errorf("you must provide at least 1 object type")
	}
	if len(objectTypeKeys) > 1 {
		//nolint:govet
		log.With("objectID", s.RootId()).Warnf("set object types: more than one object type, setting layout to the first one")
	}

	if !ignoreRestrictions {
		if err = bs.Restrictions().Object.Check(model.Restrictions_TypeChange); errors.Is(err, restriction.ErrRestricted) {
			return fmt.Errorf("objectType change is restricted for object '%s': %w", bs.Id(), err)
		}

		if objectTypeKeys[0] == bundle.TypeKeyTemplate {
			return fmt.Errorf("changing object type to template is restricted")
		}
	}

	s.SetObjectTypeKeys(objectTypeKeys)
	removeInternalFlags(s)

	if pbtypes.GetInt64(bs.CombinedDetails(), bundle.RelationKeyOrigin.String()) == int64(model.ObjectOrigin_none) {
		bs.lastUsedUpdater.UpdateLastUsedDate(bs.SpaceID(), objectTypeKeys[0], time.Now().Unix())
	}

	toLayout, err := bs.getLayoutForType(objectTypeKeys[0])
	if err != nil {
		return fmt.Errorf("get layout for type %s: %w", objectTypeKeys[0], err)
	}
	return bs.SetLayoutInState(s, toLayout, ignoreRestrictions)
}

func (bs *basic) getLayoutForType(objectTypeKey domain.TypeKey) (model.ObjectTypeLayout, error) {
	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, objectTypeKey.String())
	if err != nil {
		return 0, fmt.Errorf("create unique key: %w", err)
	}
	typeDetails, err := bs.objectStore.GetObjectByUniqueKey(uk)
	if err != nil {
		return 0, fmt.Errorf("get object by unique key: %w", err)
	}
	rawLayout := pbtypes.GetInt64(typeDetails.GetDetails(), bundle.RelationKeyRecommendedLayout.String())
	return model.ObjectTypeLayout(rawLayout), nil
}

func (bs *basic) SetLayoutInState(s *state.State, toLayout model.ObjectTypeLayout, ignoreRestriction bool) (err error) {
	if !ignoreRestriction {
		if err = bs.Restrictions().Object.Check(model.Restrictions_LayoutChange); errors.Is(err, restriction.ErrRestricted) {
			return fmt.Errorf("layout change is restricted for object '%s': %w", bs.Id(), err)
		}
	}

	fromLayout, _ := s.Layout()
	s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(toLayout)))
	if err = bs.layoutConverter.Convert(bs.Space(), s, fromLayout, toLayout); err != nil {
		return fmt.Errorf("convert layout: %w", err)
	}
	return nil
}

func removeInternalFlags(s *state.State) {
	flags := internalflag.NewFromState(s)
	flags.Remove(model.InternalFlag_editorSelectType)
	flags.Remove(model.InternalFlag_editorDeleteEmpty)
	flags.AddToState(s)
}
