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
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/table"
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

func (tts testTemplateService) CreateTemplateStateWithDetails(id string, details *types.Struct) (st *state.State, err error) {
	if id == "" {
		st = state.NewDoc("", nil).NewState()
		template.InitTemplate(st, template.WithEmpty,
			template.WithDefaultFeaturedRelations,
			template.WithFeaturedRelations,
			template.WithRequiredRelations,
			template.WithTitle,
		)
	} else {
		st = tts.templates[id]
	}
	templateDetails := st.Details()
	newDetails := pbtypes.StructMerge(templateDetails, details, false)
	st.SetDetails(newDetails)
	return st, nil
}

func (tts testTemplateService) CreateTemplateStateFromSmartBlock(sb smartblock.SmartBlock, details *types.Struct) *state.State {
	return tts.templates[sb.Id()]
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
	objectId := "test"
	makeTestObject := func() *smarttest.SmartTest {
		sb := smarttest.New(objectId)
		sb.AddBlock(simple.New(&model.Block{Id: objectId, ChildrenIds: []string{"1", "2", "3"}}))
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

	makeTemplateState := func(id string) *state.State {
		sb := smarttest.New(id)
		sb.AddBlock(simple.New(&model.Block{Id: id, ChildrenIds: []string{"A", "B"}}))
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
		typeKey              string
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
		{
			name:                 "if target layout includes title, root is not added",
			blockIds:             []string{"1.1"},
			typeKey:              bundle.TypeKeyTask.String(),
			wantObjectsWithTexts: [][]string{{"text 1.1.1"}},
			wantDetails: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyName.String(): pbtypes.String("1.1"),
			}},
		},
		{
			name:                 "template and source are the same objects",
			blockIds:             []string{"1.1"},
			typeKey:              bundle.TypeKeyTask.String(),
			templateId:           objectId,
			wantObjectsWithTexts: [][]string{{"text 1.1.1", "text 2.1", "text 3.1"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newFixture(t)
			defer fixture.cleanUp()

			creator := testCreator{objects: map[string]*smarttest.SmartTest{}}
			sb := makeTestObject()
			creator.Add(sb)

			ts := testTemplateService{templates: map[string]*state.State{}}
			var tmpl *state.State
			if tc.templateId == objectId {
				tmpl = sb.NewState()
			} else {
				tmpl = makeTemplateState(tc.templateId)
			}
			ts.AddTemplate(tc.templateId, tmpl)

			if tc.typeKey == "" {
				tc.typeKey = bundle.TypeKeyNote.String()
			}

			req := pb.RpcBlockListConvertToObjectsRequest{
				ContextId:           "test",
				BlockIds:            tc.blockIds,
				TemplateId:          tc.templateId,
				ObjectTypeUniqueKey: domain.MustUniqueKey(coresb.SmartBlockTypeObjectType, tc.typeKey).Marshal(),
			}
			ctx := session.NewContext()
			linkIds, err := NewBasic(sb, fixture.store, converter.NewLayoutConverter(), nil, nil).ExtractBlocksToObjects(ctx, creator, ts, req)
			assert.NoError(t, err)

			gotBlockIds := []string{}
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
	t.Run("add custom link block", func(t *testing.T) {
		fixture := newFixture(t)
		defer fixture.cleanUp()
		creator := testCreator{objects: map[string]*smarttest.SmartTest{}}
		sb := makeTestObject()
		creator.Add(sb)

		ts := testTemplateService{templates: map[string]*state.State{}}
		tmpl := makeTemplateState("template")
		ts.AddTemplate("template", tmpl)

		req := pb.RpcBlockListConvertToObjectsRequest{
			ContextId:           "test",
			BlockIds:            []string{"1"},
			ObjectTypeUniqueKey: domain.MustUniqueKey(coresb.SmartBlockTypeObjectType, bundle.TypeKeyNote.String()).Marshal(),
			Block: &model.Block{Id: "newId", Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					CardStyle: model.BlockContentLink_Card,
				},
			}},
		}
		ctx := session.NewContext()
		_, err := NewBasic(sb, fixture.store, converter.NewLayoutConverter(), nil, nil).ExtractBlocksToObjects(ctx, creator, ts, req)
		assert.NoError(t, err)
		var block *model.Block
		for _, block = range sb.Blocks() {
			if block.GetLink() != nil {
				break
			}
		}
		assert.NotNil(t, block)
		assert.Equal(t, block.GetLink().GetCardStyle(), model.BlockContentLink_Card)
	})
	t.Run("add custom link block for multiple blocks", func(t *testing.T) {
		fixture := newFixture(t)
		defer fixture.cleanUp()
		creator := testCreator{objects: map[string]*smarttest.SmartTest{}}
		sb := makeTestObject()
		creator.Add(sb)

		ts := testTemplateService{templates: map[string]*state.State{}}
		tmpl := makeTemplateState("template")
		ts.AddTemplate("template", tmpl)

		req := pb.RpcBlockListConvertToObjectsRequest{
			ContextId:           "test",
			BlockIds:            []string{"1", "2"},
			ObjectTypeUniqueKey: domain.MustUniqueKey(coresb.SmartBlockTypeObjectType, bundle.TypeKeyNote.String()).Marshal(),
			Block: &model.Block{Id: "newId", Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					CardStyle: model.BlockContentLink_Card,
				},
			}},
		}
		ctx := session.NewContext()
		_, err := NewBasic(sb, fixture.store, converter.NewLayoutConverter(), nil, nil).ExtractBlocksToObjects(ctx, creator, ts, req)
		assert.NoError(t, err)
		var addedBlocks []*model.Block
		for _, message := range sb.Results.Events {
			for _, eventMessage := range message {
				if blockAdd := eventMessage.Msg.GetBlockAdd(); blockAdd != nil {
					addedBlocks = append(addedBlocks, blockAdd.Blocks...)
				}
			}
		}
		assert.Len(t, addedBlocks, 2)
		assert.NotEqual(t, addedBlocks[0].Id, addedBlocks[1].Id)
		assert.NotEqual(t, addedBlocks[0].GetLink().GetTargetBlockId(), addedBlocks[1].GetLink().GetTargetBlockId())
	})
}

func TestBuildBlock(t *testing.T) {
	const target = "target"

	for _, tc := range []struct {
		name          string
		input, output *model.Block
	}{
		{
			name:  "nil",
			input: nil,
			output: &model.Block{Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
				TargetBlockId: target,
				Style:         model.BlockContentLink_Page,
			}}},
		},
		{
			name: "link",
			input: &model.Block{Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
				Style:     model.BlockContentLink_Dashboard,
				CardStyle: model.BlockContentLink_Card,
			}}},
			output: &model.Block{Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
				TargetBlockId: target,
				Style:         model.BlockContentLink_Dashboard,
				CardStyle:     model.BlockContentLink_Card,
			}}},
		},
		{
			name: "bookmark",
			input: &model.Block{Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{
				Type:  model.LinkPreview_Image,
				State: model.BlockContentBookmark_Fetching,
			}}},
			output: &model.Block{Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{
				TargetObjectId: target,
				Type:           model.LinkPreview_Image,
				State:          model.BlockContentBookmark_Fetching,
			}}},
		},
		{
			name: "file",
			input: &model.Block{Content: &model.BlockContentOfFile{File: &model.BlockContentFile{
				Type: model.BlockContentFile_Image,
			}}},
			output: &model.Block{Content: &model.BlockContentOfFile{File: &model.BlockContentFile{
				TargetObjectId: target,
				Type:           model.BlockContentFile_Image,
			}}},
		},
		{
			name: "dataview",
			input: &model.Block{Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				IsCollection: true,
				Source:       []string{"ot-note"},
			}}},
			output: &model.Block{Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				TargetObjectId: target,
				IsCollection:   true,
				Source:         []string{"ot-note"},
			}}},
		},
		{
			name: "other",
			input: &model.Block{Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{
				IsHeader: true,
			}}},
			output: &model.Block{Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
				TargetBlockId: target,
				Style:         model.BlockContentLink_Page,
			}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.output, buildBlock(tc.input, target))
		})
	}
}

func TestReassignSubtreeIds(t *testing.T) {
	t.Run("plain blocks receive new ids", func(t *testing.T) {
		// given
		blocks := []simple.Block{
			simple.New(&model.Block{Id: "text", ChildrenIds: []string{"1", "2"}}),
			simple.New(&model.Block{Id: "1", ChildrenIds: []string{"1.1"}}),
			simple.New(&model.Block{Id: "2"}),
			simple.New(&model.Block{Id: "1.1"}),
		}
		s := generateState("text", blocks)

		// when
		newRoot, newBlocks := copySubtreeOfBlocks(s, "text", blocks)

		// then
		assert.Len(t, newBlocks, len(blocks))
		assert.NotEqual(t, "text", newRoot)
		for i := 0; i < len(blocks); i++ {
			assert.NotEqual(t, blocks[i].Model().Id, newBlocks[i].Model().Id)
			assert.True(t, bson.IsObjectIdHex(newBlocks[i].Model().Id))
		}
	})

	t.Run("table blocks receive new ids", func(t *testing.T) {
		// given
		blocks := []simple.Block{
			simple.New(&model.Block{Id: "parent", ChildrenIds: []string{"table"}}),
			simple.New(&model.Block{Id: "table", ChildrenIds: []string{"cols", "rows"}, Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}}),
			simple.New(&model.Block{Id: "cols", ChildrenIds: []string{"col1", "col2"}, Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}}}),
			simple.New(&model.Block{Id: "col1", Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}}),
			simple.New(&model.Block{Id: "col2", Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}}),
			simple.New(&model.Block{Id: "rows", ChildrenIds: []string{"row1", "row2"}, Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}}}),
			simple.New(&model.Block{Id: "row1", ChildrenIds: []string{"row1-col1", "row1-col2"}, Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}}),
			simple.New(&model.Block{Id: "row2", ChildrenIds: []string{"row2-col1", "row2-col2"}, Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}}),
			simple.New(&model.Block{Id: "row1-col1"}),
			simple.New(&model.Block{Id: "row1-col2"}),
			simple.New(&model.Block{Id: "row2-col1"}),
			simple.New(&model.Block{Id: "row2-col2"}),
		}
		s := generateState("parent", blocks)

		// when
		root, newBlocks := copySubtreeOfBlocks(s, "parent", blocks)

		// then
		assert.Len(t, newBlocks, len(blocks))
		assert.NotEqual(t, "text", root)

		blocksMap := make(map[string]simple.Block, len(newBlocks))
		tableId := ""
		for i := 0; i < len(blocks); i++ {
			nb := newBlocks[i]
			assert.NotEqual(t, blocks[i].Model().Id, nb.Model().Id)
			blocksMap[nb.Model().Id] = nb
			if tb := nb.Model().GetTable(); tb != nil {
				tableId = nb.Model().Id
			}
		}
		require.NotEmpty(t, tableId)

		newState := state.NewDoc("new", blocksMap).NewState()
		tbl, err := table.NewTable(newState, tableId)

		assert.NoError(t, err)

		rows := tbl.RowIDs()
		cols := tbl.ColumnIDs()
		require.NoError(t, tbl.Iterate(func(b simple.Block, pos table.CellPosition) bool {
			assert.Equal(t, pos.RowID, rows[pos.RowNumber])
			assert.Equal(t, pos.ColID, cols[pos.ColNumber])
			return true
		}))
	})

	t.Run("table blocks receive plain ids in case of error on dup", func(t *testing.T) {
		// given
		blocks := []simple.Block{
			simple.New(&model.Block{Id: "parent", ChildrenIds: []string{"table"}}),
			simple.New(&model.Block{Id: "table", ChildrenIds: []string{"cols", "rows"}, Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}}),
			simple.New(&model.Block{Id: "rows", ChildrenIds: []string{}, Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}}}),
		}
		s := generateState("parent", blocks)

		// when
		root, newBlocks := copySubtreeOfBlocks(s, "parent", blocks)

		// then
		assert.Len(t, newBlocks, len(blocks))
		assert.NotEqual(t, "text", root)
		for i := 0; i < len(blocks); i++ {
			assert.NotEqual(t, blocks[i].Model().Id, newBlocks[i].Model().Id)
			assert.True(t, bson.IsObjectIdHex(newBlocks[i].Model().Id))
		}
	})
}

func generateState(root string, blocks []simple.Block) *state.State {
	mapping := make(map[string]simple.Block, len(blocks))

	for _, b := range blocks {
		mapping[b.Model().Id] = b
	}

	s := state.NewDoc(root, mapping).NewState()
	s.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{root}}))
	return s
}

type fixture struct {
	t     *testing.T
	ctrl  *gomock.Controller
	store *testMock.MockObjectStore
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	objectStore := testMock.NewMockObjectStore(ctrl)

	objectStore.EXPECT().GetObjectByUniqueKey(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ string, uk domain.UniqueKey) (*model.ObjectDetails, error) {
			layout := pbtypes.Int64(int64(model.ObjectType_basic))
			switch uk.InternalKey() {
			case "note":
				layout = pbtypes.Int64(int64(model.ObjectType_note))
			case "task":
				layout = pbtypes.Int64(int64(model.ObjectType_todo))
			}
			return &model.ObjectDetails{
				Details: &types.Struct{
					Fields: map[string]*types.Value{
						bundle.RelationKeyRecommendedLayout.String(): layout,
					},
				},
			}, nil
		}).AnyTimes()

	return &fixture{
		t:     t,
		ctrl:  ctrl,
		store: objectStore,
	}
}

func (fx *fixture) cleanUp() {
	fx.ctrl.Finish()
}
