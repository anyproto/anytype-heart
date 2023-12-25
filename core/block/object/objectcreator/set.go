package objectcreator

import (
	"context"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) createSet(ctx context.Context, space clientspace.Space, req *pb.RpcObjectCreateSetRequest) (setID string, newDetails *types.Struct, err error) {
	req.Details = internalflag.PutToDetails(req.Details, req.InternalFlags)

	dvContent, err := dataview.BlockBySource(s.objectStore, req.Source)
	if err != nil {
		return
	}

	newState := state.NewDoc("", nil).NewState()
	if len(req.Source) > 0 {
		newState.SetDetailAndBundledRelation(bundle.RelationKeySetOf, pbtypes.StringList(req.Source))
	}
	newState.AddDetails(req.Details)
	newState.BlocksInit(newState)

	tmpls := []template.StateTransformer{
		template.WithRequiredRelations(),
	}

	for i, view := range dvContent.Dataview.Views {
		if view.Relations == nil {
			dvContent.Dataview.Views[i].Relations = editor.GetDefaultViewRelations(dvContent.Dataview.Relations)
		}
	}
	tmpls = append(tmpls,
		template.WithDataview(dvContent, false),
	)

	template.InitTemplate(newState, tmpls...)

	return s.createSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeySet}, newState)
}
