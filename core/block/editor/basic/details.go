package basic

import (
	"errors"
	"fmt"
	"strings"
	"time"

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
	"github.com/anyproto/anytype-heart/util/uri"
)

var log = logging.Logger("anytype-mw-editor-basic")

func (bs *basic) SetDetails(ctx session.Context, details []domain.Detail, showEvent bool) (err error) {
	_, err = bs.setDetails(ctx, details, showEvent)
	return err
}

func (bs *basic) SetDetailsAndUpdateLastUsed(ctx session.Context, details []domain.Detail, showEvent bool) (err error) {
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

func (bs *basic) setDetails(ctx session.Context, details []domain.Detail, showEvent bool) (updatedKeys []domain.RelationKey, err error) {
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

func (bs *basic) UpdateDetails(update func(current *domain.Details) (*domain.Details, error)) (err error) {
	_, _, err = bs.updateDetails(update)
	return err
}

func (bs *basic) UpdateDetailsAndLastUsed(update func(current *domain.Details) (*domain.Details, error)) error {
	oldDetails, newDetails, err := bs.updateDetails(update)
	if err != nil {
		return err
	}

	diff := domain.StructDiff(oldDetails, newDetails)
	if diff.Len() == 0 {
		return nil
	}
	ts := time.Now().Unix()
	for _, key := range diff.Keys() {
		bs.lastUsedUpdater.UpdateLastUsedDate(bs.SpaceID(), key, ts)
	}
	return nil
}

func (bs *basic) updateDetails(update func(current *domain.Details) (*domain.Details, error)) (oldDetails *domain.Details, newDetails *domain.Details, err error) {
	if update == nil {
		return nil, nil, fmt.Errorf("update function is nil")
	}
	s := bs.NewState()

	oldDetails = s.CombinedDetails()
	oldDetailsCopy := oldDetails.Copy()

	newDetails, err = update(oldDetailsCopy)
	if err != nil {
		return
	}
	s.SetDetails(newDetails)

	if err = bs.addRelationLinks(s, newDetails.Keys()...); err != nil {
		return nil, nil, err
	}

	return oldDetails, newDetails, bs.Apply(s)
}

func (bs *basic) collectDetailUpdates(details []domain.Detail, s *state.State) ([]domain.Detail, []domain.RelationKey) {
	updates := make([]domain.Detail, 0, len(details))
	keys := make([]domain.RelationKey, 0, len(details))
	for _, detail := range details {
		update, err := bs.createDetailUpdate(s, detail)
		if err == nil {
			updates = append(updates, update)
			keys = append(keys, update.Key)
		} else {
			log.Errorf("can't set detail %s: %s", detail.Key, err)
		}
	}
	return updates, keys
}

func applyDetailUpdates(oldDetails *domain.Details, updates []domain.Detail) *domain.Details {
	newDetails := oldDetails.Copy()
	if newDetails == nil {
		newDetails = domain.NewDetails()
	}
	for _, update := range updates {
		if update.Value.IsNull() {
			newDetails.Delete(update.Key)
		} else {
			newDetails.Set(update.Key, update.Value)
		}
	}
	return newDetails
}

// TODO make no sense?
func (bs *basic) createDetailUpdate(st *state.State, detail domain.Detail) (domain.Detail, error) {
	if detail.Value.Ok() {
		if err := bs.setDetailSpecialCases(st, detail); err != nil {
			return domain.Detail{}, fmt.Errorf("special case: %w", err)
		}
		if err := bs.addRelationLink(st, detail.Key); err != nil {
			return domain.Detail{}, err
		}
		if err := bs.validateDetailFormat(bs.SpaceID(), detail.Key, detail.Value); err != nil {
			return domain.Detail{}, fmt.Errorf("failed to validate relation: %w", err)
		}
	}
	return domain.Detail{
		Key:   detail.Key,
		Value: detail.Value,
	}, nil
}

func (bs *basic) validateDetailFormat(spaceID string, key domain.RelationKey, v domain.Value) error {
	if !v.Ok() {
		return fmt.Errorf("invalid value")
	}
	r, err := bs.objectStore.FetchRelationByKey(key.String())
	if err != nil {
		return err
	}
	if v.IsNull() {
		// allow null value for any field
		return nil
	}

	switch r.Format {
	case model.RelationFormat_longtext, model.RelationFormat_shorttext:
		if !v.IsString() {
			return fmt.Errorf("incorrect type: %v instead of string", v)
		}
		return nil
	case model.RelationFormat_number:
		if !v.IsFloat64() {
			return fmt.Errorf("incorrect type: %v instead of number", v)
		}
		return nil
	case model.RelationFormat_status:
		vals, ok := v.TryStringList()
		if !ok {
			return fmt.Errorf("incorrect type: %v instead of string list", v)
		}
		if len(vals) > 1 {
			return fmt.Errorf("status should not contain more than one value")
		}
		return bs.validateOptions(r, vals)

	case model.RelationFormat_tag:
		vals, ok := v.TryStringList()
		if !ok {
			return fmt.Errorf("incorrect type: %v instead of string list", v)
		}
		if r.MaxCount > 0 && len(vals) > int(r.MaxCount) {
			return fmt.Errorf("maxCount exceeded")
		}

		return bs.validateOptions(r, vals)
	case model.RelationFormat_date:
		if !v.IsFloat64() {
			return fmt.Errorf("incorrect type: %v instead of number", v)
		}

		return nil
	case model.RelationFormat_file, model.RelationFormat_object:
		vals, ok := v.TryStringList()
		if !ok {
			return fmt.Errorf("incorrect type: %v instead of string list", v)
		}
		if r.MaxCount > 0 && len(vals) > int(r.MaxCount) {
			return fmt.Errorf("relation %s(%s) has maxCount exceeded", r.Key, r.Format.String())
		}

		for i, lv := range vals {
			if lv == "" {
				return fmt.Errorf("empty option at index %d", i)
			}
		}
		return nil

	case model.RelationFormat_checkbox:
		if !v.IsBool() {
			return fmt.Errorf("incorrect type: %v instead of bool", v)
		}

		return nil
	case model.RelationFormat_url:
		val, ok := v.TryString()
		if !ok {
			return fmt.Errorf("incorrect type: %v instead of string", v)
		}
		s := strings.TrimSpace(val)
		if s != "" {
			err := uri.ValidateURI(s)
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
		_, ok := v.TryString()
		if !ok {
			return fmt.Errorf("incorrect type: %v instead of string", v)
		}
		// todo: revise regexp and reimplement
		/*valid := uri.ValidateEmail(v.GetStringValue())
		if !valid {
			return fmt.Errorf("failed to validate email")
		}*/
		return nil
	case model.RelationFormat_phone:
		_, ok := v.TryString()
		if !ok {
			return fmt.Errorf("incorrect type: %v instead of string", v)
		}

		// todo: revise regexp and reimplement
		/*valid := uri.ValidatePhone(v.GetStringValue())
		if !valid {
			return fmt.Errorf("failed to validate phone")
		}*/
		return nil
	case model.RelationFormat_emoji:
		_, ok := v.TryString()
		if !ok {
			return fmt.Errorf("incorrect type: %v instead of string", v)
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

func (bs *basic) setDetailSpecialCases(st *state.State, detail domain.Detail) error {
	if detail.Key == bundle.RelationKeyType {
		return fmt.Errorf("can't change object type directly: %w", domain.ErrValidationFailed)
	}
	if detail.Key == bundle.RelationKeyLayout {
		// special case when client sets the layout detail directly instead of using SetLayoutInState command
		return bs.SetLayoutInState(st, model.ObjectTypeLayout(detail.Value.Int64()), false)
	}
	return nil
}

func (bs *basic) addRelationLink(st *state.State, relationKey domain.RelationKey) error {
	relLink, err := bs.objectStore.GetRelationLink(relationKey.String())
	if err != nil || relLink == nil {
		return fmt.Errorf("failed to get relation: %w", err)
	}
	st.AddRelationLinks(relLink)
	return nil
}

// addRelationLinks is deprecated and will be removed in release 7
func (bs *basic) addRelationLinks(st *state.State, relationKeys ...domain.RelationKey) error {
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

	if bs.CombinedDetails().GetInt64(bundle.RelationKeyOrigin) == int64(model.ObjectOrigin_none) {
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
	rawLayout := typeDetails.GetInt64(bundle.RelationKeyRecommendedLayout)
	return model.ObjectTypeLayout(rawLayout), nil
}

func (bs *basic) SetLayoutInState(s *state.State, toLayout model.ObjectTypeLayout, ignoreRestriction bool) (err error) {
	if !ignoreRestriction {
		if err = bs.Restrictions().Object.Check(model.Restrictions_LayoutChange); errors.Is(err, restriction.ErrRestricted) {
			return fmt.Errorf("layout change is restricted for object '%s': %w", bs.Id(), err)
		}
	}

	fromLayout, _ := s.Layout()
	s.SetDetail(bundle.RelationKeyLayout, domain.Int64(toLayout))
	if err = bs.layoutConverter.Convert(s, fromLayout, toLayout); err != nil {
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
