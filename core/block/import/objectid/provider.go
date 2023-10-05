package objectid

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space"
)

type IDProvider interface {
	GetIDAndPayload(ctx context.Context, spaceID string, sn *converter.Snapshot, createdTime time.Time, getExisting bool) (string, treestorage.TreeStorageCreatePayload, error)
}

type Provider struct {
	objectStore                objectstore.ObjectStore
	objectCache                objectcache.Cache
	spaceService               space.SpaceService
	idProviderBySmartBlockType map[sb.SmartBlockType]IDProvider
}

func NewIDProvider(objectStore objectstore.ObjectStore, objectCache objectcache.Cache, spaceService space.SpaceService) IDProvider {
	p := &Provider{
		objectStore:                objectStore,
		objectCache:                objectCache,
		spaceService:               spaceService,
		idProviderBySmartBlockType: make(map[sb.SmartBlockType]IDProvider, 0),
	}
	initializeProviders(objectStore, objectCache, p, spaceService)
	return p
}

func initializeProviders(objectStore objectstore.ObjectStore, cache objectcache.Cache, p *Provider, core space.SpaceService) {
	existingObject := newExistingObject(objectStore)
	treeObject := newTreeObject(existingObject, cache)
	derivedObject := newDerivedObject(existingObject, objectStore, cache)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeWorkspace] = newWorkspace(core)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeWidget] = newWidget(core)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeRelation] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeObjectType] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeRelationOption] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypePage] = treeObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeProfilePage] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeTemplate] = treeObject
}

func (p *Provider) GetIDAndPayload(ctx context.Context, spaceID string, sn *converter.Snapshot, createdTime time.Time, getExisting bool) (string, treestorage.TreeStorageCreatePayload, error) {
	if idProvider, ok := p.idProviderBySmartBlockType[sn.SbType]; ok {
		return idProvider.GetIDAndPayload(ctx, spaceID, sn, createdTime, getExisting)
	}
	return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("unsupported smartblock to import")
}
