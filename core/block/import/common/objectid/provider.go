package objectid

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space"
)

type IDProvider interface {
	GetIDAndPayload(ctx context.Context, spaceID string, sn *common.Snapshot, createdTime time.Time, getExisting bool) (string, treestorage.TreeStorageCreatePayload, error)
}

type Provider struct {
	objectStore                objectstore.ObjectStore
	idProviderBySmartBlockType map[sb.SmartBlockType]IDProvider
}

func NewIDProvider(objectStore objectstore.ObjectStore, spaceService space.Service) IDProvider {
	p := &Provider{
		objectStore:                objectStore,
		idProviderBySmartBlockType: make(map[sb.SmartBlockType]IDProvider, 0),
	}
	initializeProviders(objectStore, p, spaceService)
	return p
}

func initializeProviders(objectStore objectstore.ObjectStore, p *Provider, spaceService space.Service) {
	existingObject := newExistingObject(objectStore)
	treeObject := newTreeObject(existingObject, spaceService)
	derivedObject := newDerivedObject(existingObject, spaceService)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeWorkspace] = newWorkspace(spaceService)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeWidget] = newWidget(spaceService)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeRelation] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeObjectType] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeRelationOption] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypePage] = treeObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeProfilePage] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeTemplate] = treeObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeFile] = newSkipObject()
}

func (p *Provider) GetIDAndPayload(ctx context.Context, spaceID string, sn *common.Snapshot, createdTime time.Time, getExisting bool) (string, treestorage.TreeStorageCreatePayload, error) {
	if idProvider, ok := p.idProviderBySmartBlockType[sn.SbType]; ok {
		return idProvider.GetIDAndPayload(ctx, spaceID, sn, createdTime, getExisting)
	}
	return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("unsupported smartblock to import")
}
