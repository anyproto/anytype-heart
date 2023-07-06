package editor

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/testMock"
)

func NewTemplateTest(ctrl *gomock.Controller, templateName string) (*Template, error) {
	sb := smarttest.New("root")
	_ = sb.SetDetails(nil, []*pb.RpcObjectSetDetailsDetail{&pb.RpcObjectSetDetailsDetail{
		Key:   bundle.RelationKeyName.String(),
		Value: pbtypes.String(templateName),
	}}, false)
	objectStore := testMock.NewMockObjectStore(ctrl)
	objectStore.EXPECT().GetObjectTypes(gomock.Any()).AnyTimes()
	t := &Template{
		Page: &Page{
			SmartBlock:  sb,
			objectStore: objectStore,
		},
	}
	initCtx := &smartblock.InitContext{IsNewObject: true}
	if err := t.Init(initCtx); err != nil {
		return nil, err
	}
	migration.RunMigrations(t, initCtx)
	if err := t.Apply(initCtx.State); err != nil {
		return nil, err
	}
	return t, nil
}

func TestTemplate_GetNewPageState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	templateName := "template"

	t.Run("empty page name", func(t *testing.T) {
		tmpl, err := NewTemplateTest(ctrl, templateName)
		require.NoError(t, err)

		st, err := tmpl.GetNewPageState("")
		require.NoError(t, err)
		require.Equal(t, st.Details().Fields[bundle.RelationKeyName.String()].GetStringValue(), templateName)
		require.Equal(t, st.Get(template.TitleBlockId).Model().GetText().Text, templateName)
	})

	t.Run("custom page name", func(t *testing.T) {
		tmpl, err := NewTemplateTest(ctrl, templateName)
		require.NoError(t, err)

		customName := "some name"
		st, err := tmpl.GetNewPageState(customName)
		require.NoError(t, err)
		require.Equal(t, st.Details().Fields[bundle.RelationKeyName.String()].GetStringValue(), customName)
		require.Equal(t, st.Get(template.TitleBlockId).Model().GetText().Text, "")
	})
}
