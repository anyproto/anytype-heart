package templateimpl

import (
	"context"
	"fmt"
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
	templateSvc "github.com/anyproto/anytype-heart/core/block/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils/mock_relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	deletedTemplateId  = "iamdeleted"
	archivedTemplateId = "iamarchived"
	testAccount        = "accountABC"
)

type testPicker struct {
	objects map[string]smartblock.SmartBlock
}

func (t *testPicker) GetObject(_ context.Context, id string) (sb smartblock.SmartBlock, err error) {
	if id == deletedTemplateId {
		return nil, spacestorage.ErrTreeStorageAlreadyDeleted
	}
	sb, ok := t.objects[id]
	if !ok {
		return nil, fmt.Errorf("object %s not found", id)
	}
	return sb, nil
}

func (t *testPicker) GetObjectByFullID(_ context.Context, id domain.FullID) (sb smartblock.SmartBlock, err error) {
	return t.GetObject(nil, id.ObjectID)
}

func (t *testPicker) Init(_ *app.App) error { return nil }

func (t *testPicker) Name() string { return "" }

func newTemplateTest(templateName, typeKey string) smartblock.SmartBlock {
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

	t.Run("empty page name should remain empty after template apply", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateName, "")
		s := service{picker: &testPicker{objects: map[string]smartblock.SmartBlock{templateName: tmpl}}}

		// when
		st, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: templateName})

		// then
		assert.NoError(t, err)
		assert.Empty(t, st.Details().GetString(bundle.RelationKeyName))
		assert.Empty(t, st.Get(template.TitleBlockId).Model().GetText().Text)
	})

	for _, nameToAdd := range []string{"custom", ""} {
		for _, nameInTemplate := range []string{templateName, "", blankTemplateId} {
			t.Run(fmt.Sprintf("must apply custom page name and description - "+
				"when template is %s and target detail is %s", nameInTemplate, nameToAdd), func(t *testing.T) {
				// given
				tmpl := newTemplateTest(nameInTemplate, "")
				s := service{picker: &testPicker{objects: map[string]smartblock.SmartBlock{templateName: tmpl}}, converter: converter.NewLayoutConverter()}
				details := domain.NewDetails()
				details.Set(bundle.RelationKeyName, domain.String(nameToAdd))
				details.Set(bundle.RelationKeyDescription, domain.String(nameToAdd))

				// when
				st, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: nameInTemplate, Details: details})

				// then
				assert.NoError(t, err)
				assert.Equal(t, nameToAdd, st.Details().GetString(bundle.RelationKeyName))
				assert.Equal(t, nameToAdd, st.Details().GetString(bundle.RelationKeyDescription))
				assert.Equal(t, nameToAdd, st.Get(template.TitleBlockId).Model().GetText().Text)
			})
		}
	}

	for _, testCase := range [][]string{
		{"templateId is empty", ""},
		{"templateId is blank", blankTemplateId},
		{"target template is deleted", deletedTemplateId},
		{"target template is archived", archivedTemplateId},
	} {
		t.Run("create blank template in case "+testCase[0], func(t *testing.T) {
			// given
			tmpl := newTemplateTest(testCase[1], "")
			s := service{picker: &testPicker{objects: map[string]smartblock.SmartBlock{testCase[1]: tmpl}}, converter: converter.NewLayoutConverter()}

			// when
			st, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: testCase[1]})

			// then
			assert.NoError(t, err)
			assert.Equal(t, blankTemplateId, st.RootId())
			assert.True(t, st.Details().Has(bundle.RelationKeyTag))
		})
	}

	t.Run("requested smartblock is not a template", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateName, "")
		tmpl.(*smarttest.SmartTest).Doc.(*state.State).SetObjectTypeKey(bundle.TypeKeyBook)
		s := service{picker: &testPicker{}}

		// when
		_, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: templateName})

		// then
		assert.Error(t, err)
	})

	t.Run("template typeKey is removed", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateName, bundle.TypeKeyGoal.String())
		s := service{picker: &testPicker{objects: map[string]smartblock.SmartBlock{templateName: tmpl}}}

		// when
		st, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: templateName})

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

			// when
			st, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: blankTemplateId, Layout: layout})

			// then
			assert.NoError(t, err)
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

		s := service{picker: &testPicker{objects: map[string]smartblock.SmartBlock{templateName: tmpl}}}

		// when
		st, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: templateName})

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
		st := s.CreateTemplateStateFromSmartBlock(nil, templateSvc.CreateTemplateRequest{Layout: model.ObjectType_todo})

		// then
		assert.Equal(t, blankTemplateId, st.RootId())
		assert.True(t, st.Details().Has(bundle.RelationKeyTag))
	})

	t.Run("create state from template smartblock", func(t *testing.T) {
		// given
		tmpl := newTemplateTest("template", bundle.TypeKeyProject.String())
		s := service{}

		// when
		st := s.CreateTemplateStateFromSmartBlock(tmpl, templateSvc.CreateTemplateRequest{})

		// then
		assert.Empty(t, st.Details().GetString(bundle.RelationKeyName))
		assert.Equal(t, "template", st.Details().GetString(bundle.RelationKeyDescription))
	})
}

func TestService_resolveValidTemplateId(t *testing.T) {
	var (
		spaceId     = "cosmos"
		templateId1 = "template1"
		templateId2 = "template2"
		templateId3 = "template3"
	)

	store := objectstore.NewStoreFixture(t)
	store.AddObjects(t, spaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String(templateId1),
			bundle.RelationKeyType:             domain.String(bundle.TypeKeyTemplate.URL()),
			bundle.RelationKeyTargetObjectType: domain.String(bundle.TypeKeyTask.URL()),
		},
		{
			bundle.RelationKeyId:               domain.String(templateId2),
			bundle.RelationKeyType:             domain.String(bundle.TypeKeyTemplate.URL()),
			bundle.RelationKeyTargetObjectType: domain.String(bundle.TypeKeyTask.URL()),
		},
		{
			bundle.RelationKeyId:               domain.String(templateId3),
			bundle.RelationKeyType:             domain.String(bundle.TypeKeyTemplate.URL()),
			bundle.RelationKeyTargetObjectType: domain.String(bundle.TypeKeyTask.URL()),
			bundle.RelationKeyIsDeleted:        domain.Bool(true),
		},
	})

	spaceService := mock_space.NewMockService(t)
	spc := mock_clientspace.NewMockSpace(t)
	spaceService.EXPECT().Get(mock.Anything, mock.Anything).Return(spc, nil)
	spc.EXPECT().GetTypeIdByKey(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.TypeKey) (string, error) {
		return key.URL(), nil
	})

	s := &service{
		store:        store,
		spaceService: spaceService,
	}

	for _, tc := range []struct {
		name                string
		typeKey             domain.TypeKey
		requestedTemplateId string
		expectedTemplateId  string
	}{
		{"requested template is valid", bundle.TypeKeyTask, templateId2, templateId2},
		{"requested template is invalid", bundle.TypeKeyTask, "invalid", ""},
		{"requested template is deleted", bundle.TypeKeyTask, templateId3, ""},
		{"requested template is empty", bundle.TypeKeyTask, "", ""},
		{"no valid template exists", bundle.TypeKeyBook, "templateId", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// when
			templateId, err := s.resolveValidTemplateId(spaceId, tc.requestedTemplateId, tc.typeKey.URL())

			// then
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedTemplateId, templateId)
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

		sp := mock_clientspace.NewMockSpace(t)
		sp.EXPECT().GetTypeIdByKey(mock.Anything, mock.Anything).Times(1).Return(bundle.TypeKeyNote.String(), nil)
		obj.SetSpace(sp)
		obj.SetSpaceId("space1")

		s := service{
			store: objectstore.NewStoreFixture(t),
		}

		// when
		st, err := s.buildTemplateStateFromObject(obj)

		// then
		assert.NoError(t, err)
		assert.NotContains(t, st.Details().GetInt64List(bundle.RelationKeyInternalFlags), model.InternalFlag_editorDeleteEmpty)
		assert.Equal(t, []domain.TypeKey{bundle.TypeKeyTemplate, bundle.TypeKeyNote}, st.ObjectTypeKeys())
		assert.Equal(t, bundle.TypeKeyNote.String(), st.Details().GetString(bundle.RelationKeyTargetObjectType))
		assert.Nil(t, st.LocalDetails())
	})
}

func newTemplateTestWithPrefillType(templateName, typeKey string, prefillType model.TemplateNamePrefillType) smartblock.SmartBlock {
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
		{
			Key:   bundle.RelationKeyTemplateNamePrefillType,
			Value: domain.Int64(int64(prefillType)),
		},
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

func TestService_TemplateNamePrefill(t *testing.T) {
	const templateName = "My Template Name"

	t.Run("prefill type Empty - name should not be inherited from template", func(t *testing.T) {
		// given
		tmpl := newTemplateTestWithPrefillType(templateName, bundle.TypeKeyTask.String(), model.TemplateNamePrefillType_Empty)
		s := service{picker: &testPicker{objects: map[string]smartblock.SmartBlock{templateName: tmpl}}}

		// when
		st, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: templateName})

		// then
		assert.NoError(t, err)
		assert.Empty(t, st.Details().GetString(bundle.RelationKeyName), "name should be empty when prefill type is Empty")
	})

	t.Run("prefill type FromTemplateName - name should be inherited from template", func(t *testing.T) {
		// given
		tmpl := newTemplateTestWithPrefillType(templateName, bundle.TypeKeyTask.String(), model.TemplateNamePrefillType_FromTemplateName)
		s := service{picker: &testPicker{objects: map[string]smartblock.SmartBlock{templateName: tmpl}}}

		// when
		st, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: templateName})

		// then
		assert.NoError(t, err)
		assert.Equal(t, templateName, st.Details().GetString(bundle.RelationKeyName), "name should be inherited when prefill type is FromTemplateName")
	})

	t.Run("prefill type not set (default) - name should not be inherited from template", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateName, bundle.TypeKeyTask.String())
		s := service{picker: &testPicker{objects: map[string]smartblock.SmartBlock{templateName: tmpl}}}

		// when
		st, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: templateName})

		// then
		assert.NoError(t, err)
		assert.Empty(t, st.Details().GetString(bundle.RelationKeyName), "name should be empty when prefill type is not set (defaults to Empty)")
	})

	t.Run("prefill type FromTemplateName with custom details - custom name takes precedence", func(t *testing.T) {
		// given
		customName := "Custom Object Name"
		tmpl := newTemplateTestWithPrefillType(templateName, bundle.TypeKeyTask.String(), model.TemplateNamePrefillType_FromTemplateName)
		s := service{picker: &testPicker{objects: map[string]smartblock.SmartBlock{templateName: tmpl}}}
		details := domain.NewDetails()
		details.Set(bundle.RelationKeyName, domain.String(customName))

		// when
		st, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: templateName, Details: details})

		// then
		assert.NoError(t, err)
		assert.Equal(t, customName, st.Details().GetString(bundle.RelationKeyName), "custom name should take precedence over template name")
		assert.True(t, st.PickRelationLinks().Has(bundle.RelationKeyName.String()), "Name relation link should exist")
	})

	t.Run("prefill type Empty with custom details - custom name is applied", func(t *testing.T) {
		// given
		customName := "Custom Object Name"
		tmpl := newTemplateTestWithPrefillType(templateName, bundle.TypeKeyTask.String(), model.TemplateNamePrefillType_Empty)
		s := service{picker: &testPicker{objects: map[string]smartblock.SmartBlock{templateName: tmpl}}}
		details := domain.NewDetails()
		details.Set(bundle.RelationKeyName, domain.String(customName))

		// when
		st, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: templateName, Details: details})

		// then
		assert.NoError(t, err)
		assert.Equal(t, customName, st.Details().GetString(bundle.RelationKeyName), "custom name should be applied when prefill type is Empty")
		assert.True(t, st.PickRelationLinks().Has(bundle.RelationKeyName.String()), "Name relation link should exist")
	})

	t.Run("prefill type FromTemplateName with empty name in details - template name should be preserved", func(t *testing.T) {
		// given
		tmpl := newTemplateTestWithPrefillType(templateName, bundle.TypeKeyTask.String(), model.TemplateNamePrefillType_FromTemplateName)
		s := service{picker: &testPicker{objects: map[string]smartblock.SmartBlock{templateName: tmpl}}}
		details := domain.NewDetails()
		details.Set(bundle.RelationKeyName, domain.String(""))

		// when
		st, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: templateName, Details: details})

		// then
		assert.NoError(t, err)
		assert.Equal(t, templateName, st.Details().GetString(bundle.RelationKeyName), "template name should be preserved when details has empty name")
	})

	t.Run("blank template without custom name - name should be empty", func(t *testing.T) {
		// given
		s := service{converter: converter.NewLayoutConverter()}

		// when
		st, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: blankTemplateId})

		// then
		assert.NoError(t, err)
		assert.Equal(t, blankTemplateId, st.RootId())
		assert.Empty(t, st.Details().GetString(bundle.RelationKeyName), "blank template should have empty name")
	})

	t.Run("blank template with custom name - custom name should be applied", func(t *testing.T) {
		// given
		customName := "Custom Object Name"
		s := service{converter: converter.NewLayoutConverter()}
		details := domain.NewDetails()
		details.Set(bundle.RelationKeyName, domain.String(customName))

		// when
		st, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: blankTemplateId, Details: details})

		// then
		assert.NoError(t, err)
		assert.Equal(t, blankTemplateId, st.RootId())
		assert.Equal(t, customName, st.Details().GetString(bundle.RelationKeyName), "custom name should be applied to blank template")
		assert.True(t, st.PickRelationLinks().Has(bundle.RelationKeyName.String()), "Name relation link should exist")
	})

	t.Run("blank template with empty name in details - name should remain empty", func(t *testing.T) {
		// given
		s := service{converter: converter.NewLayoutConverter()}
		details := domain.NewDetails()
		details.Set(bundle.RelationKeyName, domain.String(""))

		// when
		st, err := s.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{TemplateId: blankTemplateId, Details: details})

		// then
		assert.NoError(t, err)
		assert.Equal(t, blankTemplateId, st.RootId())
		assert.Empty(t, st.Details().GetString(bundle.RelationKeyName), "blank template should have empty name when details has empty name")
	})
}

type accountServiceStub struct {
	accountId string
}

func (a accountServiceStub) AccountID() string { return a.accountId }

func newTemplateTestWithPlaceholders(templateName, typeKey string, placeholders []*model.Placeholder) smartblock.SmartBlock {
	sb := newTemplateTest(templateName, typeKey).(*smarttest.SmartTest)
	s := sb.NewState()
	for _, placeholder := range placeholders {
		key := placeholder.RelationKey
		values := make([]*types.Value, 0, len(placeholder.Values))
		for _, v := range placeholder.Values {
			values = append(values, pbtypes.Struct(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				"type":  domain.Int64(int64(v.Type)),
				"value": domain.ValueFromProto(v.Value),
			}).ToProto()))
		}
		s.SetInStore([]string{placeholdersStoreKey, key}, &types.Value{
			Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: values}},
		})
	}
	_ = sb.Apply(s)
	return sb
}

type fixture struct {
	*service
	picker *testPicker
	store  *objectstore.StoreFixture
}

func newFixture(t *testing.T) *fixture {
	picker := &testPicker{objects: make(map[string]smartblock.SmartBlock)}
	store := objectstore.NewStoreFixture(t)
	fetcher := mock_relationutils.NewMockRelationFormatFetcher(t)
	fetcher.EXPECT().GetRelationFormatByKey(mock.Anything, mock.Anything).RunAndReturn(func(spaceId string, key domain.RelationKey) (model.RelationFormat, error) {
		// Try bundle first
		format, err := bundle.GetRelationFormat(key)
		if err == nil {
			return format, nil
		}
		// Fallback to store for custom test relations
		rel, err := store.SpaceIndex(spaceId).FetchRelationByKey(key.String())
		if err != nil {
			return 0, err
		}
		return rel.Format, nil
	}).Maybe()
	return &fixture{
		service: &service{
			picker:         picker,
			store:          store,
			formatFetcher:  fetcher,
			accountService: accountServiceStub{accountId: testAccount},
		},
		picker: picker,
		store:  store,
	}
}

func TestService_ResolveTemplatePlaceholders(t *testing.T) {
	const (
		testSpaceId  = "space1"
		templateName = "template"
	)

	t.Run("template with _today placeholder resolves to current date", func(t *testing.T) {
		// given
		tmpl := newTemplateTestWithPlaceholders(templateName, bundle.TypeKeyTask.String(), []*model.Placeholder{{
			RelationKey: "dueDate",
			Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderToday, Value: pbtypes.Null()},
			},
		}})
		fx := newFixture(t)
		fx.picker.objects[templateName] = tmpl

		// when
		st, err := fx.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{
			SpaceId:    testSpaceId,
			TemplateId: templateName,
		})

		// then
		require.NoError(t, err)
		dueDateVal := st.Details().GetFloat64("dueDate")
		assert.NotZero(t, dueDateVal, "dueDate should be resolved to a timestamp")
		placeholders := st.GetStoreStruct(placeholdersStoreKey)
		assert.True(t, placeholders == nil || placeholders.Fields == nil || len(placeholders.Fields) == 0, "templatePlaceholders should be removed from new object")
	})

	t.Run("template with _current_user placeholder resolves to participant ID", func(t *testing.T) {
		// given
		tmpl := newTemplateTestWithPlaceholders(templateName, bundle.TypeKeyTask.String(), []*model.Placeholder{
			{RelationKey: "assignee", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderCurrentUser, Value: pbtypes.Null()}}},
		})
		fx := newFixture(t)
		fx.picker.objects[templateName] = tmpl

		// when
		st, err := fx.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{
			SpaceId:    testSpaceId,
			TemplateId: templateName,
		})

		// then
		require.NoError(t, err)
		assigneeList := st.Details().GetStringList("assignee")
		expectedParticipantId := domain.NewParticipantId(testSpaceId, testAccount)
		require.Len(t, assigneeList, 1)
		assert.Equal(t, expectedParticipantId, assigneeList[0])
		placeholders := st.GetStoreStruct(placeholdersStoreKey)
		assert.True(t, placeholders == nil || placeholders.Fields == nil || len(placeholders.Fields) == 0)
	})

	t.Run("template with no placeholders - no changes", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateName, bundle.TypeKeyTask.String())
		fx := newFixture(t)
		fx.picker.objects[templateName] = tmpl

		// when
		st, err := fx.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{
			SpaceId:    testSpaceId,
			TemplateId: templateName,
		})

		// then
		require.NoError(t, err)
		placeholders := st.GetStoreStruct(placeholdersStoreKey)
		assert.True(t, placeholders == nil || placeholders.Fields == nil || len(placeholders.Fields) == 0)
	})

	t.Run("template with multiple placeholders", func(t *testing.T) {
		// given
		tmpl := newTemplateTestWithPlaceholders(templateName, bundle.TypeKeyTask.String(), []*model.Placeholder{
			{RelationKey: "assignee", Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderCurrentUser, Value: pbtypes.Null()},
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("kirill")},
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("GO team")},
			}},
		})
		fx := newFixture(t)
		fx.picker.objects[templateName] = tmpl

		// when
		st, err := fx.CreateTemplateStateWithDetails(templateSvc.CreateTemplateRequest{
			SpaceId:    testSpaceId,
			TemplateId: templateName,
		})

		// then
		require.NoError(t, err)
		assigneeList := st.Details().GetStringList("assignee")
		expectedParticipantId := domain.NewParticipantId(testSpaceId, testAccount)
		require.Len(t, assigneeList, 3)
		assert.Equal(t, expectedParticipantId, assigneeList[0])
		assert.Equal(t, "kirill", assigneeList[1])
		assert.Equal(t, "GO team", assigneeList[2])
		placeholders := st.GetStoreStruct(placeholdersStoreKey)
		assert.True(t, placeholders == nil || placeholders.Fields == nil || len(placeholders.Fields) == 0)
	})

	t.Run("CreateTemplateStateFromSmartBlock also resolves placeholders", func(t *testing.T) {
		// given
		tmpl := newTemplateTestWithPlaceholders(templateName, bundle.TypeKeyTask.String(), []*model.Placeholder{
			{RelationKey: "assignee", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderCurrentUser, Value: pbtypes.Null()}}},
		})
		fx := newFixture(t)
		fx.picker.objects[templateName] = tmpl

		// when
		st := fx.CreateTemplateStateFromSmartBlock(tmpl, templateSvc.CreateTemplateRequest{
			SpaceId: testSpaceId,
		})

		// then
		assigneeList := st.Details().GetStringList(domain.RelationKey("assignee"))
		expectedParticipantId := domain.NewParticipantId(testSpaceId, testAccount)
		require.Len(t, assigneeList, 1)
		assert.Equal(t, expectedParticipantId, assigneeList[0])
		placeholders := st.GetStoreStruct(placeholdersStoreKey)
		assert.True(t, placeholders == nil || placeholders.Fields == nil || len(placeholders.Fields) == 0)
	})
}

func TestService_ObjectApplyTemplate(t *testing.T) {
	const (
		testSpaceId = "space1"
		testAccount = "accountABC"
		objectId    = "targetObject"
		templateId  = "myTemplate"
	)

	t.Run("applying template with _today placeholder resolves it on the target object", func(t *testing.T) {
		// given
		tmpl := newTemplateTestWithPlaceholders(templateId, bundle.TypeKeyTask.String(), []*model.Placeholder{
			{RelationKey: "dueDate", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderToday, Value: pbtypes.Null()}}},
		})

		targetObj := smarttest.New(objectId)
		targetObj.SetSpaceId(testSpaceId)
		targetObj.Doc.(*state.State).SetLocalDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeySpaceId:        domain.String(testSpaceId),
			bundle.RelationKeyType:           domain.String("ot-task"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_todo)),
		}))
		targetObj.Doc.(*state.State).SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTask})

		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.picker.objects[objectId] = targetObj

		// when
		err := fx.ObjectApplyTemplate(objectId, templateId)

		// then
		require.NoError(t, err)
		dueDateVal := targetObj.NewState().Details().GetFloat64(domain.RelationKey("dueDate"))
		assert.NotZero(t, dueDateVal, "dueDate should be resolved to a timestamp")
		placeholders := targetObj.NewState().GetStoreStruct(placeholdersStoreKey)
		assert.True(t, placeholders == nil || placeholders.Fields == nil || len(placeholders.Fields) == 0,
			"templatePlaceholders should be removed from store after resolution")
	})

	t.Run("applying template with _current_user placeholder resolves to participant ID", func(t *testing.T) {
		// given
		tmpl := newTemplateTestWithPlaceholders(templateId, bundle.TypeKeyTask.String(), []*model.Placeholder{
			{RelationKey: "assignee", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderCurrentUser, Value: pbtypes.Null()}}},
		})

		targetObj := smarttest.New(objectId)
		targetObj.SetSpaceId(testSpaceId)
		targetObj.Doc.(*state.State).SetLocalDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeySpaceId:        domain.String(testSpaceId),
			bundle.RelationKeyType:           domain.String("ot-task"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_todo)),
		}))
		targetObj.Doc.(*state.State).SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTask})

		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.picker.objects[objectId] = targetObj

		// when
		err := fx.ObjectApplyTemplate(objectId, templateId)

		// then
		require.NoError(t, err)
		assigneeList := targetObj.NewState().Details().GetStringList("assignee")
		expectedParticipantId := domain.NewParticipantId(testSpaceId, testAccount)
		require.Len(t, assigneeList, 1)
		assert.Equal(t, expectedParticipantId, assigneeList[0])
		placeholders := targetObj.NewState().GetStoreStruct(placeholdersStoreKey)
		assert.True(t, placeholders == nil || placeholders.Fields == nil || len(placeholders.Fields) == 0)
	})

	t.Run("applying template with multiple placeholders resolves all of them", func(t *testing.T) {
		// given
		tmpl := newTemplateTestWithPlaceholders(templateId, bundle.TypeKeyTask.String(), []*model.Placeholder{
			{RelationKey: "dueDate", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderToday, Value: pbtypes.Null()}}},
			{RelationKey: "assignee", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderCurrentUser, Value: pbtypes.Null()}}},
		})

		targetObj := smarttest.New(objectId)
		targetObj.SetSpaceId(testSpaceId)
		targetObj.Doc.(*state.State).SetLocalDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeySpaceId:        domain.String(testSpaceId),
			bundle.RelationKeyType:           domain.String("ot-task"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_todo)),
		}))
		targetObj.Doc.(*state.State).SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTask})

		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.picker.objects[objectId] = targetObj

		// when
		err := fx.ObjectApplyTemplate(objectId, templateId)

		// then
		require.NoError(t, err)

		details := targetObj.NewState().Details()
		dueDateVal := details.GetFloat64(domain.RelationKey("dueDate"))
		assert.NotZero(t, dueDateVal, "dueDate should be resolved")

		assigneeList := details.GetStringList(domain.RelationKey("assignee"))
		expectedParticipantId := domain.NewParticipantId(testSpaceId, testAccount)
		require.Len(t, assigneeList, 1)
		assert.Equal(t, expectedParticipantId, assigneeList[0])

		placeholders := targetObj.NewState().GetStoreStruct(placeholdersStoreKey)
		assert.True(t, placeholders == nil || placeholders.Fields == nil || len(placeholders.Fields) == 0)
	})

	t.Run("applying template without placeholders does not set placeholder-related details", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String())

		targetObj := smarttest.New(objectId)
		targetObj.SetSpaceId(testSpaceId)
		targetObj.Doc.(*state.State).SetLocalDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeySpaceId:        domain.String(testSpaceId),
			bundle.RelationKeyType:           domain.String("ot-task"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_todo)),
		}))
		targetObj.Doc.(*state.State).SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTask})

		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.picker.objects[objectId] = targetObj

		// when
		err := fx.ObjectApplyTemplate(objectId, templateId)

		// then
		require.NoError(t, err)
		placeholders := targetObj.NewState().GetStoreStruct(placeholdersStoreKey)
		assert.True(t, placeholders == nil || placeholders.Fields == nil || len(placeholders.Fields) == 0)
	})
}

func TestService_collectOriginalDetails(t *testing.T) {
	const (
		spaceId          = "space1"
		sourceTemplateId = "template1"
		templateEmoji    = "🚂"
		customEmoji      = "🎉"
		customObjectName = "Custom Name"
		coverIdValue     = "Sunset"
	)

	t.Run("removes Layout, SourceObject and empty values", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		s := service{store: store}

		st := state.NewDoc("test", nil).NewState()
		st.SetDetail(bundle.RelationKeyName, domain.String(customObjectName))
		st.SetDetail(bundle.RelationKeyDescription, domain.String(""))
		st.SetDetail(bundle.RelationKeyIconEmoji, domain.String(""))
		st.SetDetail(bundle.RelationKeyCoverId, domain.String(coverIdValue))
		st.SetDetail(bundle.RelationKeySourceObject, domain.String(sourceTemplateId))
		st.SetDetail(bundle.RelationKeyLayout, domain.Int64(int64(model.ObjectType_basic)))

		// when
		result := s.collectOriginalDetails(spaceId, st)

		// then
		assert.Equal(t, customObjectName, result.GetString(bundle.RelationKeyName))
		assert.False(t, result.Has(bundle.RelationKeyDescription), "empty description should be removed")
		assert.False(t, result.Has(bundle.RelationKeyIconEmoji), "empty icon should be removed")
		assert.Equal(t, coverIdValue, result.GetString(bundle.RelationKeyCoverId))
		assert.False(t, result.Has(bundle.RelationKeySourceObject), "SourceObject should be removed")
		assert.False(t, result.Has(bundle.RelationKeyLayout), "Layout should be removed")
	})

	t.Run("removes emoji when it matches previous template emoji", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:        domain.String(sourceTemplateId),
				bundle.RelationKeyIconEmoji: domain.String(templateEmoji),
			},
		})
		s := service{store: store}

		st := state.NewDoc("test", nil).NewState()
		st.SetDetail(bundle.RelationKeyIconEmoji, domain.String(templateEmoji))
		st.SetDetail(bundle.RelationKeySourceObject, domain.String(sourceTemplateId))

		// when
		result := s.collectOriginalDetails(spaceId, st)

		// then
		assert.False(t, result.Has(bundle.RelationKeyIconEmoji), "Name should be removed when it matches template name")
		assert.False(t, result.Has(bundle.RelationKeySourceObject), "SourceObject should always be removed")
	})

	t.Run("keeps emoji when it differs from previous template emoji", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:        domain.String(sourceTemplateId),
				bundle.RelationKeyIconEmoji: domain.String(templateEmoji),
			},
		})
		s := service{store: store}

		st := state.NewDoc("test", nil).NewState()
		st.SetDetail(bundle.RelationKeyIconEmoji, domain.String(customEmoji))

		// when
		result := s.collectOriginalDetails(spaceId, st)

		// then
		assert.Equal(t, customEmoji, result.GetString(bundle.RelationKeyIconEmoji))
	})

	t.Run("keeps emoji when sourceObject is empty", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		s := service{store: store}

		st := state.NewDoc("test", nil).NewState()
		st.SetDetail(bundle.RelationKeyIconEmoji, domain.String(customEmoji))

		// when
		result := s.collectOriginalDetails(spaceId, st)

		// then
		assert.Equal(t, customEmoji, result.GetString(bundle.RelationKeyIconEmoji))
	})

	t.Run("keeps empty name when no previous template", func(t *testing.T) {
		// given
		s := service{store: objectstore.NewStoreFixture(t)}
		st := state.NewDoc("test", nil).NewState()
		st.SetDetail(bundle.RelationKeyName, domain.String(""))

		// when
		result := s.collectOriginalDetails(spaceId, st)

		// then
		assert.True(t, result.Has(bundle.RelationKeyName))
	})

	t.Run("removes name when it matches previous template name", func(t *testing.T) {
		// given
		templateName := "Template Name"
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:   domain.String(sourceTemplateId),
				bundle.RelationKeyName: domain.String(templateName),
			},
		})
		s := service{store: store}

		st := state.NewDoc("test", nil).NewState()
		st.SetDetail(bundle.RelationKeyName, domain.String(templateName))
		st.SetDetail(bundle.RelationKeySourceObject, domain.String(sourceTemplateId))

		// when
		result := s.collectOriginalDetails(spaceId, st)

		// then
		assert.False(t, result.Has(bundle.RelationKeyName), "Name should be removed when it matches template name")
		assert.False(t, result.Has(bundle.RelationKeySourceObject), "SourceObject should always be removed")
	})

	t.Run("keeps name when it differs from previous template name", func(t *testing.T) {
		// given
		templateName := "Template Name"
		customName := "My Custom Name"
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:   domain.String(sourceTemplateId),
				bundle.RelationKeyName: domain.String(templateName),
			},
		})
		s := service{store: store}

		st := state.NewDoc("test", nil).NewState()
		st.SetDetail(bundle.RelationKeyName, domain.String(customName))
		st.SetDetail(bundle.RelationKeySourceObject, domain.String(sourceTemplateId))

		// when
		result := s.collectOriginalDetails(spaceId, st)

		// then
		assert.Equal(t, customName, result.GetString(bundle.RelationKeyName), "Custom name should be preserved")
	})

	t.Run("keeps name when sourceObject is empty", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		s := service{store: store}

		st := state.NewDoc("test", nil).NewState()
		st.SetDetail(bundle.RelationKeyName, domain.String(customObjectName))

		// when
		result := s.collectOriginalDetails(spaceId, st)

		// then
		assert.Equal(t, customObjectName, result.GetString(bundle.RelationKeyName))
	})

	t.Run("removes empty name when previous template also had empty name", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:   domain.String(sourceTemplateId),
				bundle.RelationKeyName: domain.String(""),
			},
		})
		s := service{store: store}

		st := state.NewDoc("test", nil).NewState()
		st.SetDetail(bundle.RelationKeyName, domain.String(""))
		st.SetDetail(bundle.RelationKeySourceObject, domain.String(sourceTemplateId))

		// when
		result := s.collectOriginalDetails(spaceId, st)

		// then
		assert.False(t, result.Has(bundle.RelationKeyName), "Empty name should be removed when previous template also had empty name")
	})
}

func TestService_SetGetTemplatePlaceholders(t *testing.T) {
	const (
		templateId = "template1"
		spaceId    = "space1"
	)

	newTemplateSmartBlock := func(id string) smartblock.SmartBlock {
		sb := smarttest.New(id)
		sb.SetSpaceId(spaceId)
		sb.SetObjectTypes([]domain.TypeKey{bundle.TypeKeyTemplate, bundle.TypeKeyTask})
		return sb
	}

	t.Run("set single placeholder and retrieve it", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		placeholders := []*model.Placeholder{
			{
				RelationKey: "dueDate",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderToday, Value: pbtypes.Null()},
				},
			},
		}

		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, placeholders)
		require.NoError(t, err)

		// then - verify storage structure
		st := tmpl.NewState()
		placeholdersStruct := st.GetStoreStruct(placeholdersStoreKey)
		require.NotNil(t, placeholdersStruct, "placeholders struct should exist in store")
		require.NotNil(t, placeholdersStruct.Fields, "placeholders fields should not be nil")

		// Verify it's a ListValue
		dueDateValue := placeholdersStruct.Fields["dueDate"]
		require.NotNil(t, dueDateValue, "dueDate should be in store")
		listValue := dueDateValue.GetListValue()
		require.NotNil(t, listValue, "dueDate should be a ListValue")
		require.Len(t, listValue.Values, 1, "should have one placeholder value")

		// Verify each value in list is a Struct with 'type' and 'value' fields
		valueStruct := listValue.Values[0].GetStructValue()
		require.NotNil(t, valueStruct, "list item should be a struct")
		require.NotNil(t, valueStruct.Fields["type"], "struct should have 'type' field")
		require.NotNil(t, valueStruct.Fields["value"], "struct should have 'value' field")
		assert.Equal(t, float64(model.Placeholder_PlaceholderToday), valueStruct.Fields["type"].GetNumberValue())

		// when - retrieve placeholders
		retrieved, err := fx.GetTemplatePlaceholders(templateId)
		require.NoError(t, err)

		// then
		require.Len(t, retrieved, 1)
		assert.Equal(t, "dueDate", retrieved[0].RelationKey)
		require.Len(t, retrieved[0].Values, 1)
		assert.Equal(t, model.Placeholder_PlaceholderToday, retrieved[0].Values[0].Type)
	})

	t.Run("set multiple placeholder values for single relation", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-tags"),
				bundle.RelationKeyRelationKey:    domain.String("tags"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
				bundle.RelationKeyUniqueKey:      domain.String("rel-tags"),
			},
		})
		placeholders := []*model.Placeholder{
			{
				RelationKey: "tags",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("tag1")},
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("tag2")},
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("tag3")},
				},
			},
		}

		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, placeholders)
		require.NoError(t, err)

		// then - verify storage structure
		st := tmpl.NewState()
		placeholdersStruct := st.GetStoreStruct(placeholdersStoreKey)
		require.NotNil(t, placeholdersStruct)

		tagsValue := placeholdersStruct.Fields["tags"]
		require.NotNil(t, tagsValue)
		listValue := tagsValue.GetListValue()
		require.NotNil(t, listValue, "tags should be stored as ListValue")
		require.Len(t, listValue.Values, 3, "should have three placeholder values")

		// Verify each value in list is a proper Struct
		for i, expectedTag := range []string{"tag1", "tag2", "tag3"} {
			valueStruct := listValue.Values[i].GetStructValue()
			require.NotNil(t, valueStruct, "list item %d should be a struct", i)
			assert.Equal(t, float64(model.Placeholder_PlaceholderValue), valueStruct.Fields["type"].GetNumberValue())
			assert.Equal(t, expectedTag, valueStruct.Fields["value"].GetStringValue())
		}

		// when - retrieve placeholders
		retrieved, err := fx.GetTemplatePlaceholders(templateId)
		require.NoError(t, err)

		// then
		require.Len(t, retrieved, 1)
		assert.Equal(t, "tags", retrieved[0].RelationKey)
		require.Len(t, retrieved[0].Values, 3)
		assert.Equal(t, "tag1", retrieved[0].Values[0].Value.GetStringValue())
		assert.Equal(t, "tag2", retrieved[0].Values[1].Value.GetStringValue())
		assert.Equal(t, "tag3", retrieved[0].Values[2].Value.GetStringValue())
	})

	t.Run("set multiple relations with different placeholder types", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		placeholders := []*model.Placeholder{
			{
				RelationKey: "dueDate",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderToday, Value: pbtypes.Null()},
				},
			},
			{
				RelationKey: "assignee",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderCurrentUser, Value: pbtypes.Null()},
				},
			},
			{
				RelationKey: "status",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("status-done")},
				},
			},
		}

		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, placeholders)
		require.NoError(t, err)

		// then - verify storage structure
		st := tmpl.NewState()
		placeholdersStruct := st.GetStoreStruct(placeholdersStoreKey)
		require.NotNil(t, placeholdersStruct)
		assert.Len(t, placeholdersStruct.Fields, 3, "should have 3 relations stored")

		// Verify each relation is stored as ListValue with proper Struct items
		for relKey, expectedType := range map[string]model.PlaceholderType{
			"dueDate":  model.Placeholder_PlaceholderToday,
			"assignee": model.Placeholder_PlaceholderCurrentUser,
			"status":   model.Placeholder_PlaceholderValue,
		} {
			relValue := placeholdersStruct.Fields[relKey]
			require.NotNil(t, relValue, "relation %s should exist", relKey)
			listValue := relValue.GetListValue()
			require.NotNil(t, listValue, "relation %s should be ListValue", relKey)
			require.Len(t, listValue.Values, 1, "relation %s should have one value", relKey)

			valueStruct := listValue.Values[0].GetStructValue()
			require.NotNil(t, valueStruct, "value for %s should be Struct", relKey)
			assert.Equal(t, float64(expectedType), valueStruct.Fields["type"].GetNumberValue())
		}

		// when - retrieve placeholders
		retrieved, err := fx.GetTemplatePlaceholders(templateId)
		require.NoError(t, err)

		// then
		require.Len(t, retrieved, 3)
		relKeys := make(map[string]*model.Placeholder)
		for _, p := range retrieved {
			relKeys[p.RelationKey] = p
		}
		assert.Contains(t, relKeys, "dueDate")
		assert.Contains(t, relKeys, "assignee")
		assert.Contains(t, relKeys, "status")
		assert.Equal(t, model.Placeholder_PlaceholderToday, relKeys["dueDate"].Values[0].Type)
		assert.Equal(t, model.Placeholder_PlaceholderCurrentUser, relKeys["assignee"].Values[0].Type)
		assert.Equal(t, model.Placeholder_PlaceholderValue, relKeys["status"].Values[0].Type)
	})

	t.Run("update existing placeholder preserves others", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl

		// Set initial placeholders
		initial := []*model.Placeholder{
			{
				RelationKey: "dueDate",
				Values:      []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderToday, Value: pbtypes.Null()}},
			},
			{
				RelationKey: "assignee",
				Values:      []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderCurrentUser, Value: pbtypes.Null()}},
			},
		}
		err := fx.SetTemplatePlaceholders(nil, templateId, initial)
		require.NoError(t, err)

		// when - update only dueDate
		updated := []*model.Placeholder{
			{
				RelationKey: "dueDate",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.Int64(1704067200)},
				},
			},
		}
		err = fx.SetTemplatePlaceholders(nil, templateId, updated)
		require.NoError(t, err)

		// then
		retrieved, err := fx.GetTemplatePlaceholders(templateId)
		require.NoError(t, err)
		require.Len(t, retrieved, 2, "should still have both relations")

		relKeys := make(map[string]*model.Placeholder)
		for _, p := range retrieved {
			relKeys[p.RelationKey] = p
		}

		// dueDate should be updated
		assert.Equal(t, model.Placeholder_PlaceholderValue, relKeys["dueDate"].Values[0].Type)
		assert.Equal(t, float64(1704067200), relKeys["dueDate"].Values[0].Value.GetNumberValue())

		// assignee should remain unchanged
		assert.Equal(t, model.Placeholder_PlaceholderCurrentUser, relKeys["assignee"].Values[0].Type)
	})

	t.Run("remove placeholder by setting empty values", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl

		// Set initial placeholders
		initial := []*model.Placeholder{
			{
				RelationKey: "dueDate",
				Values:      []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderToday, Value: pbtypes.Null()}},
			},
			{
				RelationKey: "assignee",
				Values:      []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderCurrentUser, Value: pbtypes.Null()}},
			},
		}
		err := fx.SetTemplatePlaceholders(nil, templateId, initial)
		require.NoError(t, err)

		// when - remove dueDate by setting empty values
		remove := []*model.Placeholder{
			{
				RelationKey: "dueDate",
				Values:      []*model.PlaceholderValue{}, // empty values
			},
		}
		err = fx.SetTemplatePlaceholders(nil, templateId, remove)
		require.NoError(t, err)

		// then
		st := tmpl.NewState()
		placeholdersStruct := st.GetStoreStruct(placeholdersStoreKey)
		require.NotNil(t, placeholdersStruct)
		assert.Nil(t, placeholdersStruct.Fields["dueDate"], "dueDate should be removed from store")
		assert.NotNil(t, placeholdersStruct.Fields["assignee"], "assignee should still exist")

		retrieved, err := fx.GetTemplatePlaceholders(templateId)
		require.NoError(t, err)
		require.Len(t, retrieved, 1, "should only have assignee left")
		assert.Equal(t, "assignee", retrieved[0].RelationKey)
	})

	t.Run("get placeholders returns nil when none exist", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl

		// when
		retrieved, err := fx.GetTemplatePlaceholders(templateId)

		// then
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("setting same placeholder values is idempotent", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-tags"),
				bundle.RelationKeyRelationKey:    domain.String("tags"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
				bundle.RelationKeyUniqueKey:      domain.String("rel-tags"),
			},
		})
		placeholders := []*model.Placeholder{
			{
				RelationKey: "tags",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("tag1")},
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("tag2")},
				},
			},
		}

		// when - set twice
		err := fx.SetTemplatePlaceholders(nil, templateId, placeholders)
		require.NoError(t, err)
		err = fx.SetTemplatePlaceholders(nil, templateId, placeholders)
		require.NoError(t, err)

		// then
		retrieved, err := fx.GetTemplatePlaceholders(templateId)
		require.NoError(t, err)
		require.Len(t, retrieved, 1)
		assert.Equal(t, "tags", retrieved[0].RelationKey)
		require.Len(t, retrieved[0].Values, 2)
	})
}

func TestService_ValidatePlaceholderValue(t *testing.T) {
	const (
		templateId = "template1"
		spaceId    = "space1"
	)

	newTemplateSmartBlock := func(id string) smartblock.SmartBlock {
		sb := smarttest.New(id)
		sb.SetSpaceId(spaceId)
		sb.SetObjectTypes([]domain.TypeKey{bundle.TypeKeyTemplate, bundle.TypeKeyTask})
		return sb
	}

	t.Run("PlaceholderToday with null value - valid", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-dueDate"),
				bundle.RelationKeyRelationKey:    domain.String("dueDate"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
			},
		})

		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{
				RelationKey: "dueDate",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderToday, Value: pbtypes.Null()},
				},
			},
		})

		// then
		require.NoError(t, err)
	})

	t.Run("PlaceholderToday with non-null value - invalid", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-dueDate"),
				bundle.RelationKeyRelationKey:    domain.String("dueDate"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
			},
		})


		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{
				RelationKey: "dueDate",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderToday, Value: pbtypes.String("2024-01-01")},
				},
			},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "null value expected")
	})

	t.Run("PlaceholderCurrentUser with null value - valid", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-assignee"),
				bundle.RelationKeyRelationKey:    domain.String("assignee"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
			},
		})


		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{
				RelationKey: "assignee",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderCurrentUser, Value: pbtypes.Null()},
				},
			},
		})

		// then
		require.NoError(t, err)
	})

	t.Run("PlaceholderCurrentUser with non-null value - invalid", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-assignee"),
				bundle.RelationKeyRelationKey:    domain.String("assignee"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
			},
		})


		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{
				RelationKey: "assignee",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderCurrentUser, Value: pbtypes.String("user123")},
				},
			},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "null value expected")
	})

	t.Run("PlaceholderValue with string for tag relation - valid", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-tags"),
				bundle.RelationKeyRelationKey:    domain.String("tags"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
				bundle.RelationKeyUniqueKey:      domain.String("rel-tags"),
			},
		})


		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{
				RelationKey: "tags",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("tag1")},
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("tag2")},
				},
			},
		})

		// then
		require.NoError(t, err)
	})

	t.Run("PlaceholderValue with number for tag relation - invalid", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-tags"),
				bundle.RelationKeyRelationKey:    domain.String("tags"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
				bundle.RelationKeyUniqueKey:      domain.String("rel-tags"),
			},
		})


		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{
				RelationKey: "tags",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.Int64(123)},
				},
			},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "string expected")
	})

	t.Run("PlaceholderValue with string for status relation - valid", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-status"),
				bundle.RelationKeyRelationKey:    domain.String("status"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
				bundle.RelationKeyUniqueKey:      domain.String("rel-status"),
			},
		})


		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{
				RelationKey: "status",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("status-done")},
				},
			},
		})

		// then
		require.NoError(t, err)
	})

	t.Run("PlaceholderValue with string for object relation - valid", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-assignee"),
				bundle.RelationKeyRelationKey:    domain.String("assignee"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
				bundle.RelationKeyUniqueKey:      domain.String("rel-assignee"),
			},
		})


		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{
				RelationKey: "assignee",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("user-id-123")},
				},
			},
		})

		// then
		require.NoError(t, err)
	})

	t.Run("PlaceholderValue with string for file relation - valid", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-cover"),
				bundle.RelationKeyRelationKey:    domain.String("cover"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_file)),
				bundle.RelationKeyUniqueKey:      domain.String("rel-cover"),
			},
		})


		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{
				RelationKey: "cover",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("file-id-456")},
				},
			},
		})

		// then
		require.NoError(t, err)
	})

	t.Run("PlaceholderValue with number for date relation - valid", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-dueDate"),
				bundle.RelationKeyRelationKey:    domain.String("dueDate"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
				bundle.RelationKeyUniqueKey:      domain.String("rel-dueDate"),
			},
		})


		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{
				RelationKey: "dueDate",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.Int64(1704067200)},
				},
			},
		})

		// then
		require.NoError(t, err)
	})

	t.Run("PlaceholderValue with string for date relation - invalid", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-dueDate"),
				bundle.RelationKeyRelationKey:    domain.String("dueDate"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
				bundle.RelationKeyUniqueKey:      domain.String("rel-dueDate"),
			},
		})


		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{
				RelationKey: "dueDate",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("2024-01-01")},
				},
			},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "number expected")
	})

	t.Run("PlaceholderValue with unsupported relation format - invalid", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-description"),
				bundle.RelationKeyRelationKey:    domain.String("description"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_longtext)),
				bundle.RelationKeyUniqueKey:      domain.String("rel-description"),
			},
		})


		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{
				RelationKey: "description",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("some text")},
				},
			},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported format")
	})

	t.Run("mixed valid and invalid placeholders - all invalid", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-tags"),
				bundle.RelationKeyRelationKey:    domain.String("tags"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
				bundle.RelationKeyUniqueKey:      domain.String("rel-tags"),
			},
			{
				bundle.RelationKeyId:             domain.String("rel-dueDate"),
				bundle.RelationKeyRelationKey:    domain.String("dueDate"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
				bundle.RelationKeyUniqueKey:      domain.String("rel-dueDate"),
			},
		})


		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{
				RelationKey: "tags",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("tag1")}, // valid
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.Int64(123)},     // invalid
				},
			},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "string expected")
	})

	t.Run("relation does not exist - error", func(t *testing.T) {
		// given
		tmpl := newTemplateSmartBlock(templateId)
		fx := newFixture(t)
		fx.picker.objects[templateId] = tmpl

		// when
		err := fx.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{
				RelationKey: "nonExistentRelation",
				Values: []*model.PlaceholderValue{
					{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("value")},
				},
			},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get relation format")
	})
}
