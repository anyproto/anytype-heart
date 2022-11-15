package basic

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/latex"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/relation"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

type Basic interface {
	Movable
	Unlinkable

	Create(ctx *session.Context, groupId string, req pb.RpcBlockCreateRequest) (id string, err error)
	Replace(ctx *session.Context, id string, block *model.Block) (newId string, err error)
	SetFields(ctx *session.Context, fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error)
	Update(ctx *session.Context, apply func(b simple.Block) error, blockIds ...string) (err error)
	SetDivStyle(ctx *session.Context, style model.BlockContentDivStyle, ids ...string) (err error)

	PasteBlocks(blocks []simple.Block, position model.BlockPosition) (err error)
	SetRelationKey(ctx *session.Context, req pb.RpcBlockRelationSetKeyRequest) error
	SetLatexText(ctx *session.Context, req pb.RpcBlockLatexSetTextRequest) error
	AddRelationAndSet(ctx *session.Context, req pb.RpcBlockRelationAddRequest) error
	FeaturedRelationAdd(ctx *session.Context, relations ...string) error
	FeaturedRelationRemove(ctx *session.Context, relations ...string) error
	ReplaceLink(oldId, newId string) error

	ExtractBlocksToObjects(ctx *session.Context, s ObjectCreator, req pb.RpcBlockListConvertToObjectsRequest) (linkIds []string, err error)
}

type Movable interface {
	Move(ctx *session.Context, req pb.RpcBlockListMoveToExistingObjectRequest) error
}

type Unlinkable interface {
	Unlink(ctx *session.Context, id ...string) (err error)
}

var ErrNotSupported = fmt.Errorf("operation not supported for this type of smartblock")

func (bs *basic) PasteBlocks(blocks []simple.Block, position model.BlockPosition) (err error) {
	s := bs.NewState()
	if err := PasteBlocks(s, blocks, "", position); err != nil {
		return fmt.Errorf("paste blocks: %w", err)
	}
	return bs.Apply(s)
}

func NewBasic(sb smartblock.SmartBlock) Basic {
	return &basic{sb}
}

type basic struct {
	smartblock.SmartBlock
}

func (bs *basic) Create(ctx *session.Context, groupId string, req pb.RpcBlockCreateRequest) (id string, err error) {
	if err = bs.Restrictions().Object.Check(model.Restrictions_Blocks); err != nil {
		return
	}
	if bs.Type() == model.SmartBlockType_Set {
		return "", ErrNotSupported
	}

	s := bs.NewStateCtx(ctx).SetGroupId(groupId)

	id, err = CreateBlock(s, groupId, req)
	if err != nil {
		return "", fmt.Errorf("create block: %w", err)
	}

	if err = bs.Apply(s); err != nil {
		return
	}
	return id, nil
}

func Duplicate(req pb.RpcBlockListDuplicateRequest, srcState, destState *state.State) (newIds []string, err error) {

	pos := req.Position
	targetId := req.TargetId
	for _, id := range req.BlockIds {
		copyId, e := copyBlocks(srcState, destState, id)
		if e != nil {
			return nil, e
		}
		if err = destState.InsertTo(targetId, pos, copyId); err != nil {
			return
		}
		pos = model.Block_Bottom
		targetId = copyId
		newIds = append(newIds, copyId)
	}
	return
}

// some types of blocks need a special duplication mechanism
type duplicatable interface {
	Duplicate(s *state.State) (newId string, visitedIds []string, blocks []simple.Block, err error)
}

func copyBlocks(srcState, destState *state.State, sourceId string) (id string, err error) {
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
		if result.Model().ChildrenIds[i], err = copyBlocks(srcState, destState, childrenId); err != nil {
			return
		}
	}
	return result.Model().Id, nil
}

func (bs *basic) Unlink(ctx *session.Context, ids ...string) (err error) {
	if bs.Type() == model.SmartBlockType_Set {
		return ErrNotSupported
	}

	s := bs.NewStateCtx(ctx)

	var someUnlinked bool
	for _, id := range ids {
		if s.Unlink(id) {
			someUnlinked = true
		}
	}
	if !someUnlinked {
		return smartblock.ErrSimpleBlockNotFound
	}
	return bs.Apply(s)
}

func (bs *basic) Move(ctx *session.Context, req pb.RpcBlockListMoveToExistingObjectRequest) (err error) {
	if bs.Type() == model.SmartBlockType_Set {
		return ErrNotSupported
	}

	s := bs.NewStateCtx(ctx)
	if req.DropTargetId != "" {
		if s.IsChild(template.HeaderLayoutId, req.DropTargetId) || req.DropTargetId == template.HeaderLayoutId {
			req.Position = model.Block_Bottom
			req.DropTargetId = template.HeaderLayoutId
		}
	}

	var replacementCandidate simple.Block
	for _, id := range req.BlockIds {
		if b := s.Pick(id); b != nil {
			if replacementCandidate == nil {
				replacementCandidate = s.Get(id)
			}
			s.Unlink(id)
		}
	}

	if req.DropTargetId == "" {
		req.DropTargetId = s.RootId()
		req.Position = model.Block_Inner
	}
	target := s.Get(req.DropTargetId)
	if target == nil {
		return fmt.Errorf("target block not found")
	}

	if targetContent, ok := target.Model().Content.(*model.BlockContentOfText); ok && targetContent.Text != nil {
		if targetContent.Text.Style == model.BlockContentText_Paragraph &&
			targetContent.Text.Text == "" && req.Position == model.Block_InnerFirst {

			req.Position = model.Block_Replace

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

	if err = s.InsertTo(req.DropTargetId, req.Position, req.BlockIds...); err != nil {
		return
	}
	return bs.Apply(s)
}

func (bs *basic) Replace(ctx *session.Context, id string, block *model.Block) (newId string, err error) {
	if bs.Type() == model.SmartBlockType_Set {
		return "", ErrNotSupported
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

func (bs *basic) SetFields(ctx *session.Context, fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error) {
	s := bs.NewStateCtx(ctx)
	for _, fr := range fields {
		if b := s.Get(fr.BlockId); b != nil {
			b.Model().Fields = fr.Fields
		}
	}
	return bs.Apply(s)
}

func (bs *basic) Update(ctx *session.Context, apply func(b simple.Block) error, blockIds ...string) (err error) {
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

func (bs *basic) SetDivStyle(ctx *session.Context, style model.BlockContentDivStyle, ids ...string) (err error) {
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

func (bs *basic) SetRelationKey(ctx *session.Context, req pb.RpcBlockRelationSetKeyRequest) (err error) {
	s := bs.NewStateCtx(ctx)
	b := s.Get(req.BlockId)
	if b == nil {
		return smartblock.ErrSimpleBlockNotFound
	}
	if !bs.HasRelation(s, req.Key) {
		return fmt.Errorf("relation with given key not found")
	}
	if rel, ok := b.(relation.Block); ok {
		rel.SetKey(req.Key)
	} else {
		return fmt.Errorf("unexpected block type: %T (want relation)", b)
	}
	return bs.Apply(s)
}

func (bs *basic) SetLatexText(ctx *session.Context, req pb.RpcBlockLatexSetTextRequest) (err error) {
	s := bs.NewStateCtx(ctx)
	b := s.Get(req.BlockId)
	if b == nil {
		return smartblock.ErrSimpleBlockNotFound
	}

	if rel, ok := b.(latex.Block); ok {
		rel.SetText(req.Text)
	} else {
		return fmt.Errorf("unexpected block type: %T (want latex)", b)
	}
	return bs.Apply(s, smartblock.NoEvent)
}

func (bs *basic) AddRelationAndSet(ctx *session.Context, req pb.RpcBlockRelationAddRequest) (err error) {
	s := bs.NewStateCtx(ctx)
	b := s.Get(req.BlockId)
	if b == nil {
		return smartblock.ErrSimpleBlockNotFound
	}

	rel, err := bs.RelationService().FetchKey(req.RelationKey)
	if err != nil {
		return
	}

	if rb, ok := b.(relation.Block); ok {
		rb.SetKey(rel.Key)
	} else {
		return fmt.Errorf("unexpected block type: %T (want relation)", b)
	}
	s.AddRelationLinks(rel.RelationLink())
	return bs.Apply(s)
}

func (bs *basic) FeaturedRelationAdd(ctx *session.Context, relations ...string) (err error) {
	s := bs.NewStateCtx(ctx)
	fr := pbtypes.GetStringList(s.Details(), bundle.RelationKeyFeaturedRelations.String())
	frc := make([]string, len(fr))
	copy(frc, fr)
	for _, r := range relations {
		if bs.HasRelation(s, r) && slice.FindPos(frc, r) == -1 {
			frc = append(frc, r)
		}
	}
	if len(frc) != len(fr) {
		s.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList(frc))
		template.WithDescription(s)
	}
	return bs.Apply(s, smartblock.NoRestrictions)
}

func (bs *basic) FeaturedRelationRemove(ctx *session.Context, relations ...string) (err error) {
	s := bs.NewStateCtx(ctx)
	fr := pbtypes.GetStringList(s.Details(), bundle.RelationKeyFeaturedRelations.String())
	frc := make([]string, len(fr))
	copy(frc, fr)
	for _, r := range relations {
		if slice.FindPos(frc, r) != -1 {
			frc = slice.Remove(frc, r)
		}
	}
	if len(frc) != len(fr) {
		s.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList(frc))
		template.WithDescription(s)
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
			if pbtypes.GetString(details, rel.Key) == oldId {
				s.SetDetail(rel.Key, pbtypes.String(newId))
			}
		}
	}
	return bs.Apply(s)
}
