package basic

import (
	"context"
	"testing"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
	"github.com/anyproto/anytype-heart/util/testMock"
)

type testCreator struct {
	objects map[string]*smarttest.SmartTest
}

func (tc testCreator) Add(object *smarttest.SmartTest) {
	tc.objects[object.Id()] = object
}

func (tc testCreator) CreateSmartBlockFromState(_ context.Context, _ string, _ []domain.TypeKey, createState *state.State) (id string, newDetails *types.Struct, err error) {
	id = bson.NewObjectId().Hex()
	object := smarttest.New(id)
	tc.objects[id] = object

	createState.SetRootId(id)
	object.Doc = createState

	return id, nil, nil
}

type testTemplateService struct {
	templates map[string]*state.State
}

func (tts testTemplateService) AddTemplate(id string, st *state.State) {
	tts.templates[id] = st
}

func (tts testTemplateService) CreateTemplateStateWithDetails(id string, details *types.Struct) (*state.State, error) {
	if id == "" {
		st := state.NewDoc("", nil).NewState()
		template.InitTemplate(st, template.WithEmpty,
			template.WithDefaultFeaturedRelations,
			template.WithFeaturedRelations,
			template.WithRequiredRelations(),
			template.WithTitle,
		)
		return st, nil
	}
	st := tts.templates[id]
	templateDetails := st.Details()
	newDetails := pbtypes.StructMerge(templateDetails, details, false)
	st.SetDetails(newDetails)
	return st, nil
}

func assertNoCommonElements(t *testing.T, a, b []string) {
	got := slice.Difference(a, b)

	assert.Equal(t, got, a)
}

func assertHasTextBlocks(t *testing.T, object *smarttest.SmartTest, texts []string) {
	var gotTexts []string

	for _, b := range object.Blocks() {
		if b.GetText() != nil {
			gotTexts = append(gotTexts, b.GetText().Text)
		}
	}

	assert.Subset(t, gotTexts, texts)
}

func assertLinkedObjectHasTextBlocks(t *testing.T, ts testCreator, sourceObject *smarttest.SmartTest, linkId string, texts []string) {
	b := sourceObject.Pick(linkId).Model()

	link := b.GetLink()
	require.NotNil(t, link)

	object := ts.objects[link.TargetBlockId]
	require.NotNil(t, object)

	assertHasTextBlocks(t, object, texts)
}

func assertDetails(t *testing.T, id string, ts testCreator, details *types.Struct) {
	object, ok := ts.objects[id]
	if !ok {
		return
	}
	objDetails := object.Details()
	for key, value := range details.Fields {
		assert.Equal(t, value, objDetails.Fields[key])
	}
}

func TestExtractObjects(t *testing.T) {
	makeTestObject := func() *smarttest.SmartTest {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2", "3"}}))
		sb.AddBlock(newTextBlock("1", "text 1", []string{"1.1", "1.2"}))
		sb.AddBlock(newTextBlock("1.1", "text 1.1", []string{"1.1.1"}))
		sb.AddBlock(newTextBlock("1.1.1", "text 1.1.1", nil))
		sb.AddBlock(newTextBlock("1.2", "text 1.2", nil))
		sb.AddBlock(newTextBlock("2", "text 2", []string{"2.1"}))
		sb.AddBlock(newTextBlock("2.1", "text 2.1", nil))
		sb.AddBlock(newTextBlock("3", "text 3", []string{"3.1"}))
		sb.AddBlock(newTextBlock("3.1", "text 3.1", []string{"3.1.1"}))
		sb.AddBlock(newTextBlock("3.1.1", "text 3.1.1", nil))
		return sb
	}

	templateDetails := []*model.Detail{
		{Key: bundle.RelationKeyName.String(), Value: pbtypes.String("template")},
		{Key: bundle.RelationKeyIconImage.String(), Value: pbtypes.String("very funny img")},
		{Key: bundle.RelationKeyFeaturedRelations.String(), Value: pbtypes.StringList([]string{"tag", "type", "status"})},
		{Key: bundle.RelationKeyCoverId.String(), Value: pbtypes.String("poster with Van Damme")},
	}

	makeTemplateState := func() *state.State {
		sb := smarttest.New("template")
		sb.AddBlock(simple.New(&model.Block{Id: "template", ChildrenIds: []string{"A", "B"}}))
		sb.AddBlock(newTextBlock("A", "text A", nil))
		sb.AddBlock(newTextBlock("B", "text B", []string{"B.1"}))
		sb.AddBlock(newTextBlock("B.1", "text B.1", nil))
		err := sb.SetDetails(nil, templateDetails, false)
		require.NoError(t, err)
		return sb.NewState()
	}

	for _, tc := range []struct {
		name                 string
		blockIds             []string
		templateId           string
		wantObjectsWithTexts [][]string
		wantDetails          *types.Struct
	}{
		{
			name:                 "undefined block",
			blockIds:             []string{"4.1.1"},
			wantObjectsWithTexts: [][]string{},
		},
		{
			name:     "leaf block",
			blockIds: []string{"1.1.1"},
			wantObjectsWithTexts: [][]string{
				{"text 1.1.1"},
			},
		},
		{
			name:     "block with one child",
			blockIds: []string{"2"},
			wantObjectsWithTexts: [][]string{
				{"text 2", "text 2.1"},
			},
		},
		{
			name:     "block with one child, child id also presents in request",
			blockIds: []string{"2", "2.1"},
			wantObjectsWithTexts: [][]string{
				{"text 2", "text 2.1"},
			},
		},
		{
			name:     "block with multiple children",
			blockIds: []string{"1"},
			wantObjectsWithTexts: [][]string{
				{
					"text 1",
					"text 1.1", "text 1.1.1",
					"text 1.2",
				},
			},
		},
		{
			name: "two blocks, all descendants present in requests",
			blockIds: []string{
				"1", "1.1", "1.1.1", "1.2",
				"2", "2.1",
			},
			wantObjectsWithTexts: [][]string{
				// First object
				{
					"text 1",
					"text 1.1", "text 1.1.1",
					"text 1.2",
				},
				// Second object
				{
					"text 2",
					"text 2.1",
				},
			},
			wantDetails: &types.Struct{},
		},
		{
			name: "two blocks, not all descendants present in requests",
			blockIds: []string{
				"1.1", "1.1.1",
				"3", "3.1.1",
			},
			wantObjectsWithTexts: [][]string{
				// First object
				{
					"text 1.1",
					"text 1.1.1",
				},
				// Second object
				{
					"text 3",
					"text 3.1",
					"text 3.1.1",
				},
			},
		},
		{
			name:       "block with children, from template",
			blockIds:   []string{"3"},
			templateId: "template",
			wantObjectsWithTexts: [][]string{
				{
					"text A", "text B", "text B.1",
					"text 3", "text 3.1", "text 3.1.1",
				},
			},
			wantDetails: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyName.String():              pbtypes.String("text 3"),
				bundle.RelationKeyIconImage.String():         pbtypes.String("very funny img"),
				bundle.RelationKeyFeaturedRelations.String(): pbtypes.StringList([]string{"tag", "type", "status"}),
				bundle.RelationKeyCoverId.String():           pbtypes.String("poster with Van Damme"),
			}},
		},
		{
			name:       "two blocks with children, from template",
			blockIds:   []string{"2", "3"},
			templateId: "template",
			wantObjectsWithTexts: [][]string{
				// first object
				{
					"text A", "text B", "text B.1",
					"text 2", "text 2.1",
				},
				// second object
				{
					"text A", "text B", "text B.1",
					"text 3", "text 3.1", "text 3.1.1",
				},
			},
			wantDetails: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyIconImage.String():         pbtypes.String("very funny img"),
				bundle.RelationKeyFeaturedRelations.String(): pbtypes.StringList([]string{"tag", "type", "status"}),
				bundle.RelationKeyCoverId.String():           pbtypes.String("poster with Van Damme"),
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newFixture(t)
			defer fixture.cleanUp()

			creator := testCreator{objects: map[string]*smarttest.SmartTest{}}
			sb := makeTestObject()
			creator.Add(sb)

			ts := testTemplateService{templates: map[string]*state.State{}}
			tmpl := makeTemplateState()
			ts.AddTemplate("template", tmpl)

			req := pb.RpcBlockListConvertToObjectsRequest{
				ContextId:           "test",
				BlockIds:            tc.blockIds,
				TemplateId:          tc.templateId,
				ObjectTypeUniqueKey: domain.MustUniqueKey(coresb.SmartBlockTypeObjectType, bundle.TypeKeyNote.String()).Marshal(),
			}
			ctx := session.NewContext()
			linkIds, err := NewBasic(sb, fixture.store, converter.NewLayoutConverter()).ExtractBlocksToObjects(ctx, creator, ts, req)
			assert.NoError(t, err)

			var gotBlockIds []string
			for _, b := range sb.Blocks() {
				gotBlockIds = append(gotBlockIds, b.Id)
			}

			// Check that requested blocks are removed from object
			assertNoCommonElements(t, gotBlockIds, req.BlockIds)

			// Check that linked objects has desired text blocks
			require.Len(t, linkIds, len(tc.wantObjectsWithTexts))
			for i, wantTexts := range tc.wantObjectsWithTexts {
				assertLinkedObjectHasTextBlocks(t, creator, sb, linkIds[i], wantTexts)
				if tc.wantDetails != nil && tc.wantDetails.Fields != nil {
					assertDetails(t, linkIds[i], creator, tc.wantDetails)
				}
			}
		})
	}

	t.Run("do not add relation name - when creating note", func(t *testing.T) {
		fields := createTargetObjectDetails("whatever name", model.ObjectType_note).Fields

		assert.NotContains(t, fields, bundle.RelationKeyName.String())
	})

	t.Run("add relation name - when creating not note", func(t *testing.T) {
		fields := createTargetObjectDetails("whatever name", model.ObjectType_basic).Fields

		assert.Contains(t, fields, bundle.RelationKeyName.String())
	})
}

type fixture struct {
	t     *testing.T
	ctrl  *gomock.Controller
	store *testMock.MockObjectStore
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	objectStore := testMock.NewMockObjectStore(ctrl)

	objectTypeDetails := &model.ObjectDetails{
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyLayout.String(): pbtypes.String(model.ObjectType_basic.String()),
			},
		},
	}
	objectStore.EXPECT().GetObjectByUniqueKey(gomock.Any(), gomock.Any()).Return(objectTypeDetails, nil).AnyTimes()

	return &fixture{
		t:     t,
		ctrl:  ctrl,
		store: objectStore,
	}
}

func (fx *fixture) cleanUp() {
	fx.ctrl.Finish()
}
