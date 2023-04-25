package basic

import (
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/internalflag"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

var log = logging.Logger("anytype-mw-editor-basic")

func (bs *basic) SetDetails(ctx *session.Context, details []*pb.RpcObjectSetDetailsDetail, showEvent bool) (err error) {
	s := bs.NewStateCtx(ctx)
	detCopy := pbtypes.CopyStruct(s.CombinedDetails())
	if detCopy == nil || detCopy.Fields == nil {
		detCopy = &types.Struct{
			Fields: make(map[string]*types.Value),
		}
	}

	for _, detail := range details {
		if detail.Value != nil {
			if err := pbtypes.ValidateValue(detail.Value); err != nil {
				return fmt.Errorf("detail %s validation error: %s", detail.Key, err.Error())
			}
			if detail.Key == bundle.RelationKeyType.String() {
				// special case when client sets the type's detail directly instead of using setObjectType command
				err = bs.SetObjectTypes(ctx, pbtypes.GetStringListValue(detail.Value))
				if err != nil {
					log.Errorf("failed to set object's type via detail: %s", err.Error())
				} else {
					continue
				}
			}
			if detail.Key == bundle.RelationKeyLayout.String() {
				// special case when client sets the layout detail directly instead of using SetLayoutInState command
				err = bs.SetLayout(ctx, model.ObjectTypeLayout(detail.Value.GetNumberValue()))
				if err != nil {
					log.Errorf("failed to set object's layout via detail: %s", err.Error())
				}
				continue
			}

			// TODO: add relation2.WithWorkspaceId(workspaceId) filter
			rel, err := bs.relationService.FetchKey(detail.Key)
			if err != nil || rel == nil {
				log.Errorf("failed to get relation: %s", err)
				continue
			}
			s.AddRelationLinks(&model.RelationLink{
				Format: rel.Format,
				Key:    rel.Key,
			})

			err = bs.relationService.ValidateFormat(detail.Key, detail.Value)
			if err != nil {
				log.Errorf("failed to validate relation: %s", err)
				continue
			}

			// special case for type relation that we are storing in a separate object's field
			if detail.Key == bundle.RelationKeyType.String() {
				ot := pbtypes.GetStringListValue(detail.Value)
				if len(ot) > 0 {
					s.SetObjectType(ot[0])
				}
			}
			detCopy.Fields[detail.Key] = detail.Value
		} else {
			delete(detCopy.Fields, detail.Key)
		}
	}
	if detCopy.Equal(s.CombinedDetails()) {
		return
	}

	s.SetDetails(detCopy)
	if err = bs.Apply(s); err != nil {
		return
	}

	// filter-out setDetails event
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
	return nil
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
		ot, err := objectstore.GetObjectType(bs.objectStore, objectTypes[0])
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

	otypes, err := objectstore.GetObjectTypes(bs.objectStore, objectTypes)
	if err != nil {
		return
	}
	if len(otypes) == 0 {
		return fmt.Errorf("object types not found")
	}

	ot := otypes[len(otypes)-1]

	prevType, _ := objectstore.GetObjectType(bs.objectStore, s.ObjectType())

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
	fromLayout, _ := s.Layout()

	s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(toLayout)))

	if err = bs.layoutConverter.Convert(s, fromLayout, toLayout); err != nil {
		return fmt.Errorf("convert layout: %w", err)
	}
	return nil
}
