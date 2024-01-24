package template

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/converter"
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

func TestService_CreateTemplateStateWithDetails(t *testing.T) {
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
				s := service{picker: &testPicker{sb: tmpl}, converter: converter.NewLayoutConverter()}
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
			s := service{picker: &testPicker{sb: tmpl}, converter: converter.NewLayoutConverter()}

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

	for _, layout := range []model.ObjectTypeLayout{
		model.ObjectType_note,
		model.ObjectType_basic,
		model.ObjectType_profile,
		model.ObjectType_todo,
		model.ObjectType_date,
		model.ObjectType_bookmark,
	} {
		t.Run("blank template should correspond "+model.ObjectTypeLayout_name[int32(layout)]+" layout", func(t *testing.T) {
			// given
			s := service{converter: converter.NewLayoutConverter()}
			details := &types.Struct{Fields: map[string]*types.Value{}}
			details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Int64(int64(layout))

			// when
			st, err := s.CreateTemplateStateWithDetails(BlankTemplateId, details)

			// then
			assert.NoError(t, err)
			assert.Equal(t, layout, model.ObjectTypeLayout(pbtypes.GetInt64(st.Details(), bundle.RelationKeyLayout.String())))
			assertLayoutBlocks(t, st, layout)
		})
	}
}

func assertLayoutBlocks(t *testing.T, st *state.State, layout model.ObjectTypeLayout) {
	switch layout {
	case model.ObjectType_bookmark:
		foundDescription, foundTag, foundSource := false, false, false
		st.Iterate(func(b simple.Block) (isContinue bool) {
			switch b.Model().Id {
			case template.DescriptionBlockId:
				foundDescription = true
			case bundle.RelationKeyTag.String():
				foundTag = true
			case bundle.RelationKeySource.String():
				foundSource = true
			}
			return true
		})
		assert.True(t, foundDescription && foundTag && foundSource)
	case model.ObjectType_note:
		foundTitle, foundDescription := false, false
		st.Iterate(func(b simple.Block) (isContinue bool) {
			switch b.Model().Id {
			case template.DescriptionBlockId:
				foundDescription = true
				return false
			case template.TitleBlockId:
				foundTitle = true
				return false
			}
			return true
		})
		assert.False(t, foundTitle || foundDescription)
	default:
		foundTitle, foundDescription := false, false
		st.Iterate(func(b simple.Block) (isContinue bool) {
			switch b.Model().Id {
			case template.DescriptionBlockId:
				foundDescription = true
				return false
			case template.TitleBlockId:
				foundTitle = true
			}
			return true
		})
		assert.True(t, foundTitle)
		assert.False(t, foundDescription)
	}
}

func TestExtractTargetDetails(t *testing.T) {
	for _, testCase := range []struct {
		Key                        string
		OriginValue, TemplateValue *types.Value
		OriginLeft                 bool
	}{
		{Key: bundle.RelationKeyLayout.String(), OriginValue: pbtypes.Int64(0), TemplateValue: pbtypes.Int64(1), OriginLeft: false},
		{Key: bundle.RelationKeyLayout.String(), OriginValue: pbtypes.Int64(5), TemplateValue: pbtypes.Int64(0), OriginLeft: false},
		{Key: bundle.RelationKeyLayout.String(), OriginValue: pbtypes.Int64(3), TemplateValue: pbtypes.Int64(3), OriginLeft: false},
		{Key: bundle.RelationKeySourceObject.String(), OriginValue: pbtypes.String(""), TemplateValue: pbtypes.String("s1"), OriginLeft: false},
		{Key: bundle.RelationKeySourceObject.String(), OriginValue: pbtypes.String("s2"), TemplateValue: pbtypes.String(""), OriginLeft: true},
		{Key: bundle.RelationKeySourceObject.String(), OriginValue: pbtypes.String("s0"), TemplateValue: pbtypes.String("s3"), OriginLeft: false},
		{Key: bundle.RelationKeyFeaturedRelations.String(), OriginValue: pbtypes.StringList([]string{"tag"}), TemplateValue: pbtypes.StringList([]string{}), OriginLeft: true},
		{Key: bundle.RelationKeyFeaturedRelations.String(), OriginValue: pbtypes.StringList([]string{}), TemplateValue: pbtypes.StringList([]string{"tag", "type"}), OriginLeft: false},
		{Key: bundle.RelationKeyFeaturedRelations.String(), OriginValue: pbtypes.StringList([]string{"type"}), TemplateValue: pbtypes.StringList([]string{"tag"}), OriginLeft: false},
		{Key: bundle.RelationKeyName.String(), OriginValue: pbtypes.String("orig"), TemplateValue: pbtypes.String(""), OriginLeft: true},
		{Key: bundle.RelationKeyName.String(), OriginValue: pbtypes.String(""), TemplateValue: pbtypes.String("tmpl"), OriginLeft: false},
		{Key: bundle.RelationKeyName.String(), OriginValue: pbtypes.String("orig"), TemplateValue: pbtypes.String("tmpl"), OriginLeft: true},
		{Key: bundle.RelationKeyDescription.String(), OriginValue: pbtypes.String("orig"), TemplateValue: pbtypes.String(""), OriginLeft: true},
		{Key: bundle.RelationKeyDescription.String(), OriginValue: pbtypes.String(""), TemplateValue: pbtypes.String("tmpl"), OriginLeft: false},
		{Key: bundle.RelationKeyDescription.String(), OriginValue: pbtypes.String("orig"), TemplateValue: pbtypes.String("tmpl"), OriginLeft: true},
		{Key: bundle.RelationKeyCoverId.String(), OriginValue: pbtypes.String("old"), TemplateValue: pbtypes.String(""), OriginLeft: true},
		{Key: bundle.RelationKeyCoverId.String(), OriginValue: pbtypes.String(""), TemplateValue: pbtypes.String("new"), OriginLeft: false},
		{Key: bundle.RelationKeyCoverId.String(), OriginValue: pbtypes.String("old"), TemplateValue: pbtypes.String("new"), OriginLeft: false},
	} {
		t.Run("merge details: key = "+testCase.Key+", origin = "+testCase.OriginValue.String()+
			", template = "+testCase.TemplateValue.String()+". Should origin be left:"+strconv.FormatBool(testCase.OriginLeft),
			func(t *testing.T) {
				// given
				originDetails := &types.Struct{Fields: map[string]*types.Value{testCase.Key: testCase.OriginValue}}
				templateDetails := &types.Struct{Fields: map[string]*types.Value{testCase.Key: testCase.TemplateValue}}

				// when
				targetDetails := extractTargetDetails(originDetails, templateDetails)

				// then
				value, found := targetDetails.Fields[testCase.Key]
				assert.Equal(t, found, testCase.OriginLeft)
				if found {
					assert.True(t, testCase.OriginValue.Equal(value))
				}
			},
		)
	}
}
