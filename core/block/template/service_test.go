package template

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const deletedTemplateId = "iamdeleted"

type testPicker struct {
	sb smartblock.SmartBlock
}

func (t *testPicker) GetObject(ctx context.Context, id string) (sb smartblock.SmartBlock, err error) {
	if id == deletedTemplateId {
		return nil, spacestorage.ErrTreeStorageAlreadyDeleted
	}
	return t.sb, nil
}

func (t *testPicker) GetObjectByFullID(ctx context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
	return t.sb, nil
}

func (t *testPicker) Init(a *app.App) error { return nil }

func (t *testPicker) Name() string { return "" }

func NewTemplateTest(templateName, typeKey string) smartblock.SmartBlock {
	sb := smarttest.New(templateName)
	_ = sb.SetDetails(nil, []*pb.RpcObjectSetDetailsDetail{{
		Key:   bundle.RelationKeyName.String(),
		Value: pbtypes.String(templateName),
	}}, false)
	sb.Doc.(*state.State).SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTemplate, domain.TypeKey(typeKey)})
	sb.AddBlock(simple.New(&model.Block{Id: templateName, ChildrenIds: []string{template.TitleBlockId}}))
	sb.AddBlock(simple.New(&model.Block{Id: template.TitleBlockId, Content: &model.BlockContentOfText{
		Text: &model.BlockContentText{
			Text: templateName,
		},
	}}))
	return sb
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

	t.Run("empty templateId", func(t *testing.T) {
		// given
		tmpl := NewTemplateTest(templateName, "")
		s := service{picker: &testPicker{sb: tmpl}}

		// when
		st, err := s.StateFromTemplate("", "")

		// then
		assert.NoError(t, err)
		assert.Equal(t, st.RootId(), BlankTemplateId)
	})

	t.Run("blank templateId", func(t *testing.T) {
		// given
		tmpl := NewTemplateTest(templateName, "")
		s := service{picker: &testPicker{sb: tmpl}}

		// when
		st, err := s.StateFromTemplate(BlankTemplateId, "")

		// then
		assert.NoError(t, err)
		assert.Equal(t, st.RootId(), BlankTemplateId)
	})

	t.Run("create blank template in case template object is deleted", func(t *testing.T) {
		// given
		s := service{picker: &testPicker{}}

		// when
		st, err := s.StateFromTemplate(deletedTemplateId, "")

		// then
		assert.NoError(t, err)
		assert.Equal(t, st.RootId(), BlankTemplateId)

	})

	t.Run("requested smartblock is not a template", func(t *testing.T) {
		// given
		tmpl := NewTemplateTest(templateName, "")
		tmpl.(*smarttest.SmartTest).Doc.(*state.State).SetObjectTypeKey(bundle.TypeKeyBook)
		s := service{picker: &testPicker{}}

		// when
		_, err := s.StateFromTemplate(templateName, "")

		// then
		assert.Error(t, err)
	})

	t.Run("template typeKey is removed", func(t *testing.T) {
		// given
		tmpl := NewTemplateTest(templateName, bundle.TypeKeyWeeklyPlan.String())
		s := service{picker: &testPicker{sb: tmpl}}

		// when
		st, err := s.StateFromTemplate(templateName, "")

		// then
		assert.NoError(t, err)
		assert.Equal(t, st.ObjectTypeKey(), bundle.TypeKeyWeeklyPlan)
	})
}
