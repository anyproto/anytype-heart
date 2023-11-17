package template

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	deletedTemplateId  = "iamdeleted"
	archivedTemplateId = "iamarchived"
)

type testPicker struct {
	sb smartblock.SmartBlock
}

func (t *testPicker) GetObject(_ context.Context, id string) (sb smartblock.SmartBlock, err error) {
	if id == deletedTemplateId {
		return nil, spacestorage.ErrTreeStorageAlreadyDeleted
	}
	return t.sb, nil
}

func (t *testPicker) GetObjectByFullID(_ context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
	return t.sb, nil
}

func (t *testPicker) Init(_ *app.App) error { return nil }

func (t *testPicker) Name() string { return "" }

func NewTemplateTest(templateName, typeKey string) smartblock.SmartBlock {
	sb := smarttest.New(templateName)
	details := []*pb.RpcObjectSetDetailsDetail{
		{
			Key:   bundle.RelationKeyName.String(),
			Value: pbtypes.String(templateName),
		},
		{
			Key:   bundle.RelationKeyDescription.String(),
			Value: pbtypes.String(templateName),
		},
	}
	if templateName == archivedTemplateId {
		details = append(details, &pb.RpcObjectSetDetailsDetail{
			Key:   bundle.RelationKeyIsArchived.String(),
			Value: pbtypes.Bool(true),
		})
	}
	_ = sb.SetDetails(nil, details, false)
	sb.Doc.(*state.State).SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTemplate, domain.TypeKey(typeKey)})
	sb.AddBlock(simple.New(&model.Block{Id: templateName, ChildrenIds: []string{template.TitleBlockId, template.DescriptionBlockId}}))
	sb.AddBlock(text.NewDetails(&model.Block{
		Id: template.TitleBlockId,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{},
		},
		Fields: &types.Struct{
			Fields: map[string]*types.Value{
				text.DetailsKeyFieldName: pbtypes.String("name"),
			},
		},
	}, text.DetailsKeys{
		Text:    "name",
		Checked: "done",
	}))
	sb.AddBlock(text.NewDetails(&model.Block{
		Id: template.DescriptionBlockId,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{},
		},
		Fields: &types.Struct{
			Fields: map[string]*types.Value{
				text.DetailsKeyFieldName: pbtypes.String(template.DescriptionBlockId),
			},
		},
	}, text.DetailsKeys{
		Text:    template.DescriptionBlockId,
		Checked: "done",
	}))
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
		st, err := s.CreateTemplateStateWithDetails(templateName, nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, st.Details().Fields[bundle.RelationKeyName.String()].GetStringValue(), templateName)
		assert.Equal(t, st.Get(template.TitleBlockId).Model().GetText().Text, templateName)
	})

	for templateIndex, templateName := range []string{templateName, "", BlankTemplateId} {
		for addedDetail, expected := range map[string][]string{
			"custom": {"custom", "custom", "custom"},
			"":       {templateName, "", ""},
		} {
			t.Run(fmt.Sprintf("custom page name and description - "+
				"when template is %s and target detail is %s", templateName, addedDetail), func(t *testing.T) {
				// given
				tmpl := NewTemplateTest(templateName, "")
				s := service{picker: &testPicker{sb: tmpl}}
				details := &types.Struct{Fields: map[string]*types.Value{}}
				details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(addedDetail)
				details.Fields[bundle.RelationKeyDescription.String()] = pbtypes.String(addedDetail)

				// when
				st, err := s.CreateTemplateStateWithDetails(templateName, details)

				// then
				assert.NoError(t, err)
				assert.Equal(t, expected[templateIndex], st.Details().Fields[bundle.RelationKeyName.String()].GetStringValue())
				assert.Equal(t, expected[templateIndex], st.Details().Fields[bundle.RelationKeyDescription.String()].GetStringValue())
				assert.Equal(t, expected[templateIndex], st.Get(template.TitleBlockId).Model().GetText().Text)
			})
		}
	}

	for _, testCase := range [][]string{
		{"templateId is empty", ""},
		{"templateId is blank", BlankTemplateId},
		{"target template is deleted", deletedTemplateId},
		{"target template is archived", archivedTemplateId},
	} {
		t.Run("create blank template in case "+testCase[0], func(t *testing.T) {
			// given
			tmpl := NewTemplateTest(testCase[1], "")
			s := service{picker: &testPicker{sb: tmpl}}

			// when
			st, err := s.CreateTemplateStateWithDetails(testCase[1], nil)

			// then
			assert.NoError(t, err)
			assert.Equal(t, BlankTemplateId, st.RootId())
		})
	}

	t.Run("requested smartblock is not a template", func(t *testing.T) {
		// given
		tmpl := NewTemplateTest(templateName, "")
		tmpl.(*smarttest.SmartTest).Doc.(*state.State).SetObjectTypeKey(bundle.TypeKeyBook)
		s := service{picker: &testPicker{}}

		// when
		_, err := s.CreateTemplateStateWithDetails(templateName, nil)

		// then
		assert.Error(t, err)
	})

	t.Run("template typeKey is removed", func(t *testing.T) {
		// given
		tmpl := NewTemplateTest(templateName, bundle.TypeKeyWeeklyPlan.String())
		s := service{picker: &testPicker{sb: tmpl}}

		// when
		st, err := s.CreateTemplateStateWithDetails(templateName, nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, bundle.TypeKeyWeeklyPlan, st.ObjectTypeKey())
	})
}
