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

type IdGetter interface {
	GetID(spaceId string, sn *converter.Snapshot, createdTime time.Time, getExisting bool) (string, treestorage.TreeStorageCreatePayload, error)
}

type IdGetterProvider interface {
	ProvideIdGetter(smartBlock sb.SmartBlockType) (IdGetter, error)
}

type Provider struct {
	idProviderBySmartBlockType map[sb.SmartBlockType]IdGetter
}

func NewProvider(objectStore objectstore.ObjectStore, core core.Service, service *block.Service) *Provider {
	p := &Provider{idProviderBySmartBlockType: make(map[sb.SmartBlockType]IdGetter, 0)}
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

func (p *Provider) ProvideIdGetter(smartBlock sb.SmartBlockType) (IdGetter, error) {
	if idGetter, ok := p.idProviderBySmartBlockType[smartBlock]; ok {
		return idGetter, nil
	}
	return nil, fmt.Errorf("failed to get id provider, unsupported smartblock type to import: %s", smartBlock.String())
}
