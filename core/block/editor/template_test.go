package editor

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/system_object/mock_system_object"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/testMock"
)

func NewTemplateTest(t *testing.T, ctrl *gomock.Controller, templateName string) (*Template, error) {
	sb := smarttest.New("root")
	_ = sb.SetDetails(nil, []*pb.RpcObjectSetDetailsDetail{&pb.RpcObjectSetDetailsDetail{
		Key:   bundle.RelationKeyName.String(),
		Value: pbtypes.String(templateName),
	}}, false)

	objectStore := testMock.NewMockObjectStore(ctrl)

	systemObjectService := mock_system_object.NewMockService(t)
	systemObjectService.EXPECT().GetObjectTypes(mock.Anything).Return(nil, nil).Maybe()
	templ := &Template{
		Page: &Page{
			SmartBlock:          sb,
			objectStore:         objectStore,
			systemObjectService: systemObjectService,
		},
	}
	uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, bundle.TypeKeyPage.String())
	require.NoError(t, err)

	systemObjectService.EXPECT().GetObjectByUniqueKey(mock.Anything, uniqueKey).Return(&model.ObjectDetails{
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyRecommendedLayout.String(): pbtypes.Int64(int64(model.ObjectType_basic)),
			},
		},
	}, nil)
	initCtx := &smartblock.InitContext{IsNewObject: true}
	if err := templ.Init(initCtx); err != nil {
		return nil, err
	}
	migration.RunMigrations(templ, initCtx)
	if err := templ.Apply(initCtx.State); err != nil {
		return nil, err
	}
	return templ, nil
}

func TestTemplate_GetNewPageState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	templateName := "template"

	t.Run("empty page name", func(t *testing.T) {
		tmpl, err := NewTemplateTest(t, ctrl, templateName)
		require.NoError(t, err)

		st, err := tmpl.GetNewPageState("")
		require.NoError(t, err)
		require.Equal(t, st.Details().Fields[bundle.RelationKeyName.String()].GetStringValue(), templateName)
		require.Equal(t, st.Get(template.TitleBlockId).Model().GetText().Text, templateName)
	})

	t.Run("custom page name", func(t *testing.T) {
		tmpl, err := NewTemplateTest(t, ctrl, templateName)
		require.NoError(t, err)

		customName := "some name"
		st, err := tmpl.GetNewPageState(customName)
		require.NoError(t, err)
		require.Equal(t, st.Details().Fields[bundle.RelationKeyName.String()].GetStringValue(), customName)
		require.Equal(t, st.Get(template.TitleBlockId).Model().GetText().Text, "")
	})
}
