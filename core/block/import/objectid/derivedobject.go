package objectid

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type DerivedObject struct {
	existingObject *ExistingObject
	objectStore    objectstore.ObjectStore
	service        *block.Service
}

func NewDerivedObject(existingObject *ExistingObject, objectStore objectstore.ObjectStore, service *block.Service) *DerivedObject {
	return &DerivedObject{existingObject: existingObject, objectStore: objectStore, service: service}
}

func (r *DerivedObject) GetID(spaceID string, sn *converter.Snapshot, createTime time.Time, getExisting bool) (string, treestorage.TreeStorageCreatePayload, error) {
	id, payload, err := r.existingObject.GetID(spaceID, sn, createTime, getExisting)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, err
	}
	if id != "" {
		return id, payload, nil
	}
	id = pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyId.String())
	uk, err := domain.UnmarshalUniqueKey(id)
	if err != nil {
		id = pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyUniqueKey.String())
		uk, err = domain.UnmarshalUniqueKey(id)
		if err != nil {
			return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("get unique key: %w", err)
		}
	}
	payload, err = r.service.DeriveTreeCreatePayload(context.Background(), spaceID, uk)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("derive tree create payload: %w", err)
	}
	return payload.RootRawChange.Id, payload, nil
}
