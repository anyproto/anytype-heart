package editor

import (
	"errors"
	"fmt"

	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/spacecore/spacecore"
)

var ErrIncorrectSpaceInfo = errors.New("space info is incorrect")

// SpaceObject is a wrapper around smartblock.SmartBlock that provides
// additional functionality for space creation/deletion/etc
type SpaceObject struct {
	smartblock.SmartBlock

	spaceService   spacecore.SpaceService
	techSpace      spacecore.TechSpace
	indexer        spaceIndexer
	objectCache    objectcache.Cache
	derivedObjects threads.DerivedSmartblockIds
	spaceID        string
}

// spaceObjectDeps is a set of dependencies for SpaceObject
type spaceObjectDeps struct {
	spaceService spacecore.SpaceService
	techSpace    spacecore.TechSpace
	objectCache  objectcache.Cache
	indexer      spaceIndexer
}

// newSpaceObject creates a new SpaceObject with given deps
func newSpaceObject(sb smartblock.SmartBlock, deps spaceObjectDeps) *SpaceObject {
	return &SpaceObject{
		SmartBlock:   sb,
		spaceService: deps.spaceService,
		techSpace:    deps.techSpace,
		objectCache:  deps.objectCache,
		indexer:      deps.indexer,
	}
}

// SpaceID returns space id of the space object
func (p *SpaceObject) SpaceID() string {
	return p.spaceID
}

// DerivedIDs returns derived smartblock ids
func (p *SpaceObject) DerivedIDs() threads.DerivedSmartblockIds {
	return p.derivedObjects
}

// Init initializes SpaceObject
func (p *SpaceObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	// get space id from the root
	p.spaceID, err = p.targetSpaceID()
	if err != nil {
		return
	}
	// TODO: check if we should even load the space
	sp, err := p.spaceService.Get(ctx.Ctx, p.spaceID)
	if err != nil {
		return
	}
	fmt.Println(sp.StoredIds(), "ids of space with", p.spaceID)
	p.derivedObjects, err = p.techSpace.PredefinedObjects(ctx.Ctx, sp, false)
	if err != nil {
		return
	}
	for _, id := range p.derivedObjects.IDs() {
		_, err := p.objectCache.ResolveObject(ctx.Ctx, id)
		if err != nil {
			return err
		}
	}
	err = p.techSpace.PreinstalledObjects(ctx.Ctx, p.spaceID)
	if err != nil {
		return
	}
	// TODO: [MR] we should save the flags somewhere
	err = p.indexer.ReindexSpace(p.spaceID, p.spaceID == p.spaceService.AccountId())
	if err != nil {
		return
	}
	p.DisableLayouts()
	return
}

// targetSpaceID returns space id from the root of space object's tree
func (p *SpaceObject) targetSpaceID() (id string, err error) {
	changeInfo := p.Tree().ChangeInfo()
	if changeInfo == nil {
		return "", ErrIncorrectSpaceInfo
	}
	var (
		changePayload = &model.ObjectChangePayload{}
		spaceHeader   = &model.SpaceObjectHeader{}
	)
	err = proto.Unmarshal(changeInfo.ChangePayload, changePayload)
	if err != nil {
		return "", ErrIncorrectSpaceInfo
	}
	err = proto.Unmarshal(changePayload.Data, spaceHeader)
	if err != nil {
		return "", ErrIncorrectSpaceInfo
	}
	return spaceHeader.SpaceID, nil
}
