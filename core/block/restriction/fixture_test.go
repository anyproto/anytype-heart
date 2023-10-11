package restriction

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/system_object/mock_system_object"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/mock_objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/tests/testutil"
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

	systemObjectService := mock_system_object.NewMockService(t)

	a := &app.App{}
	a.Register(objectStore)
	a.Register(sbtProvider)
	a.Register(testutil.PrepareMock(context.Background(), a, systemObjectService))
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

func givenObjectType(typeKey domain.TypeKey) RestrictionHolder {
	return newRestrictionHolder(
		smartblock.SmartBlockTypeObjectType,
		model.ObjectType_objectType,
		domain.MustUniqueKey(smartblock.SmartBlockTypeObjectType, typeKey.String()),
		fakeDerivedID(typeKey.String()),
	)
}

func givenRelation(relationKey domain.RelationKey) RestrictionHolder {
	return newRestrictionHolder(
		smartblock.SmartBlockTypeRelation,
		model.ObjectType_relation,
		domain.MustUniqueKey(smartblock.SmartBlockTypeRelation, relationKey.String()),
		fakeDerivedID(relationKey.String()),
	)
}
