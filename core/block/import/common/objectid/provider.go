package objectid

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space"
)

type IDProvider interface {
	GetIDAndPayload(ctx context.Context, spaceID string, sn *common.Snapshot, createdTime time.Time, getExisting bool, origin objectorigin.ObjectOrigin) (string, treestorage.TreeStorageCreatePayload, error)
}

type Provider struct {
	objectStore                objectstore.ObjectStore
	idProviderBySmartBlockType map[sb.SmartBlockType]IDProvider
}

func NewIDProvider(
	objectStore objectstore.ObjectStore,
	spaceService space.Service,
	blockService *block.Service,
	fileStore filestore.FileStore,
	fileObjectService fileobject.Service,
) IDProvider {
	p := &Provider{
		objectStore:                objectStore,
		idProviderBySmartBlockType: make(map[sb.SmartBlockType]IDProvider, 0),
	}
	existingObject := newExistingObject(objectStore)
	treeObject := newTreeObject(existingObject, spaceService)
	derivedObject := newDerivedObject(existingObject, spaceService)
	oldFile := &oldFile{
		blockService:      blockService,
		fileStore:         fileStore,
		fileObjectService: fileObjectService,
	}
	p.idProviderBySmartBlockType[sb.SmartBlockTypeWorkspace] = newWorkspace(spaceService)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeWidget] = newWidget(spaceService)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeRelation] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeObjectType] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeRelationOption] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypePage] = treeObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeFileObject] = treeObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeFile] = oldFile
	p.idProviderBySmartBlockType[sb.SmartBlockTypeProfilePage] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeTemplate] = treeObject
	return p
}

func (p *Provider) GetIDAndPayload(ctx context.Context, spaceID string, sn *common.Snapshot, createdTime time.Time, getExisting bool, origin objectorigin.ObjectOrigin) (string, treestorage.TreeStorageCreatePayload, error) {
	if idProvider, ok := p.idProviderBySmartBlockType[sn.SbType]; ok {
		return idProvider.GetIDAndPayload(ctx, spaceID, sn, createdTime, getExisting, origin)
	}
	return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("unsupported smartblock to import")
}
