package restriction

import (
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/uniquekey"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/mock_objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider/mock_typeprovider"
)

type fixture struct {
	Service
	objectStoreMock *mock_objectstore.MockObjectStore
}

func newFixture(t *testing.T) *fixture {
	objectStore := mock_objectstore.NewMockObjectStore(t)
	objectStore.EXPECT().Name().Return("objectstore")

	sbtProvider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
	sbtProvider.EXPECT().Name().Return("sbtProvider")

	a := &app.App{}
	a.Register(objectStore)
	a.Register(sbtProvider)
	s := New()
	err := s.Init(a)
	require.NoError(t, err)
	return &fixture{
		Service:         s,
		objectStoreMock: objectStore,
	}
}

func fakeDerivedID(key string) string {
	return fmt.Sprintf("derivedFrom(%s)", key)
}

func givenObjectType(typeKey bundle.TypeKey) RestrictionHolder {
	return newRestrictionHolder(
		smartblock.SmartBlockTypeObjectType,
		model.ObjectType_objectType,
		uniquekey.MustUniqueKey(model.SmartBlockType_STType, typeKey.String()),
		fakeDerivedID(typeKey.String()),
	)
}

func givenRelation(relationKey bundle.RelationKey) RestrictionHolder {
	return newRestrictionHolder(
		smartblock.SmartBlockTypeRelation,
		model.ObjectType_relation,
		uniquekey.MustUniqueKey(model.SmartBlockType_STRelation, relationKey.String()),
		fakeDerivedID(relationKey.String()),
	)
}
