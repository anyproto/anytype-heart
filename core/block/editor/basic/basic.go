package basic

import (
	"fmt"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/block/simple/embed"
	"github.com/anyproto/anytype-heart/core/block/simple/link"
	relationblock "github.com/anyproto/anytype-heart/core/block/simple/relation"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	templateSvc "github.com/anyproto/anytype-heart/core/block/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

type AllOperations interface {
	Movable
	Duplicatable
	Unlinkable
	Creatable
	Replaceable
	Updatable

	CommonOperations
}

type CommonOperations interface {
	DetailsSettable
	DetailsUpdatable

	SetFields(ctx session.Context, fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error)
	SetDivStyle(ctx session.Context, style model.BlockContentDivStyle, ids ...string) (err error)
	SetLatexText(ctx session.Context, req pb.RpcBlockLatexSetTextRequest) error

	SetRelationKey(ctx session.Context, req pb.RpcBlockRelationSetKeyRequest) error
	AddRelationAndSet(ctx session.Context, req pb.RpcBlockRelationAddRequest) error
	FeaturedRelationAdd(ctx session.Context, relations ...string) error
	FeaturedRelationRemove(ctx session.Context, relations ...string) error

	ReplaceLink(oldId, newId string) error
	ExtractBlocksToObjects(ctx session.Context, oc ObjectCreator, tsc templateSvc.Service, req pb.RpcBlockListConvertToObjectsRequest) (linkIds []string, err error)

	SetObjectTypes(ctx session.Context, objectTypeKeys []domain.TypeKey, ignoreRestrictions bool) (err error)
	SetObjectTypesInState(s *state.State, objectTypeKeys []domain.TypeKey, ignoreRestrictions bool) (err error)
	SetLayout(ctx session.Context, layout model.ObjectTypeLayout) (err error)
	SetLayoutInState(s *state.State, layout model.ObjectTypeLayout, ignoreRestriction bool) (err error)
}

type DetailsSettable interface {
	SetDetails(ctx session.Context, details []domain.Detail, showEvent bool) (err error)
}

type DetailsUpdatable interface {
	UpdateDetails(ctx session.Context, update func(current *domain.Details) (*domain.Details, error)) (err error)
}

type Restrictionable interface {
	Restrictions() restriction.Restrictions
}

type Movable interface {
	Move(srcState, destState *state.State, targetBlockId string, position model.BlockPosition, blockIds []string) error
}

type Duplicatable interface {
	Duplicate(srcState, destState *state.State, targetBlockId string, position model.BlockPosition, blockIds []string) (newIds []string, err error)
}

type Unlinkable interface {
	Unlink(ctx session.Context, id ...string) (err error)
}

type Creatable interface {
	CreateBlock(s *state.State, req pb.RpcBlockCreateRequest) (id string, err error)
}

type Replaceable interface {
	Replace(ctx session.Context, id string, block *model.Block) (newId string, err error)
}

type Updatable interface {
	Update(ctx session.Context, apply func(b simple.Block) error, blockIds ...string) (err error)
}

func NewBasic(
	sb smartblock.SmartBlock,
	objectStore spaceindex.Store,
	layoutConverter converter.LayoutConverter,
	fileObjectService fileobject.Service,
) AllOperations {
	return &basic{
		SmartBlock:        sb,
		objectStore:       objectStore,
		layoutConverter:   layoutConverter,
		fileObjectService: fileObjectService,
	}
}

type basic struct {
	smartblock.SmartBlock

	objectStore       spaceindex.Store
	layoutConverter   converter.LayoutConverter
	fileObjectService fileobject.Service
}

func (bs *basic) CreateBlock(s *state.State, req pb.RpcBlockCreateRequest) (id string, err error) {
	if err = bs.Restrictions().Object.Check(model.Restrictions_Blocks); err != nil {
		return
	}
	if req.TargetId != "" {
		if s.IsChild(template.HeaderLayoutId, req.TargetId) {
			req.Position = model.Block_Bottom
			req.TargetId = template.HeaderLayoutId
		}
	}
	if req.Block.GetContent() == nil {
		err = fmt.Errorf("no block content")
		return
	}
	req.Block.Id = ""
	block := simple.New(req.Block)
	block.Model().ChildrenIds = nil
	err = block.Validate()
	if err != nil {
		return
	}
	s.Add(block)
	if err = s.InsertTo(req.TargetId, req.Position, block.Model().Id); err != nil {
		return
	}
	return block.Model().Id, nil
}

func (bs *basic) Duplicate(srcState, destState *state.State, targetBlockId string, position model.BlockPosition, blockIds []string) (newIds []string, err error) {
	blockIds = srcState.SelectRoots(blockIds)
	for _, id := range blockIds {
		copyId, e := bs.copyBlocks(srcState, destState, id)
		if e != nil {
			return nil, e
		}
		if err = destState.InsertTo(targetBlockId, position, copyId); err != nil {
			return
		}
		position = model.Block_Bottom
		targetBlockId = copyId
		newIds = append(newIds, copyId)
	}
	return
}

// some types of blocks need a special duplication mechanism
// TODO: maybe name this Copy? Duplication is copying and pasting, but this method only copies blocks into memory
type duplicatable interface {
	Duplicate(s *state.State) (newId string, visitedIds []string, blocks []simple.Block, err error)
}

func (bs *basic) copyBlocks(srcState, destState *state.State, sourceId string) (id string, err error) {
	b := srcState.Pick(sourceId)
	if b == nil {
		return "", smartblock.ErrSimpleBlockNotFound
	}
	if v, ok := b.(duplicatable); ok {
		newId, _, blocks, err := v.Duplicate(srcState)
		if err != nil {
			return "", fmt.Errorf("custom block duplication: %w", err)
		}
		for _, b := range blocks {
			destState.Add(b)
		}
		return newId, nil
	}

	m := b.Copy().Model()
	m.Id = "" // reset id
	result := simple.New(m)
	destState.Add(result)
	for i, childrenId := range result.Model().ChildrenIds {
		if result.Model().ChildrenIds[i], err = bs.copyBlocks(srcState, destState, childrenId); err != nil {
			return
		}
	}

	if f, ok := result.Model().Content.(*model.BlockContentOfFile); ok && srcState.SpaceID() != destState.SpaceID() {
		bs.processFileBlock(f, destState.SpaceID())
	}

	return result.Model().Id, nil
}

func (bs *basic) processFileBlock(f *model.BlockContentOfFile, spaceId string) {
	fileId, err := bs.fileObjectService.GetFileIdFromObject(f.File.TargetObjectId)
	if err != nil {
		log.Errorf("failed to get fileId: %v", err)
		return
	}

	objectId, err := bs.fileObjectService.CreateFromImport(
		domain.FullFileId{SpaceId: spaceId, FileId: fileId.FileId},
		objectorigin.ObjectOrigin{Origin: model.ObjectOrigin_clipboard},
	)
	if err != nil {
		log.Errorf("failed to create file object: %v", err)
		return
	}

	f.File.TargetObjectId = objectId
}

func (bs *basic) Unlink(ctx session.Context, ids ...string) (err error) {
	s := bs.NewStateCtx(ctx)

	var someUnlinked bool
	for _, id := range ids {
		if !state.IsRequiredBlockId(id) {
			if s.Unlink(id) {
				someUnlinked = true
			}
		}
	}
	if !someUnlinked {
		return smartblock.ErrSimpleBlockNotFound
	}
	return bs.Apply(s)
}

func (bs *basic) Move(srcState, destState *state.State, targetBlockId string, position model.BlockPosition, blockIds []string) (err error) {
	if lo.ContainsBy(blockIds, state.IsRequiredBlockId) {
		return fmt.Errorf("can not move required block")
	}

	if srcState != destState && destState != nil {
		_, err := bs.Duplicate(srcState, destState, targetBlockId, position, blockIds)
		if err != nil {
			return fmt.Errorf("paste: %w", err)
		}
		for _, id := range blockIds {
			srcState.Unlink(id)
		}
		return nil
	}

	if targetBlockId != "" {
		if srcState.IsChild(template.HeaderLayoutId, targetBlockId) || targetBlockId == template.HeaderLayoutId {
			position = model.Block_Bottom
			targetBlockId = template.HeaderLayoutId
		}
	}

	targetBlockId, position, err = table.CheckTableBlocksMove(srcState, targetBlockId, position, blockIds)
	if err != nil {
		return err
	}

	var replacementCandidate simple.Block
	for _, id := range blockIds {
		if b := srcState.Pick(id); b != nil {
			if replacementCandidate == nil {
				replacementCandidate = srcState.Get(id)
			}
			srcState.Unlink(id)
		}
	}

	if targetBlockId == "" {
		targetBlockId = srcState.RootId()
		position = model.Block_Inner
	}
	target := srcState.Get(targetBlockId)
	if target == nil {
		return fmt.Errorf("target block not found")
	}

	if targetContent, ok := target.Model().Content.(*model.BlockContentOfText); ok && targetContent.Text != nil {
		if targetContent.Text.Style == model.BlockContentText_Paragraph &&
			targetContent.Text.Text == "" && position == model.Block_InnerFirst {

			position = model.Block_Replace

			if replacementCandidate != nil {
				if replacementCandidate.Model().BackgroundColor == "" {
					replacementCandidate.Model().BackgroundColor = target.Model().BackgroundColor
				}
			}

			if replacementContent, ok := replacementCandidate.Model().Content.(*model.BlockContentOfText); ok {
				if replacementContent.Text.Color == "" {
					replacementContent.Text.Color = targetContent.Text.Color
				}
			}
		}
	}

	return srcState.InsertTo(targetBlockId, position, blockIds...)
}

func (bs *basic) Replace(ctx session.Context, id string, block *model.Block) (newId string, err error) {
	if state.IsRequiredBlockId(id) {
		return "", fmt.Errorf("can not replace required block")
	}
	s := bs.NewStateCtx(ctx)
	if block.GetContent() == nil {
		err = fmt.Errorf("no block content")
		return
	}
	new := simple.New(block)
	newId = new.Model().Id
	new.Model().ChildrenIds = nil
	err = new.Validate()
	if err != nil {
		return
	}
	s.Add(new)
	if err = s.InsertTo(id, model.Block_Replace, newId); err != nil {
		return
	}
	if err = bs.Apply(s); err != nil {
		return
	}
	return
}

func (bs *basic) SetFields(ctx session.Context, fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error) {
	s := bs.NewStateCtx(ctx)
	for _, fr := range fields {
		if b := s.Get(fr.BlockId); b != nil {
			b.Model().Fields = fr.Fields
		}
	}
	return bs.Apply(s)
}

func (bs *basic) Update(ctx session.Context, apply func(b simple.Block) error, blockIds ...string) (err error) {
	s := bs.NewStateCtx(ctx)
	for _, id := range blockIds {
		if b := s.Get(id); b != nil {
			if err = apply(b); err != nil {
				return
			}
		} else {
			return smartblock.ErrSimpleBlockNotFound
		}
	}
	return bs.Apply(s)
}

func (bs *basic) SetDivStyle(ctx session.Context, style model.BlockContentDivStyle, ids ...string) (err error) {
	s := bs.NewStateCtx(ctx)
	for _, id := range ids {
		b := s.Get(id)
		if b == nil {
			return smartblock.ErrSimpleBlockNotFound
		}
		if div, ok := b.(base.DivBlock); ok {
			div.SetStyle(style)
		} else {
			return fmt.Errorf("unexpected block type: %T (want Div)", b)
		}
	}
	return bs.Apply(s)
}

func (bs *basic) SetRelationKey(ctx session.Context, req pb.RpcBlockRelationSetKeyRequest) (err error) {
	s := bs.NewStateCtx(ctx)
	b := s.Get(req.BlockId)
	if b == nil {
		return smartblock.ErrSimpleBlockNotFound
	}
	if !bs.HasRelation(s, req.Key) {
		return fmt.Errorf("relation with given key not found")
	}
	if rel, ok := b.(relationblock.Block); ok {
		rel.SetKey(req.Key)
	} else {
		return fmt.Errorf("unexpected block type: %T (want relation)", b)
	}
	return bs.Apply(s)
}

func (bs *basic) SetLatexText(ctx session.Context, req pb.RpcBlockLatexSetTextRequest) (err error) {
	s := bs.NewStateCtx(ctx)
	b := s.Get(req.BlockId)
	if b == nil {
		return smartblock.ErrSimpleBlockNotFound
	}

	if rel, ok := b.(embed.Block); ok {
		rel.SetText(req.Text)
	} else {
		return fmt.Errorf("unexpected block type: %T (want embed)", b)
	}
	return bs.Apply(s, smartblock.NoEvent)
}

func (bs *basic) AddRelationAndSet(ctx session.Context, req pb.RpcBlockRelationAddRequest) (err error) {
	s := bs.NewStateCtx(ctx)
	b := s.Get(req.BlockId)
	if b == nil {
		return smartblock.ErrSimpleBlockNotFound
	}

	rel, err := bs.objectStore.FetchRelationByKey(req.RelationKey)
	if err != nil {
		return
	}

	if rb, ok := b.(relationblock.Block); ok {
		rb.SetKey(rel.Key)
	} else {
		return fmt.Errorf("unexpected block type: %T (want relation)", b)
	}
	s.AddRelationLinks(rel.RelationLink())
	return bs.Apply(s)
}

func (bs *basic) FeaturedRelationAdd(ctx session.Context, relations ...string) (err error) {
	s := bs.NewStateCtx(ctx)
	fr := s.Details().GetStringList(bundle.RelationKeyFeaturedRelations)
	frc := make([]string, len(fr))
	copy(frc, fr)
	for _, r := range relations {
		if slice.FindPos(frc, r) == -1 {
			// special case
			if r == bundle.RelationKeyDescription.String() {
				// todo: looks like it's not ok to use templates here but it has a lot of logic inside
				template.WithForcedDescription(s)
			}
			frc = append(frc, r)
			if !bs.HasRelation(s, r) {
				err = bs.addRelationLink(s, domain.RelationKey(r))
				if err != nil {
					return fmt.Errorf("failed to add relation link on adding featured relation '%s': %w", r, err)
				}
			}
		}
	}
	if len(frc) != len(fr) {
		s.SetDetail(bundle.RelationKeyFeaturedRelations, domain.StringList(frc))
	}
	return bs.Apply(s, smartblock.NoRestrictions)
}

func (bs *basic) FeaturedRelationRemove(ctx session.Context, relations ...string) (err error) {
	s := bs.NewStateCtx(ctx)
	fr := s.Details().GetStringList(bundle.RelationKeyFeaturedRelations)
	frc := make([]string, len(fr))
	copy(frc, fr)
	for _, r := range relations {
		if slice.FindPos(frc, r) != -1 {
			// special cases
			switch r {
			case bundle.RelationKeyDescription.String():
				s.Unlink(state.DescriptionBlockID)
			}
			frc = slice.RemoveMut(frc, r)
		}
	}
	if len(frc) != len(fr) {
		s.SetDetail(bundle.RelationKeyFeaturedRelations, domain.StringList(frc))
	}
	return bs.Apply(s, smartblock.NoRestrictions)
}

func (bs *basic) ReplaceLink(oldId, newId string) error {
	s := bs.NewState()
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if l, ok := b.(link.Block); ok {
			if l.Model().GetLink().TargetBlockId == oldId {
				s.Get(b.Model().Id).Model().GetLink().TargetBlockId = newId
			}
		} else if t, ok := b.(text.Block); ok {
			if marks := t.Model().GetText().Marks; marks != nil {
				for i, m := range marks.Marks {
					if m.Param == oldId {
						s.Get(b.Model().Id).Model().GetText().Marks.Marks[i].Param = newId
					}
				}
			}
		}
		return true
	})
	// TODO: use relations service with state
	rels := bs.GetRelationLinks()
	details := s.Details()
	for _, rel := range rels {
		if rel.Format == model.RelationFormat_object {
			key := domain.RelationKey(rel.Key)
			if details.GetString(key) == oldId {
				s.SetDetail(key, domain.String(newId))
			}
		}
	}
	return bs.Apply(s)
}
