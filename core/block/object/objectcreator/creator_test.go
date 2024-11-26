package objectcreator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
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
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

func (tts *testTemplateService) CreateTemplateStateWithDetails(templateId string, details *types.Struct) (*state.State, error) {
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
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyTargetObjectType.String(): pbtypes.String(bundle.TypeKeyTask.URL()),
			}},
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
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyTimestamp.String(): pbtypes.Int64(dateObject.Time().Unix()),
			}},
		})

		// then
		assert.NoError(t, err)
		assert.True(t, strings.HasPrefix(id, dateObject.Id()))
		assert.Equal(t, spaceId, pbtypes.GetString(details, bundle.RelationKeySpaceId.String()))
		assert.Equal(t, bundle.TypeKeyDate.URL(), pbtypes.GetString(details, bundle.RelationKeyType.String()))
	})
}
