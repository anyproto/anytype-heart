package spaceindex

import (
	"context"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// emptyStore is used for uninitialized spaces (as sharedEmptyStore singleton).
// Read methods return empty results, write methods return ErrSpaceNotInitialized.
// SpaceId is handled by storeProxy, not emptyStore.
type emptyStore struct{}

func (s *emptyStore) SpaceId() string {
	return "" // SpaceId is handled by storeProxy
}

func (s *emptyStore) Close() error {
	return nil
}

func (s *emptyStore) Init() error {
	return ErrSpaceNotInitialized
}

func (s *emptyStore) Remove() error {
	return nil
}

// Read methods - return empty results

func (s *emptyStore) Query(q database.Query) (records []database.Record, err error) {
	return []database.Record{}, nil
}

func (s *emptyStore) QueryRaw(f *database.Filters, limit int, offset int) (records []database.Record, err error) {
	return []database.Record{}, nil
}

func (s *emptyStore) QueryByIds(ids []string) (records []database.Record, err error) {
	return []database.Record{}, nil
}

func (s *emptyStore) QueryByIdsAndSubscribeForChanges(ids []string, subscription database.Subscription) (records []database.Record, close func(), err error) {
	return []database.Record{}, func() {}, nil
}

func (s *emptyStore) QueryObjectIds(q database.Query) (ids []string, total int, err error) {
	return []string{}, 0, nil
}

func (s *emptyStore) QueryIterate(q database.Query, proc func(details *domain.Details)) error {
	return nil
}

func (s *emptyStore) IterateAll(proc func(doc *anyenc.Value) error) error {
	return nil
}

func (s *emptyStore) HasIds(ids []string) (exists []string, err error) {
	return []string{}, nil
}

func (s *emptyStore) GetInfosByIds(ids []string) ([]*database.ObjectInfo, error) {
	return []*database.ObjectInfo{}, nil
}

func (s *emptyStore) List(includeArchived bool) ([]*database.ObjectInfo, error) {
	return []*database.ObjectInfo{}, nil
}

func (s *emptyStore) ListIds() ([]string, error) {
	return []string{}, nil
}

func (s *emptyStore) ListFullIds() ([]domain.FullID, error) {
	return []domain.FullID{}, nil
}

func (s *emptyStore) GetDetails(id string) (*domain.Details, error) {
	return domain.NewDetails(), nil
}

func (s *emptyStore) GetObjectByUniqueKey(uniqueKey domain.UniqueKey) (*domain.Details, error) {
	return nil, ErrObjectNotFound
}

func (s *emptyStore) GetUniqueKeyById(id string) (key domain.UniqueKey, err error) {
	return nil, ErrObjectNotFound
}

func (s *emptyStore) GetInboundLinksById(id string) ([]string, error) {
	return []string{}, nil
}

func (s *emptyStore) GetOutboundLinksById(id string) ([]string, error) {
	return []string{}, nil
}

func (s *emptyStore) GetWithLinksInfoById(id string) (*model.ObjectInfoWithLinks, error) {
	return nil, ErrObjectNotFound
}

func (s *emptyStore) GetActiveViews(objectId string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (s *emptyStore) GetRelationLink(key string) (*model.RelationLink, error) {
	return nil, ErrObjectNotFound
}

func (s *emptyStore) FetchRelationByKey(key string) (relation *relationutils.Relation, err error) {
	return nil, ErrObjectNotFound
}

func (s *emptyStore) FetchRelationByKeys(keys ...domain.RelationKey) (relations relationutils.Relations, err error) {
	return relationutils.Relations{}, nil
}

func (s *emptyStore) FetchRelationByLinks(links pbtypes.RelationLinks) (relations relationutils.Relations, err error) {
	return relationutils.Relations{}, nil
}

func (s *emptyStore) ListAllRelations() (relations relationutils.Relations, err error) {
	return relationutils.Relations{}, nil
}

func (s *emptyStore) GetRelationById(id string) (relation *model.Relation, err error) {
	return nil, ErrObjectNotFound
}

func (s *emptyStore) GetRelationByKey(key string) (*model.Relation, error) {
	return nil, ErrObjectNotFound
}

func (s *emptyStore) GetRelationFormatByKey(key domain.RelationKey) (model.RelationFormat, error) {
	return 0, ErrObjectNotFound
}

func (s *emptyStore) ListRelationOptions(relationKey domain.RelationKey) (options []*model.RelationOption, err error) {
	return []*model.RelationOption{}, nil
}

func (s *emptyStore) GetObjectType(id string) (*model.ObjectType, error) {
	return nil, ErrObjectNotFound
}

func (s *emptyStore) GetLastIndexedHeadsHash(ctx context.Context, id string) (headsHash string, err error) {
	return "", nil
}

// Write methods - return error

func (s *emptyStore) UpdateObjectDetails(ctx context.Context, id string, details *domain.Details) error {
	return ErrSpaceNotInitialized
}

func (s *emptyStore) UpdateObjectLinks(ctx context.Context, id string, links []string) error {
	return ErrSpaceNotInitialized
}

func (s *emptyStore) UpdatePendingLocalDetails(id string, proc func(details *domain.Details) (*domain.Details, error)) error {
	return ErrSpaceNotInitialized
}

func (s *emptyStore) ModifyObjectDetails(id string, proc func(details *domain.Details) (*domain.Details, bool, error)) error {
	return ErrSpaceNotInitialized
}

func (s *emptyStore) DeleteObject(id string) error {
	return ErrSpaceNotInitialized
}

func (s *emptyStore) DeleteDetails(ctx context.Context, ids []string) error {
	return ErrSpaceNotInitialized
}

func (s *emptyStore) DeleteLinks(ids []string) error {
	return ErrSpaceNotInitialized
}

func (s *emptyStore) SetActiveView(objectId, blockId, viewId string) error {
	return ErrSpaceNotInitialized
}

func (s *emptyStore) SetActiveViews(objectId string, views map[string]string) error {
	return ErrSpaceNotInitialized
}

func (s *emptyStore) SaveLastIndexedHeadsHash(ctx context.Context, id string, headsHash string) (err error) {
	return ErrSpaceNotInitialized
}

func (s *emptyStore) WriteTx(ctx context.Context) (anystore.WriteTx, error) {
	return nil, ErrSpaceNotInitialized
}

func (s *emptyStore) SubscribeForAll(callback func(rec database.Record)) {
	// No-op for empty store
}

func (s *emptyStore) AddFileKeys(fileKeys ...domain.FileEncryptionKeys) error {
	return ErrSpaceNotInitialized
}

func (s *emptyStore) GetFileKeys(fileId domain.FileId) (map[string]string, error) {
	return nil, ErrObjectNotFound
}
