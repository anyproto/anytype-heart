package restriction

import (
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/system_object/mock_system_object"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/mock_objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	Service
	objectStoreMock         *mock_objectstore.MockObjectStore
	systemObjectServiceMock *mock_system_object.MockService
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
	a.Register(testutil.PrepareMock(a, systemObjectService))
	s := New()
	err := s.Init(a)
	require.NoError(t, err)
	return &fixture{
		Service:                 s,
		objectStoreMock:         objectStore,
		systemObjectServiceMock: systemObjectService,
	}
}

func fakeDerivedID(key string) string {
	return fmt.Sprintf("derivedFrom(%s)", key)
}

func givenObjectType(typeKey bundle.TypeKey) RestrictionHolder {
	return newRestrictionHolder(
		smartblock.SmartBlockTypeObjectType,
		model.ObjectType_objectType,
		domain.MustUniqueKey(smartblock.SmartBlockTypeObjectType, typeKey.String()),
		fakeDerivedID(typeKey.String()),
	)
}

func givenRelation(relationKey bundle.RelationKey) RestrictionHolder {
	return newRestrictionHolder(
		smartblock.SmartBlockTypeRelation,
		model.ObjectType_relation,
		domain.MustUniqueKey(smartblock.SmartBlockTypeRelation, relationKey.String()),
		fakeDerivedID(relationKey.String()),
	)
}
