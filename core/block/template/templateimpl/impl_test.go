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
		s := service{picker: &testPicker{sb: tmpl}}

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
				s := service{picker: &testPicker{sb: tmpl}, converter: converter.NewLayoutConverter()}
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
			s := service{picker: &testPicker{sb: tmpl}, converter: converter.NewLayoutConverter()}

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
		s := service{picker: &testPicker{sb: tmpl}}

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

		s := service{picker: &testPicker{tmpl}}

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

	t.Run("keeps empty name", func(t *testing.T) {
		// given
		s := service{}
		st := state.NewDoc("test", nil).NewState()
		st.SetDetail(bundle.RelationKeyName, domain.String(""))

		// when
		result := s.collectOriginalDetails(spaceId, st)

		// then
		assert.True(t, result.Has(bundle.RelationKeyName))
	})
}
