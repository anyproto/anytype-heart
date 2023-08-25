package basic

import (
	"context"
	"testing"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relation/mock_relation"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
	"github.com/anyproto/anytype-heart/util/testMock"
)

type testExtractObjects struct {
	objects map[string]*smarttest.SmartTest
}

func (t testExtractObjects) Add(object *smarttest.SmartTest) {
	t.objects[object.Id()] = object
}

func (t testExtractObjects) CreateSmartBlockFromState(ctx context.Context, spaceID string, sbType coresb.SmartBlockType, _ []bundle.TypeKey, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error) {
	id = bson.NewObjectId().Hex()
	object := smarttest.New(id)
	t.objects[id] = object

	createState.SetRootId(id)
	object.Doc = createState

	return id, nil, nil
}

func (t testExtractObjects) InjectWorkspaceID(details *types.Struct, spaceID string, objectID string) {
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

func assertLinkedObjectHasTextBlocks(t *testing.T, ts testExtractObjects, sourceObject *smarttest.SmartTest, linkId string, texts []string) {
	b := sourceObject.Pick(linkId).Model()

	link := b.GetLink()
	require.NotNil(t, link)

	object := ts.objects[link.TargetBlockId]
	require.NotNil(t, object)

	assertHasTextBlocks(t, object, texts)
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

	for _, tc := range []struct {
		name                 string
		blockIds             []string
		wantObjectsWithTexts [][]string
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newFixture(t)
			defer fixture.cleanUp()

			ts := testExtractObjects{
				objects: map[string]*smarttest.SmartTest{},
			}

			sb := makeTestObject()
			ts.Add(sb)

			req := pb.RpcBlockListConvertToObjectsRequest{
				ContextId:           "test",
				BlockIds:            tc.blockIds,
				ObjectTypeUniqueKey: domain.MustUniqueKey(model.SmartBlockType_STType, bundle.TypeKeyNote.String()).Marshal(),
			}
			ctx := session.NewContext()
			linkIds, err := NewBasic(sb, fixture.store, fixture.relationService, converter.NewLayoutConverter()).ExtractBlocksToObjects(ctx, ts, req)
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
				assertLinkedObjectHasTextBlocks(t, ts, sb, linkIds[i], wantTexts)
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
	t               *testing.T
	ctrl            *gomock.Controller
	store           *testMock.MockObjectStore
	relationService *mock_relation.MockService
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

	relationService := mock_relation.NewMockService(t)
	relationService.EXPECT().GetObjectByUniqueKey(mock.Anything, mock.Anything).Return(objectTypeDetails, nil).Maybe()

	return &fixture{
		t:               t,
		ctrl:            ctrl,
		store:           objectStore,
		relationService: relationService,
	}
}

func (fx *fixture) cleanUp() {
	fx.ctrl.Finish()
}
