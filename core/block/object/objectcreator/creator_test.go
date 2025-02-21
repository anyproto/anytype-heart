package objectcreator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/template"
	"github.com/anyproto/anytype-heart/core/block/template/mock_template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/util/dateutil"
)

const spaceId = "spc1"

type fixture struct {
	spaceService    *mock_space.MockService
	spc             *mock_clientspace.MockSpace
	templateService *mock_template.MockService
	objectStore     *objectstore.StoreFixture
	service         Service
}

func newFixture(t *testing.T) *fixture {
	spaceService := mock_space.NewMockService(t)
	spc := mock_clientspace.NewMockSpace(t)

	templateSvc := mock_template.NewMockService(t)
	store := objectstore.NewStoreFixture(t)

	s := &service{
		spaceService:    spaceService,
		templateService: templateSvc,
		objectStore:     store,
	}

	return &fixture{
		spaceService:    spaceService,
		spc:             spc,
		templateService: templateSvc,
		objectStore:     store,
		service:         s,
	}
}

func TestService_CreateObject(t *testing.T) {
	t.Run("template creation", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		f := newFixture(t)
		f.spaceService.EXPECT().Get(mock.Anything, mock.Anything).Return(f.spc, nil)
		f.spc.EXPECT().CreateTreeObject(mock.Anything, mock.Anything).Return(sb, nil)
		f.spc.EXPECT().Id().Return(spaceId)
		f.spc.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return key.Marshal(), nil
		})

		f.templateService.EXPECT().CreateTemplateStateWithDetails(mock.Anything).RunAndReturn(func(req template.CreateTemplateRequest) (*state.State, error) {
			st := state.NewDoc(req.TemplateId, nil).NewState()
			st.SetDetails(req.Details)
			return st, nil
		})
		f.templateService.EXPECT().SetDefaultTemplateInType(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, typeId string, templateId string) error {
			assert.Equal(t, "test", templateId)
			assert.Equal(t, bundle.TypeKeyTask.URL(), typeId)
			return nil
		})

		f.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:                domain.String(bundle.TypeKeyTask.URL()),
				bundle.RelationKeySpaceId:           domain.String(spaceId),
				bundle.RelationKeyUniqueKey:         domain.String(bundle.TypeKeyTask.URL()),
				bundle.RelationKeyRecommendedLayout: domain.Int64(model.ObjectType_todo),
			},
		})

		// when
		id, _, err := f.service.CreateObject(context.Background(), spaceId, CreateObjectRequest{
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyTargetObjectType: domain.String(bundle.TypeKeyTask.URL()),
			}),
			ObjectTypeKey: bundle.TypeKeyTemplate,
		})

		// then
		assert.NoError(t, err)
		assert.Equal(t, "test", id)
	})

	t.Run("template creation - no target type", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.spaceService.EXPECT().Get(mock.Anything, mock.Anything).Return(f.spc, nil)

		// when
		_, _, err := f.service.CreateObject(context.Background(), spaceId, CreateObjectRequest{
			ObjectTypeKey: bundle.TypeKeyTemplate,
		})

		// then
		assert.Error(t, err)
	})

	t.Run("date object creation", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.spaceService.EXPECT().Get(mock.Anything, mock.Anything).Return(f.spc, nil)
		f.spc.EXPECT().Id().Return(spaceId)
		f.spc.EXPECT().GetTypeIdByKey(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, key domain.TypeKey) (string, error) {
			assert.Equal(t, bundle.TypeKeyDate, key)
			return bundle.TypeKeyDate.URL(), nil
		})
		ts := time.Now()
		dateObject := dateutil.NewDateObject(ts, false)

		// when
		id, details, err := f.service.CreateObject(context.Background(), spaceId, CreateObjectRequest{
			ObjectTypeKey: bundle.TypeKeyDate,
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyTimestamp: domain.Int64(dateObject.Time().Unix()),
			}),
		})

		// then
		assert.NoError(t, err)
		assert.True(t, strings.HasPrefix(id, dateObject.Id()))
		assert.Equal(t, spaceId, details.GetString(bundle.RelationKeySpaceId))
		assert.Equal(t, bundle.TypeKeyDate.URL(), details.GetString(bundle.RelationKeyType))
	})
}
