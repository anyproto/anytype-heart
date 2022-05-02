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
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/globalsign/mgo/bson"
)

type Basic interface {
	Create(ctx *state.Context, groupId string, req pb.RpcBlockCreateRequest) (id string, err error)
	Duplicate(ctx *state.Context, req pb.RpcBlockListDuplicateRequest) (newIds []string, err error)
	Unlink(ctx *state.Context, id ...string) (err error)
	Move(ctx *state.Context, req pb.RpcBlockListMoveRequest) error
	Replace(ctx *state.Context, id string, block *model.Block) (newId string, err error)
	SetFields(ctx *state.Context, fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error)
	Update(ctx *state.Context, apply func(b simple.Block) error, blockIds ...string) (err error)
	SetDivStyle(ctx *state.Context, style model.BlockContentDivStyle, ids ...string) (err error)
	InternalCut(ctx *state.Context, req pb.RpcBlockListMoveRequest) (apply func() error, blocks []simple.Block, err error)
	InternalPaste(blocks []simple.Block) (err error)
	SetRelationKey(ctx *state.Context, req pb.RpcBlockRelationSetKeyRequest) error
	SetLatexText(ctx *state.Context, req pb.RpcBlockSetLatexTextRequest) error
	AddRelationAndSet(ctx *state.Context, req pb.RpcBlockRelationAddRequest) error
	FeaturedRelationAdd(ctx *state.Context, relations ...string) error
	FeaturedRelationRemove(ctx *state.Context, relations ...string) error
	ReplaceLink(oldId, newId string) error
}

var ErrNotSupported = fmt.Errorf("operation not supported for this type of smartblock")

// InternalCut will only unlink blocks you've cut after you call apply()
func (bs *basic) InternalCut(ctx *state.Context, req pb.RpcBlockListMoveRequest) (apply func() error, blocks []simple.Block, err error) {
	s := bs.NewStateCtx(ctx)
	var uniqMap = make(map[string]struct{})
	for _, bId := range req.BlockIds {
		b := s.Pick(bId)
		if b != nil {
			descendants := bs.getAllDescendants(uniqMap, b.Copy(), []simple.Block{})
			blocks = append(blocks, descendants...)
			s.Unlink(b.Model().Id)
		}
	}

	return func() error { return bs.Apply(s) }, blocks, err
}

func (bs *basic) InternalPaste(blocks []simple.Block) (err error) {
	s := bs.NewState()
	childIdsRewrite := make(map[string]string)
	for _, b := range blocks {
		for i, cId := range b.Model().ChildrenIds {
			newId := bson.NewObjectId().Hex()
			childIdsRewrite[cId] = newId
			b.Model().ChildrenIds[i] = newId
		}
	}
	for _, b := range blocks {
		var child bool
		if newId, ok := childIdsRewrite[b.Model().Id]; ok {
			b.Model().Id = newId
			child = true
		} else {
			b.Model().Id = bson.NewObjectId().Hex()
		}
		s.Add(b)
		if !child {
			err := s.InsertTo("", model.Block_Inner, b.Model().Id)
			if err != nil {
				return err
			}
		}
	}
	return bs.Apply(s)
}

func NewBasic(sb smartblock.SmartBlock) Basic {
	return &basic{sb}
}

type basic struct {
	smartblock.SmartBlock
}

func (bs *basic) Create(ctx *state.Context, groupId string, req pb.RpcBlockCreateRequest) (id string, err error) {
	if err = bs.Restrictions().Object.Check(model.Restrictions_Blocks); err != nil {
		return
	}
	if bs.Type() == model.SmartBlockType_Set {
		return "", ErrNotSupported
	}
	s := bs.NewStateCtx(ctx).SetGroupId(groupId)
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
	if err = bs.Apply(s); err != nil {
		return
	}
	return block.Model().Id, nil
}

func (bs *basic) Duplicate(ctx *state.Context, req pb.RpcBlockListDuplicateRequest) (newIds []string, err error) {
	if bs.Type() == model.SmartBlockType_Set {
		return nil, ErrNotSupported
	}

	s := bs.NewStateCtx(ctx)
	pos := req.Position
	targetId := req.TargetId
	for _, id := range req.BlockIds {
		copyId, e := bs.copy(s, id)
		if e != nil {
			return nil, e
		}
		if err = s.InsertTo(targetId, pos, copyId); err != nil {
			return
		}
		pos = model.Block_Bottom
		targetId = copyId
		newIds = append(newIds, copyId)
	}
	if err = bs.Apply(s); err != nil {
		return
	}
	return
}

func (bs *basic) copy(s *state.State, sourceId string) (id string, err error) {
	b := s.Get(sourceId)
	if bs == nil {
		return "", smartblock.ErrSimpleBlockNotFound
	}
	m := b.Copy().Model()
	m.Id = "" // reset id
	copy := simple.New(m)
	s.Add(copy)
	for i, childrenId := range copy.Model().ChildrenIds {
		if copy.Model().ChildrenIds[i], err = bs.copy(s, childrenId); err != nil {
			return
		}
	}
	return copy.Model().Id, nil
}

func (bs *basic) Unlink(ctx *state.Context, ids ...string) (err error) {
	if bs.Type() == model.SmartBlockType_Set {
		return ErrNotSupported
	}

	s := bs.NewStateCtx(ctx)
	for _, id := range ids {
		if !s.Unlink(id) {
			return smartblock.ErrSimpleBlockNotFound
		}
	}
	return bs.Apply(s)
}

func (bs *basic) Move(ctx *state.Context, req pb.RpcBlockListMoveRequest) (err error) {
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

	target := s.Get(req.DropTargetId)
	if target == nil {
		return fmt.Errorf("target block not found")
	}

	if targetContent, ok := target.Model().Content.(*model.BlockContentOfText); ok && targetContent.Text != nil {
		if targetContent.Text.Style == model.BlockContentText_Paragraph && targetContent.Text.Text == "" {

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

func (bs *basic) Replace(ctx *state.Context, id string, block *model.Block) (newId string, err error) {
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

func (bs *basic) SetFields(ctx *state.Context, fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error) {
	s := bs.NewStateCtx(ctx)
	for _, fr := range fields {
		if b := s.Get(fr.BlockId); b != nil {
			b.Model().Fields = fr.Fields
		}
	}
	return bs.Apply(s)
}

func (bs *basic) Update(ctx *state.Context, apply func(b simple.Block) error, blockIds ...string) (err error) {
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

func (bs *basic) SetDivStyle(ctx *state.Context, style model.BlockContentDivStyle, ids ...string) (err error) {
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

func (bs *basic) SetRelationKey(ctx *state.Context, req pb.RpcBlockRelationSetKeyRequest) (err error) {
	s := bs.NewStateCtx(ctx)
	b := s.Get(req.BlockId)
	if b == nil {
		return smartblock.ErrSimpleBlockNotFound
	}
	if !bs.HasRelation(req.Key) {
		return fmt.Errorf("relation with given key not found")
	}
	if rel, ok := b.(relation.Block); ok {
		rel.SetKey(req.Key)
	} else {
		return fmt.Errorf("unexpected block type: %T (want relation)", b)
	}
	return bs.Apply(s)
}

func (bs *basic) SetLatexText(ctx *state.Context, req pb.RpcBlockSetLatexTextRequest) (err error) {
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

func (bs *basic) AddRelationAndSet(ctx *state.Context, req pb.RpcBlockRelationAddRequest) (err error) {
	s := bs.NewStateCtx(ctx)
	b := s.Get(req.BlockId)
	if b == nil {
		return smartblock.ErrSimpleBlockNotFound
	}
	key := req.Relation.Key
	if !s.HasRelation(key) {
		if req.Relation.Key == "" {
			req.Relation.Key = bson.NewObjectId().Hex()
		}
		s.AddRelation(req.Relation)
	}
	if rel, ok := b.(relation.Block); ok {
		rel.SetKey(req.Relation.Key)
	} else {
		return fmt.Errorf("unexpected block type: %T (want relation)", b)
	}
	return bs.Apply(s)
}

func (bs *basic) getAllDescendants(uniqMap map[string]struct{}, block simple.Block, blocks []simple.Block) []simple.Block {
	if _, ok := uniqMap[block.Model().Id]; ok {
		return blocks
	}
	blocks = append(blocks, block)
	uniqMap[block.Model().Id] = struct{}{}
	for _, cId := range block.Model().ChildrenIds {
		blocks = bs.getAllDescendants(uniqMap, bs.Pick(cId).Copy(), blocks)
	}
	return blocks
}

func (bs *basic) FeaturedRelationAdd(ctx *state.Context, relations ...string) (err error) {
	s := bs.NewStateCtx(ctx)
	fr := pbtypes.GetStringList(s.Details(), bundle.RelationKeyFeaturedRelations.String())
	frc := make([]string, len(fr))
	copy(frc, fr)
	for _, r := range relations {
		if bs.HasRelation(r) && slice.FindPos(frc, r) == -1 {
			frc = append(frc, r)
		}
	}
	if len(frc) != len(fr) {
		s.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList(frc))
		template.WithDescription(s)
	}
	return bs.Apply(s, smartblock.NoRestrictions)
}

func (bs *basic) FeaturedRelationRemove(ctx *state.Context, relations ...string) (err error) {
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
	rels := bs.RelationsState(s, true)
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
