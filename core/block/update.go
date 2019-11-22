package block

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
)

func (p *commonSmart) applyChanges(updateCtx uniqueIds, changes *pb.ChangesBlock) (origBlock simple.Block, err error) {
	if v, ok := p.versions[changes.Id]; ok {
		if v.Virtual() {
			err = fmt.Errorf("can't update virtual block[%s]", changes.Id)
			return
		}
		origBlock = v
	} else {
		err = fmt.Errorf("can't update block[%s]: not found", changes.Id)
		return
	}
	block := simple.Copy(origBlock)
	if changes.ChildrenIds != nil {
		if err = p.updateChildrenIds(block.Model(), changes.ChildrenIds.ChildrenIds); err != nil {
			return
		}
	}
	if changes.IsArchived != block.Model().IsArchived {
		if err = p.updateIsArchived(block.Model(), changes.IsArchived); err != nil {
			return
		}
	}
	if changes.Fields != nil {
		if err = p.updateFields(block.Model(), changes.Fields); err != nil {
			return
		}
	}
	if changes.Content != nil && changes.Content.Content != nil {
		if err = block.ApplyContentChanges(changes.Content.Content); err != nil {
			return
		}
	}
	if changes.Permissions != nil {
		if err = p.updatePermissions(block.Model(), changes.Permissions); err != nil {
			return
		}
	}
	p.versions[changes.Id] = block
	updateCtx.Add(changes.Id)
	return
}

func (p *commonSmart) updateChildrenIds(b *model.Block, childrenIds []string) (err error) {
	b.ChildrenIds = childrenIds
	return p.validateChildrenIds(b)
}

func (p *commonSmart) updateIsArchived(b *model.Block, isArchived bool) (err error) {
	return
}

func (p *commonSmart) updateFields(b *model.Block, fields *types.Struct) (err error) {
	b.Fields = fields
	return
}

func (p *commonSmart) updateContent(b *model.Block, content model.IsBlockCoreContent) (err error) {
	// TODO: validate content
	b.Content = &model.BlockCore{Content: content}
	return
}

func (p *commonSmart) updatePermissions(b *model.Block, permissions *model.BlockPermissions) (err error) {
	return
}
