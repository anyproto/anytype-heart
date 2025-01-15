package template

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	mock_space "github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
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
	details := []domain.Detail{
		{
			Key:   bundle.RelationKeyName,
			Value: domain.String(templateName),
		},
		{
			Key:   bundle.RelationKeyDescription,
			Value: domain.String(templateName),
		},
	}
	if templateName == archivedTemplateId {
		details = append(details, domain.Detail{
			Key:   bundle.RelationKeyIsArchived,
			Value: domain.Bool(true),
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
		}}, text.DetailsKeys{
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
		assert.Equal(t, st.Details().GetString(bundle.RelationKeyName), templateName)
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
				details := domain.NewDetails()
				details.Set(bundle.RelationKeyName, domain.String(addedDetail))
				details.Set(bundle.RelationKeyDescription, domain.String(addedDetail))

				// when
				st, err := s.CreateTemplateStateWithDetails(templateName, details)

				// then
				assert.NoError(t, err)
				assert.Equal(t, expected[templateIndex], st.Details().GetString(bundle.RelationKeyName))
				assert.Equal(t, expected[templateIndex], st.Details().GetString(bundle.RelationKeyDescription))
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
			assert.Contains(t, st.Details().GetStringList(bundle.RelationKeyFeaturedRelations), bundle.RelationKeyTag.String())
			assert.True(t, st.Details().Has(bundle.RelationKeyTag))
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
		tmpl := NewTemplateTest(templateName, bundle.TypeKeyGoal.String())
		s := service{picker: &testPicker{sb: tmpl}}

		// when
		st, err := s.CreateTemplateStateWithDetails(templateName, nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, bundle.TypeKeyGoal, st.ObjectTypeKey())
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
			details := domain.NewDetails()
			details.Set(bundle.RelationKeyResolvedLayout, domain.Int64(int64(layout)))

			// when
			st, err := s.CreateTemplateStateWithDetails(BlankTemplateId, details)

			// then
			assert.NoError(t, err)
			assert.Equal(t, layout, model.ObjectTypeLayout(st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout)))
			assertLayoutBlocks(t, st, layout)
		})
	}

	t.Run("do not inherit addedDate and creationDate", func(t *testing.T) {
		// given
		sometime := time.Now().Unix()

		tmpl := smarttest.New(templateName)
		tmpl.Doc.(*state.State).SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTemplate, bundle.TypeKeyBook})
		tmpl.Doc.(*state.State).SetOriginalCreatedTimestamp(sometime)
		err := tmpl.SetDetails(nil, []domain.Detail{{Key: bundle.RelationKeyAddedDate, Value: domain.Int64(sometime)}}, false)
		require.NoError(t, err)

		s := service{picker: &testPicker{tmpl}}

		// when
		st, err := s.CreateTemplateStateWithDetails(templateName, nil)

		// then
		assert.NoError(t, err)
		assert.Zero(t, st.OriginalCreatedTimestamp())
		assert.Zero(t, st.Details().GetInt64(bundle.RelationKeyAddedDate))
		assert.Zero(t, st.Details().GetInt64(bundle.RelationKeyCreatedDate))
	})
}

func TestCreateTemplateStateFromSmartBlock(t *testing.T) {
	t.Run("if failed to build state -> return blank template", func(t *testing.T) {
		// given
		s := service{converter: converter.NewLayoutConverter()}

		// when
		st := s.CreateTemplateStateFromSmartBlock(nil, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_todo)),
		}))

		// then
		assert.Equal(t, BlankTemplateId, st.RootId())
		assert.Contains(t, st.Details().GetStringList(bundle.RelationKeyFeaturedRelations), bundle.RelationKeyTag.String())
		assert.True(t, st.Details().Has(bundle.RelationKeyTag))
	})

	t.Run("create state from template smartblock", func(t *testing.T) {
		// given
		tmpl := NewTemplateTest("template", bundle.TypeKeyProject.String())
		s := service{}

		// when
		st := s.CreateTemplateStateFromSmartBlock(tmpl, nil)

		// then
		assert.Equal(t, "template", st.Details().GetString(bundle.RelationKeyName))
		assert.Equal(t, "template", st.Details().GetString(bundle.RelationKeyDescription))
	})
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
		Key                        domain.RelationKey
		OriginValue, TemplateValue domain.Value
		OriginLeft                 bool
	}{
		{Key: bundle.RelationKeyResolvedLayout, OriginValue: domain.Int64(0), TemplateValue: domain.Int64(1), OriginLeft: false},
		{Key: bundle.RelationKeyResolvedLayout, OriginValue: domain.Int64(5), TemplateValue: domain.Int64(0), OriginLeft: false},
		{Key: bundle.RelationKeyResolvedLayout, OriginValue: domain.Int64(3), TemplateValue: domain.Int64(3), OriginLeft: false},
		{Key: bundle.RelationKeySourceObject, OriginValue: domain.String(""), TemplateValue: domain.String("s1"), OriginLeft: false},
		{Key: bundle.RelationKeySourceObject, OriginValue: domain.String("s2"), TemplateValue: domain.String(""), OriginLeft: true},
		{Key: bundle.RelationKeySourceObject, OriginValue: domain.String("s0"), TemplateValue: domain.String("s3"), OriginLeft: false},
		{Key: bundle.RelationKeyFeaturedRelations, OriginValue: domain.StringList([]string{"tag"}), TemplateValue: domain.StringList([]string{}), OriginLeft: true},
		{Key: bundle.RelationKeyFeaturedRelations, OriginValue: domain.StringList([]string{}), TemplateValue: domain.StringList([]string{"tag", "type"}), OriginLeft: false},
		{Key: bundle.RelationKeyFeaturedRelations, OriginValue: domain.StringList([]string{"type"}), TemplateValue: domain.StringList([]string{"tag"}), OriginLeft: false},
		{Key: bundle.RelationKeyName, OriginValue: domain.String("orig"), TemplateValue: domain.String(""), OriginLeft: true},
		{Key: bundle.RelationKeyName, OriginValue: domain.String(""), TemplateValue: domain.String("tmpl"), OriginLeft: false},
		{Key: bundle.RelationKeyName, OriginValue: domain.String("orig"), TemplateValue: domain.String("tmpl"), OriginLeft: true},
		{Key: bundle.RelationKeyDescription, OriginValue: domain.String("orig"), TemplateValue: domain.String(""), OriginLeft: true},
		{Key: bundle.RelationKeyDescription, OriginValue: domain.String(""), TemplateValue: domain.String("tmpl"), OriginLeft: false},
		{Key: bundle.RelationKeyDescription, OriginValue: domain.String("orig"), TemplateValue: domain.String("tmpl"), OriginLeft: true},
		{Key: bundle.RelationKeyCoverId, OriginValue: domain.String("old"), TemplateValue: domain.String(""), OriginLeft: true},
		{Key: bundle.RelationKeyCoverId, OriginValue: domain.String(""), TemplateValue: domain.String("new"), OriginLeft: false},
		{Key: bundle.RelationKeyCoverId, OriginValue: domain.String("old"), TemplateValue: domain.String("new"), OriginLeft: false},
	} {
		t.Run("merge details: key = "+testCase.Key.String()+", origin = "+testCase.OriginValue.String()+
			", template = "+testCase.TemplateValue.String()+". Should origin be left:"+strconv.FormatBool(testCase.OriginLeft),
			func(t *testing.T) {
				// given
				originDetails := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{testCase.Key: testCase.OriginValue})
				templateDetails := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{testCase.Key: testCase.TemplateValue})

				// when
				targetDetails := extractTargetDetails(originDetails, templateDetails)

				// then
				value, found := targetDetails.TryGet(testCase.Key)
				assert.Equal(t, found, testCase.OriginLeft)
				if found {
					assert.True(t, testCase.OriginValue.Equal(value))
				}
			},
		)
	}
}

func TestBuildTemplateStateFromObject(t *testing.T) {
	t.Run("building state for new template", func(t *testing.T) {
		// given
		obj := smarttest.New("object")
		err := obj.SetDetails(nil, []domain.Detail{{
			Key:   bundle.RelationKeyInternalFlags,
			Value: domain.Int64List([]int64{0, 1, 2, 3}),
		}}, false)
		assert.NoError(t, err)

		obj.SetObjectTypes([]domain.TypeKey{bundle.TypeKeyNote})

		sp := mock_space.NewMockSpace(t)
		sp.EXPECT().GetTypeIdByKey(mock.Anything, mock.Anything).Times(1).Return(bundle.TypeKeyNote.String(), nil)
		obj.SetSpace(sp)

		// when
		st, err := buildTemplateStateFromObject(obj)

		// then
		assert.NoError(t, err)
		assert.NotContains(t, st.Details().GetInt64List(bundle.RelationKeyInternalFlags), model.InternalFlag_editorDeleteEmpty)
		assert.Equal(t, []domain.TypeKey{bundle.TypeKeyTemplate, bundle.TypeKeyNote}, st.ObjectTypeKeys())
		assert.Equal(t, bundle.TypeKeyNote.String(), st.Details().GetString(bundle.RelationKeyTargetObjectType))
		assert.Nil(t, st.LocalDetails())
	})
}
