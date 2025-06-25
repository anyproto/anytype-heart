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
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
)

var log = logging.Logger("import").Desugar()

type IdAndKeyProvider interface {
	IDProvider
	InternalKeyProvider
}

type IDProvider interface {
	GetIDAndPayload(ctx context.Context, spaceID string, sn *common.Snapshot, createdTime time.Time, getExisting bool, origin objectorigin.ObjectOrigin) (string, treestorage.TreeStorageCreatePayload, error)
}

type InternalKeyProvider interface {
	GetInternalKey(sbType sb.SmartBlockType) string
}

type Provider struct {
	idProviderBySmartBlockType map[sb.SmartBlockType]IDProvider
}

func NewIDProvider(
	objectStore objectstore.ObjectStore,
	spaceService space.Service,
	blockService *block.Service,
	fileObjectService fileobject.Service,
) IdAndKeyProvider {
	p := &Provider{
		idProviderBySmartBlockType: make(map[sb.SmartBlockType]IDProvider, 0),
	}
	existingObject := newExistingObject(objectStore)
	treeObject := newTreeObject(existingObject, spaceService)
	derivedObject := newDerivedObject(existingObject, spaceService, objectStore)
	fileObject := &fileObject{
		treeObject:   treeObject,
		blockService: blockService,
	}
	oldFile := &oldFile{
		blockService:      blockService,
		objectStore:       objectStore,
		fileObjectService: fileObjectService,
	}
	p.idProviderBySmartBlockType[sb.SmartBlockTypeWorkspace] = newWorkspace(spaceService)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeWidget] = newWidget(spaceService)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeRelation] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeObjectType] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeRelationOption] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypePage] = treeObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeFileObject] = fileObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeFile] = oldFile
	p.idProviderBySmartBlockType[sb.SmartBlockTypeProfilePage] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeTemplate] = treeObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeParticipant] = newParticipant()
	return p
}

func (p *Provider) GetIDAndPayload(
	ctx context.Context,
	spaceID string,
	sn *common.Snapshot,
	createdTime time.Time,
	getExisting bool,
	origin objectorigin.ObjectOrigin,
) (string, treestorage.TreeStorageCreatePayload, error) {
	if idProvider, ok := p.idProviderBySmartBlockType[sn.Snapshot.SbType]; ok {
		return idProvider.GetIDAndPayload(ctx, spaceID, sn, createdTime, getExisting, origin)
	}
	return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("unsupported smartblock to import")
}

func (p *Provider) GetInternalKey(sbType sb.SmartBlockType) string {
	if idProvider, ok := p.idProviderBySmartBlockType[sbType]; ok {
		if internalKeyProvider, ok := idProvider.(InternalKeyProvider); ok {
			return internalKeyProvider.GetInternalKey(sbType)
		}
	}
	return ""
}
