package objectcreator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/editor/lastused/mock_lastused"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/util/dateutil"
)

const spaceId = "spc1"

type fixture struct {
	spaceService    *mock_space.MockService
	spc             *mock_clientspace.MockSpace
	templateService *testTemplateService
	lastUsedService *mock_lastused.MockObjectUsageUpdater
	service         Service
}

func newFixture(t *testing.T) *fixture {
	spaceService := mock_space.NewMockService(t)
	spc := mock_clientspace.NewMockSpace(t)

	templateSvc := &testTemplateService{}
	lastUsedSvc := mock_lastused.NewMockObjectUsageUpdater(t)

	s := &service{
		spaceService:    spaceService,
		templateService: templateSvc,
		lastUsedUpdater: lastUsedSvc,
	}

	return &fixture{
		spaceService:    spaceService,
		spc:             spc,
		templateService: templateSvc,
		lastUsedService: lastUsedSvc,
		service:         s,
	}
}

type testTemplateService struct {
	templates map[string]*state.State
}

func (tts *testTemplateService) CreateTemplateStateWithDetails(templateId string, details *domain.Details) (*state.State, error) {
	if tts.templates != nil {
		if st, found := tts.templates[templateId]; found {
			return st, nil
		}
	}
	return state.NewDoc(templateId, nil).NewState(), nil
}

func (tts *testTemplateService) TemplateCloneInSpace(space clientspace.Space, id string) (templateId string, err error) {
	return "", nil
}

func TestService_CreateObject(t *testing.T) {
	t.Run("template creation", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		f := newFixture(t)
		f.spaceService.EXPECT().Get(mock.Anything, mock.Anything).Return(f.spc, nil)
		f.spc.EXPECT().CreateTreeObject(mock.Anything, mock.Anything).Return(sb, nil)
		f.spc.EXPECT().Id().Return(spaceId)
		f.lastUsedService.EXPECT().UpdateLastUsedDate(spaceId, bundle.TypeKeyTemplate, mock.Anything).Return()

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
		// TODO: GO-4494 - Remove links relation id fetch
		f.spc.EXPECT().GetRelationIdByKey(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, key domain.RelationKey) (string, error) {
			assert.Equal(t, bundle.RelationKeyLinks, key)
			return bundle.RelationKeyLinks.URL(), nil
		})
		ts := time.Now()
		name := dateutil.TimeToDateName(ts)

		// when
		id, details, err := f.service.CreateObject(context.Background(), spaceId, CreateObjectRequest{
			ObjectTypeKey: bundle.TypeKeyDate,
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String(name),
			}),
		})

		// then
		assert.NoError(t, err)
		assert.True(t, strings.HasPrefix(id, dateutil.TimeToDateId(ts)))
		assert.Equal(t, spaceId, details.GetString(bundle.RelationKeySpaceId))
		assert.Equal(t, bundle.TypeKeyDate.URL(), details.GetString(bundle.RelationKeyType))
	})

	t.Run("date object creation - invalid name", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.spaceService.EXPECT().Get(mock.Anything, mock.Anything).Return(f.spc, nil)
		ts := time.Now()
		name := ts.Format(time.RFC3339)

		// when
		_, _, err := f.service.CreateObject(context.Background(), spaceId, CreateObjectRequest{
			ObjectTypeKey: bundle.TypeKeyDate,
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String(name),
			}),
		})

		// then
		assert.Error(t, err)
	})
}
