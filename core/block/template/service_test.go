package template

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type testPicker struct {
	sb smartblock.SmartBlock
}

func (t *testPicker) GetObject(ctx context.Context, id string) (sb smartblock.SmartBlock, err error) {
	return t.sb, nil
}

func (t *testPicker) GetObjectByFullID(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
	return t.sb, nil
}

func (t *testPicker) Init(a *app.App) error { return nil }

func (t *testPicker) Name() string { return "" }

func NewTemplateTest(templateName, typeKey string) *editor.Template {
	sb := smarttest.New(templateName)
	_ = sb.SetDetails(nil, []*pb.RpcObjectSetDetailsDetail{{
		Key:   bundle.RelationKeyName.String(),
		Value: pbtypes.String(templateName),
	}}, false)
	sb.AddBlock(simple.New(&model.Block{Id: templateName, ChildrenIds: []string{template.TitleBlockId}}))
	sb.AddBlock(simple.New(&model.Block{Id: template.TitleBlockId, Content: &model.BlockContentOfText{
		Text: &model.BlockContentText{
			Text: templateName,
		},
	}}))

	return &editor.Template{
		Page: &editor.Page{
			SmartBlock: sb,
		},
	}
}

func TestService_StateFromTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	templateName := "template"

	t.Run("empty page name", func(t *testing.T) {
		// given
		tmpl := NewTemplateTest(templateName, "")
		s := service{picker: &testPicker{sb: tmpl}}

		// when
		st, err := s.StateFromTemplate(templateName, "")

		// then
		assert.NoError(t, err)
		assert.Equal(t, st.Details().Fields[bundle.RelationKeyName.String()].GetStringValue(), templateName)
		assert.Equal(t, st.Get(template.TitleBlockId).Model().GetText().Text, templateName)
	})

	t.Run("custom page name", func(t *testing.T) {
		// given
		tmpl := NewTemplateTest(templateName, "")
		s := service{picker: &testPicker{sb: tmpl}}
		customName := "custom"

		// when
		st, err := s.StateFromTemplate(templateName, customName)

		// then
		assert.NoError(t, err)
		assert.Equal(t, st.Details().Fields[bundle.RelationKeyName.String()].GetStringValue(), customName)
		assert.Equal(t, st.Get(template.TitleBlockId).Model().GetText().Text, customName)
	})
}
