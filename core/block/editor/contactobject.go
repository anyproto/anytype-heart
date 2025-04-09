package editor

import (
	"context"
	"slices"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/editor/userdataobject"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

type techSpaceProvider interface {
	TechSpace() *clientspace.TechSpace
}

type ContactObject struct {
	basic.DetailsSettable
	smartblock.SmartBlock
	store             spaceindex.Store
	techSpaceProvider techSpaceProvider
}

func NewContactObject(smartBlock smartblock.SmartBlock, store spaceindex.Store, techSpaceProvider techSpaceProvider) *ContactObject {
	return &ContactObject{SmartBlock: smartBlock, store: store, techSpaceProvider: techSpaceProvider}
}

func (co *ContactObject) Init(ctx *smartblock.InitContext) error {
	err := co.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}
	records, err := co.store.QueryByIds([]string{co.Id()})
	if err != nil {
		return err
	}
	if len(records) > 0 {
		ctx.State.SetDetails(records[0].Details)
	}
	template.InitTemplate(ctx.State,
		template.WithEmpty,
		template.WithTitle,
		template.WithForcedObjectTypes([]domain.TypeKey{bundle.TypeKeyContact}),
		template.WithLayout(model.ObjectType_contact))
	return nil
}

func (co *ContactObject) SetDetails(ctx session.Context, details []domain.Detail, showEvent bool) (err error) {
	state := co.NewStateCtx(ctx)
	for _, detail := range details {
		if !slices.Contains(userdataobject.AllowedDetailsToChange(), detail.Key) {
			continue
		}
		state.SetDetail(detail.Key, detail.Value)
	}
	err = co.Apply(state)
	if err != nil {
		return err
	}
	combinedDetails := state.CombinedDetails()
	co.updateContactInStore(combinedDetails)
	return nil
}

func (co *ContactObject) updateContactInStore(combinedDetails *domain.Details) {
	space := co.techSpaceProvider.TechSpace()
	ctx := context.Background()
	err := space.DoUserDataObject(ctx, func(userDataObject userdataobject.UserDataObject) error {
		return userDataObject.UpdateContactByDetails(ctx, co.Id(), combinedDetails)
	})
	if err != nil {
		log.Errorf("failed to update user data object: %v", err)
	}
}
