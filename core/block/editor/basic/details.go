package basic

import (
	"errors"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const detailSizeLimit = 64 * 1024

var log = logging.Logger("anytype-mw-editor-basic")

type detailUpdate struct {
	key   string
	value *types.Value
}

func (bs *basic) SetDetails(ctx *session.Context, details []*pb.RpcObjectSetDetailsDetail, showEvent bool) (err error) {
	s := bs.NewStateCtx(ctx)

	if err = checkDetailSizes(details); err != nil {
		log.With("objectID", bs.Id()).Errorf(err.Error())
		return err
	}

	// Collect updates handling special cases. These cases could update details themselves, so we
	// have to apply changes later
	updates := bs.collectDetailUpdates(details, s)
	newDetails := applyDetailUpdates(s.CombinedDetails(), updates)
	s.SetDetails(newDetails)

	if err = bs.Apply(s, smartblock.NoRestrictions); err != nil {
		return
	}

	bs.discardOwnSetDetailsEvent(ctx, showEvent)
	return nil
}

func checkDetailSizes(details []*pb.RpcObjectSetDetailsDetail) error {
	for _, d := range details {
		if d.Size() > detailSizeLimit {
			return fmt.Errorf("failed to set detail %s as its size (%d) is above the limit of %d", d.Key, d.Size(), detailSizeLimit)
		}
	}
	return nil
}

func (bs *basic) collectDetailUpdates(details []*pb.RpcObjectSetDetailsDetail, s *state.State) []*detailUpdate {
	updates := make([]*detailUpdate, 0, len(details))
	for _, detail := range details {
		update, err := bs.createDetailUpdate(s, detail)
		if err == nil {
			updates = append(updates, update)
		} else {
			log.Errorf("can't set detail %s: %s", detail.Key, err)
		}
	}
	return updates
}

func applyDetailUpdates(oldDetails *types.Struct, updates []*detailUpdate) *types.Struct {
	newDetails := pbtypes.CopyStruct(oldDetails)
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

func (bs *basic) createDetailUpdate(st *state.State, detail *pb.RpcObjectSetDetailsDetail) (*detailUpdate, error) {
	if detail.Value != nil {
		if err := pbtypes.ValidateValue(detail.Value); err != nil {
			return nil, fmt.Errorf("detail %s validation error: %s", detail.Key, err.Error())
		}
		if err := bs.setDetailSpecialCases(st, detail); err != nil {
			return nil, fmt.Errorf("special case: %w", err)
		}
		if err := bs.addRelationLink(detail.Key, st); err != nil {
			return nil, err
		}
		if err := bs.relationService.ValidateFormat(detail.Key, detail.Value); err != nil {
			return nil, fmt.Errorf("failed to validate relation: %w", err)
		}
	}
	return &detailUpdate{
		key:   detail.Key,
		value: detail.Value,
	}, nil
}

func (bs *basic) setDetailSpecialCases(st *state.State, detail *pb.RpcObjectSetDetailsDetail) error {
	if detail.Key == bundle.RelationKeyType.String() {
		// special case when client sets the type's detail directly instead of using setObjectType command
		return bs.SetObjectTypesInState(st, pbtypes.GetStringListValue(detail.Value))
	}
	if detail.Key == bundle.RelationKeyLayout.String() {
		// special case when client sets the layout detail directly instead of using SetLayoutInState command
		return bs.SetLayoutInState(st, model.ObjectTypeLayout(detail.Value.GetNumberValue()))
	}
	return nil
}

func (bs *basic) addRelationLink(relationKey string, st *state.State) error {
	// TODO: add relation.WithWorkspaceId(workspaceId) filter
	rel, err := bs.relationService.FetchKey(relationKey)
	if err != nil || rel == nil {
		return fmt.Errorf("failed to get relation: %w", err)
	}
	st.AddRelationLinks(&model.RelationLink{
		Format: rel.Format,
		Key:    rel.Key,
	})
	return nil
}

func (bs *basic) discardOwnSetDetailsEvent(ctx *session.Context, showEvent bool) {
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

func (bs *basic) SetLayout(ctx *session.Context, layout model.ObjectTypeLayout) (err error) {
	if err = bs.Restrictions().Object.Check(model.Restrictions_LayoutChange); err != nil {
		return
	}

	s := bs.NewStateCtx(ctx)
	if err = bs.SetLayoutInState(s, layout); err != nil {
		return
	}
	return bs.Apply(s, smartblock.NoRestrictions)
}

func (bs *basic) SetObjectTypes(ctx *session.Context, objectTypes []string) (err error) {
	s := bs.NewStateCtx(ctx)

	var toLayout model.ObjectTypeLayout
	if len(objectTypes) > 0 {
		//nolint:govet
		ot, err := bs.objectStore.GetObjectType(objectTypes[0])
		if err != nil {
			return err
		}

		toLayout = ot.Layout
	}

	if err = bs.SetLayoutInState(s, toLayout); err != nil {
		return fmt.Errorf("convert layout: %w", err)
	}

	if err = bs.SetObjectTypesInState(s, objectTypes); err != nil {
		return
	}

	flags := internalflag.NewFromState(s)
	flags.Remove(model.InternalFlag_editorSelectType)
	flags.AddToState(s)

	// send event here to send updated details to client
	if err = bs.Apply(s, smartblock.NoRestrictions); err != nil {
		return
	}
	return
}

func (bs *basic) SetObjectTypesInState(s *state.State, objectTypes []string) (err error) {
	if len(objectTypes) == 0 {
		return fmt.Errorf("you must provide at least 1 object type")
	}

	if err = bs.Restrictions().Object.Check(model.Restrictions_TypeChange); errors.Is(err, restriction.ErrRestricted) {
		return fmt.Errorf("objectType change is restricted for object '%s': %v", bs.Id(), err)
	}

	otypes, err := bs.objectStore.GetObjectTypes(objectTypes)
	if err != nil {
		return
	}
	if len(otypes) == 0 {
		return fmt.Errorf("object types not found")
	}

	ot := otypes[len(otypes)-1]
	// nolint:errcheck
	prevType, _ := bs.objectStore.GetObjectType(s.ObjectType())

	s.SetObjectTypes(objectTypes)
	if v := pbtypes.Get(s.Details(), bundle.RelationKeyLayout.String()); v == nil || // if layout is not set yet
		prevType == nil || // if we have no type set for some reason or it is missing
		float64(prevType.Layout) == v.GetNumberValue() { // or we have a objecttype recommended layout set for this object
		if err = bs.SetLayoutInState(s, ot.Layout); err != nil {
			return
		}
	}
	return
}

func (bs *basic) SetLayoutInState(s *state.State, toLayout model.ObjectTypeLayout) (err error) {
	if err = bs.Restrictions().Object.Check(model.Restrictions_LayoutChange); errors.Is(err, restriction.ErrRestricted) {
		return fmt.Errorf("layout change is restricted for object '%s': %v", bs.Id(), err)
	}

	fromLayout, _ := s.Layout()

	s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(toLayout)))

	if err = bs.layoutConverter.Convert(s, fromLayout, toLayout); err != nil {
		return fmt.Errorf("convert layout: %w", err)
	}
	return nil
}
