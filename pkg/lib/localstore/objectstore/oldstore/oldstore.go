package oldstore

import (
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/dgraph-io/badger/v4"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

var pagesDetailsBase = ds.NewKey("/pages/details")

const CName = "objectstore.oldstore"

type Service interface {
	app.Component

	// SetDetails is for testing purposes only
	SetDetails(objectId string, details *types.Struct) error
	GetLocalDetails(objectId string) (*types.Struct, error)
	DeleteDetails(objectId string) error
}

type service struct {
	db *badger.DB
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) error {
	datastoreService := a.MustComponent(datastore.CName).(datastore.Datastore)
	var err error
	s.db, err = datastoreService.LocalStorage()
	if err != nil {
		return fmt.Errorf("get badger: %w", err)
	}
	return nil
}

func (s *service) Name() string { return CName }

func (s *service) SetDetails(objectId string, details *types.Struct) error {
	return s.db.Update(func(txn *badger.Txn) error {
		val, err := proto.Marshal(&model.ObjectDetails{Details: details})
		if err != nil {
			return fmt.Errorf("marshal details: %w", err)
		}
		return txn.Set(pagesDetailsBase.ChildString(objectId).Bytes(), val)
	})
}

func (s *service) GetLocalDetails(objectId string) (*types.Struct, error) {
	var objDetails *model.ObjectDetails
	err := s.db.View(func(txn *badger.Txn) error {
		it, err := txn.Get(pagesDetailsBase.ChildString(objectId).Bytes())
		if err != nil {
			return fmt.Errorf("get details: %w", err)
		}
		objDetails, err = s.unmarshalDetailsFromItem(it)
		if err != nil {
			return fmt.Errorf("unmarshal details: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	details := pbtypes.StructFilterKeys(objDetails.Details, slice.IntoStrings(bundle.LocalRelationsKeys))
	return details, nil
}

func (s *service) unmarshalDetailsFromItem(it *badger.Item) (*model.ObjectDetails, error) {
	var details *model.ObjectDetails
	err := it.Value(func(val []byte) error {
		var err error
		details, err = unmarshalDetails(val)
		if err != nil {
			return fmt.Errorf("unmarshal details: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get item value: %w", err)
	}
	return details, nil
}

func unmarshalDetails(rawValue []byte) (*model.ObjectDetails, error) {
	result := &model.ObjectDetails{}
	if err := proto.Unmarshal(rawValue, result); err != nil {
		return nil, err
	}
	if result.Details == nil {
		result.Details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	if result.Details.Fields == nil {
		result.Details.Fields = map[string]*types.Value{}
	} else {
		pbtypes.StructDeleteEmptyFields(result.Details)
	}
	return result, nil
}

func (s *service) DeleteDetails(objectId string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(pagesDetailsBase.ChildString(objectId).Bytes())
	})
}
