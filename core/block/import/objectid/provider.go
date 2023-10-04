package objectid

import (
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

type IDProvider interface {
	GetID(spaceID string, sn *converter.Snapshot, createdTime time.Time, getExisting bool) (string, treestorage.TreeStorageCreatePayload, error)
}

type Provider struct {
	objectStore                objectstore.ObjectStore
	objectCache                objectcache.Cache
	core                       core.Service
	idProviderBySmartBlockType map[sb.SmartBlockType]IDProvider
}

func NewIDProvider(objectStore objectstore.ObjectStore,
	objectCache objectcache.Cache,
	core core.Service) IDProvider {
	p := &Provider{
		objectStore:                objectStore,
		objectCache:                objectCache,
		core:                       core,
		idProviderBySmartBlockType: make(map[sb.SmartBlockType]IDProvider, 0),
	}
	initializeProviders(objectStore, objectCache, p, core)
	return p
}

func initializeProviders(objectStore objectstore.ObjectStore, cache objectcache.Cache, p *Provider, core core.Service) {
	existingObject := NewExistingObject(objectStore)
	treeObject := NewTreeObject(existingObject, cache)
	derivedObject := NewDerivedObject(existingObject, objectStore, cache)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeWorkspace] = NewWorkspace(core)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeWidget] = NewWidget(core)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeRelation] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeObjectType] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeRelationOption] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypePage] = treeObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeProfilePage] = treeObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeTemplate] = treeObject
}

func (p *Provider) GetID(spaceID string, sn *converter.Snapshot, createdTime time.Time, getExisting bool,
) (string, treestorage.TreeStorageCreatePayload, error) {
	if idProvider, ok := p.idProviderBySmartBlockType[sn.SbType]; ok {
		return idProvider.GetID(spaceID, sn, createdTime, getExisting)
	}
	return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("unsupported smartblock to import")
}
