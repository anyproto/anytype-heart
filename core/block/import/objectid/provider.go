package objectid

import (
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

type IDProvider interface {
	GetID(spaceID string, sn *converter.Snapshot, createdTime time.Time, getExisting bool) (string, treestorage.TreeStorageCreatePayload, error)
}

type Provider struct {
	objectStore                objectstore.ObjectStore
	core                       core.Service
	service                    *block.Service
	idProviderBySmartBlockType map[sb.SmartBlockType]IDProvider
}

func NewIDProvider(objectStore objectstore.ObjectStore, core core.Service, service *block.Service) IDProvider {
	p := &Provider{
		objectStore: objectStore,
		core:        core,
		service:     service,
	}
	initializeProviders(objectStore, core, service, p)
	return p
}

func initializeProviders(objectStore objectstore.ObjectStore, core core.Service, service *block.Service, p *Provider) {
	existingObject := NewExistingObject(objectStore)
	treeObject := NewTreeObject(existingObject, service)
	derivedObject := NewDerivedObject(existingObject, objectStore, service)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeWorkspace] = NewWorkspace(core)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeWidget] = NewWidget(core)
	p.idProviderBySmartBlockType[sb.SmartBlockTypeRelation] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeObjectType] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeRelationOption] = derivedObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypePage] = treeObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeProfilePage] = treeObject
	p.idProviderBySmartBlockType[sb.SmartBlockTypeTemplate] = treeObject
}

func (p *Provider) GetID(spaceId string, sn *converter.Snapshot, createdTime time.Time, getExisting bool,
) (string, treestorage.TreeStorageCreatePayload, error) {
	if idProvider, ok := p.idProviderBySmartBlockType[sn.SbType]; ok {
		return idProvider.GetID(spaceId, sn, createdTime, getExisting)
	}
	return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("unsupported smartblock to import")
}
