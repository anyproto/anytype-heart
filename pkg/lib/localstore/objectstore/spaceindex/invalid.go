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

type invalidStore struct {
	err error
}

func (s *invalidStore) IterateAll(proc func(doc *anyenc.Value) error) error {
	return nil
}

var _ Store = (*invalidStore)(nil)

func NewInvalidStore(err error) Store {
	return &invalidStore{err: err}
}

func (s *invalidStore) ListFullIds() ([]domain.FullID, error) {
	return nil, s.err
}

func (s *invalidStore) SpaceId() string {
	return ""
}

func (s *invalidStore) Close() error {
	return s.err
}

func (s *invalidStore) Init() error {
	return s.err
}

func (s *invalidStore) Query(q database.Query) (records []database.Record, err error) {
	return nil, s.err
}

func (s *invalidStore) QueryRaw(f *database.Filters, limit int, offset int) (records []database.Record, err error) {
	return nil, s.err
}

func (s *invalidStore) QueryByIds(ids []string) (records []database.Record, err error) {
	return nil, s.err
}

func (s *invalidStore) QueryByIdsAndSubscribeForChanges(ids []string, subscription database.Subscription) (records []database.Record, close func(), err error) {
	return nil, nil, s.err
}

func (s *invalidStore) QueryObjectIds(q database.Query) (ids []string, total int, err error) {
	return nil, 0, s.err
}

func (s *invalidStore) QueryIterate(q database.Query, proc func(details *domain.Details)) error {
	return s.err
}

func (s *invalidStore) HasIds(ids []string) (exists []string, err error) {
	return nil, s.err
}

func (s *invalidStore) GetInfosByIds(ids []string) ([]*database.ObjectInfo, error) {
	return nil, s.err
}

func (s *invalidStore) List(includeArchived bool) ([]*database.ObjectInfo, error) {
	return nil, s.err
}

func (s *invalidStore) ListIds() ([]string, error) {
	return nil, s.err
}

func (s *invalidStore) UpdateObjectDetails(ctx context.Context, id string, details *domain.Details) error {
	return s.err
}

func (s *invalidStore) UpdateObjectLinks(ctx context.Context, id string, links []string) error {
	return s.err
}

func (s *invalidStore) UpdateObjectLinksDetailed(ctx context.Context, id string, outgoingLinks []OutgoingLink) error {
	return s.err
}

func (s *invalidStore) UpdatePendingLocalDetails(id string, proc func(details *domain.Details) (*domain.Details, error)) error {
	return s.err
}

func (s *invalidStore) ModifyObjectDetails(id string, proc func(details *domain.Details) (*domain.Details, bool, error)) error {
	return s.err
}

func (s *invalidStore) DeleteObject(id string) error {
	return s.err
}

func (s *invalidStore) DeleteDetails(ctx context.Context, ids []string) error {
	return s.err
}

func (s *invalidStore) DeleteLinks(ids []string) error {
	return s.err
}

func (s *invalidStore) GetDetails(id string) (*domain.Details, error) {
	return nil, s.err
}

func (s *invalidStore) GetObjectByUniqueKey(uniqueKey domain.UniqueKey) (*domain.Details, error) {
	return nil, s.err
}

func (s *invalidStore) GetUniqueKeyById(id string) (key domain.UniqueKey, err error) {
	return nil, s.err
}

func (s *invalidStore) GetInboundLinksById(id string) ([]string, error) {
	return nil, s.err
}

func (s *invalidStore) GetOutboundLinksById(id string) ([]string, error) {
	return nil, s.err
}

func (s *invalidStore) GetOutboundLinksDetailedById(id string) ([]OutgoingLink, error) {
	return nil, s.err
}

func (s *invalidStore) GetWithLinksInfoById(id string) (*model.ObjectInfoWithLinks, error) {
	return nil, s.err
}

func (s *invalidStore) SetActiveView(objectId, blockId, viewId string) error {
	return s.err
}

func (s *invalidStore) SetActiveViews(objectId string, views map[string]string) error {
	return s.err
}

func (s *invalidStore) GetActiveViews(objectId string) (map[string]string, error) {
	return nil, s.err
}

func (s *invalidStore) GetRelationLink(key string) (*model.RelationLink, error) {
	return nil, s.err
}

func (s *invalidStore) FetchRelationByKey(key string) (relation *relationutils.Relation, err error) {
	return nil, s.err
}

func (s *invalidStore) FetchRelationByKeys(keys ...domain.RelationKey) (relations relationutils.Relations, err error) {
	return nil, s.err
}

func (s *invalidStore) FetchRelationByLinks(links pbtypes.RelationLinks) (relations relationutils.Relations, err error) {
	return nil, s.err
}

func (s *invalidStore) ListAllRelations() (relations relationutils.Relations, err error) {
	return nil, s.err
}

func (s *invalidStore) GetRelationById(id string) (relation *model.Relation, err error) {
	return nil, s.err
}

func (s *invalidStore) GetRelationByKey(key string) (*model.Relation, error) {
	return nil, s.err
}

func (s *invalidStore) GetRelationFormatByKey(key domain.RelationKey) (model.RelationFormat, error) {
	return 0, s.err
}

func (s *invalidStore) ListRelationOptions(relationKey domain.RelationKey) (options []*model.RelationOption, err error) {
	return nil, s.err
}

func (s *invalidStore) GetObjectType(id string) (*model.ObjectType, error) {
	return nil, s.err
}

func (s *invalidStore) GetLastIndexedHeadsHash(ctx context.Context, id string) (headsHash string, err error) {
	return "", s.err
}

func (s *invalidStore) SaveLastIndexedHeadsHash(ctx context.Context, id string, headsHash string) (err error) {
	return s.err
}

func (s *invalidStore) WriteTx(ctx context.Context) (anystore.WriteTx, error) {
	return nil, s.err
}

func (s *invalidStore) SubscribeForAll(callback func(rec database.Record)) {

}

func (s *invalidStore) AddFileKeys(fileKeys ...domain.FileEncryptionKeys) error {
	return s.err
}

func (s *invalidStore) GetFileKeys(fileId domain.FileId) (map[string]string, error) {
	return nil, s.err
}
