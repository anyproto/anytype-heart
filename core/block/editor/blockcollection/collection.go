package blockcollection

import (
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app/ocache"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var ErrObjectNotFound = fmt.Errorf("collection object not found")

func NewCollection(sb smartblock.SmartBlock, objectStore spaceindex.Store) Collection {
	return &objectLinksCollection{SmartBlock: sb, objectStore: objectStore}
}

type Collection interface {
	AddObject(id string) (err error)
	HasObject(id string) (exists bool, linkId string)
	RemoveObject(id string) (err error)
	GetIds() (ids []string, err error)
	ModifyLocalDetails(
		objectId string,
		modifier func(current *domain.Details) (*domain.Details, error),
	) (err error)
}

type objectLinksCollection struct {
	smartblock.SmartBlock
	objectStore spaceindex.Store
}

func (p *objectLinksCollection) AddObject(id string) (err error) {
	s := p.NewState()
	var found bool
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil && link.TargetBlockId == id {
			found = true
			return false
		}
		return true
	})
	if found {
		return
	}

	link := simple.New(&model.Block{
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: id,
				Style:         model.BlockContentLink_Page,
			},
		},
	})
	s.Add(link)
	var lastTarget string
	if s == nil || s.Get(s.RootId()) == nil || s.Get(s.RootId()).Model() == nil {
		// todo: find a reason of empty state
		return fmt.Errorf("root block not found")
	}
	if chIds := s.Get(s.RootId()).Model().GetChildrenIds(); len(chIds) > 0 {
		lastTarget = chIds[0]
	}
	if err = s.InsertTo(lastTarget, model.Block_Top, link.Model().Id); err != nil {
		return
	}
	return p.Apply(s, smartblock.NoHistory)
}

func (p *objectLinksCollection) HasObject(id string) (exists bool, linkId string) {
	s := p.NewState()
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil && link.TargetBlockId == id {
			exists = true
			linkId = b.Model().Id
			return false
		}
		return true
	})

	return
}

func (p *objectLinksCollection) RemoveObject(id string) (err error) {
	s := p.NewState()
	exists, linkId := p.HasObject(id)
	if !exists {
		return ErrObjectNotFound
	}

	s.Unlink(linkId)
	return p.Apply(s, smartblock.NoHistory)
}

func (p *objectLinksCollection) GetIds() (ids []string, err error) {
	err = p.Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil {
			ids = append(ids, link.TargetBlockId)
		}
		return true
	})
	return
}

// ModifyLocalDetails modifies local details of the object in cache,
// and if it is not found, sets pending details in object store
func (p *objectLinksCollection) ModifyLocalDetails(
	objectId string,
	modifier func(current *domain.Details) (*domain.Details, error),
) (err error) {
	if modifier == nil {
		return fmt.Errorf("modifier is nil")
	}
	// we set pending details if object is not in cache
	// we do this under lock to prevent races if the object is created in parallel
	// because in that case we can lose changes
	err = p.Space().DoLockedIfNotExists(objectId, func() error {
		return p.objectStore.UpdatePendingLocalDetails(objectId, modifier)
	})
	if err != nil && !errors.Is(err, ocache.ErrExists) {
		return err
	}
	err = p.Space().Do(objectId, func(b smartblock.SmartBlock) error {
		// we just need to invoke the smartblock, so it reads from pending details
		// no need to call modify twice
		if err == nil {
			return b.Apply(b.NewState())
		}

		dets, err := modifier(b.CombinedDetails())
		if err != nil {
			return err
		}

		return b.Apply(b.NewState().SetDetails(dets), smartblock.KeepInternalFlags)
	})
	return err
}
