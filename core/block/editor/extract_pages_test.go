package editor

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testExtractPages struct {
	pages map[string]*smarttest.SmartTest
}

func (t testExtractPages) Add(page *smarttest.SmartTest) {
	t.pages[page.Id()] = page
}

func (t testExtractPages) Do(id string, apply func(b smartblock.SmartBlock) error) error {
	return apply(t.pages[id])
}

func (t testExtractPages) CreatePageFromState(ctx *state.Context, groupId string, req pb.RpcBlockCreatePageRequest, state *state.State) (linkId string, pageId string, err error) {
	id := bson.NewObjectId().Hex()
	page := smarttest.New(id)
	t.pages[id] = page

	state.SetRootId(id)
	page.Doc = state

	return "", id, nil
}

func newTextBlock(id string, childrenIds []string, text string) simple.Block {
	return simple.New(&model.Block{Id: id, ChildrenIds: childrenIds, Content: &model.BlockContentOfText{
		Text: &model.BlockContentText{
			Text: text,
		},
	}})
}

func assertNoCommonElements(t *testing.T, a, b []string) {
	got := slice.Difference(a, b)

	assert.Equal(t, got, a)
}

func assertHasTextBlocks(t *testing.T, page *smarttest.SmartTest, texts []string) {
	var gotTexts []string

	for _, b := range page.Blocks() {
		if b.GetText() != nil {
			gotTexts = append(gotTexts, b.GetText().Text)
		}
	}

	assert.Subset(t, gotTexts, texts)
}

func assertLinkedPageHasTextBlocks(t *testing.T, ts testExtractPages, sourcePage *smarttest.SmartTest, linkId string, texts []string) {
	b := sourcePage.Pick(linkId).Model()

	link := b.GetLink()
	require.NotNil(t, link)

	page := ts.pages[link.TargetBlockId]
	require.NotNil(t, page)

	assertHasTextBlocks(t, page, texts)
}

func TestExtractPages(t *testing.T) {
	makeTestPage := func() *smarttest.SmartTest {
		sb := smarttest.New("test")
		sb.AddBlock(newTextBlock("test", []string{"1", "2"}, ""))
		sb.AddBlock(newTextBlock("1", []string{"1.1", "1.2"}, "text 1"))
		sb.AddBlock(newTextBlock("1.1", []string{"1.1.1"}, "text 1.1"))
		sb.AddBlock(newTextBlock("1.1.1", nil, "text 1.1.1"))
		sb.AddBlock(newTextBlock("1.2", nil, "text 1.2"))
		sb.AddBlock(newTextBlock("2", []string{"2.1"}, "text 2"))
		sb.AddBlock(newTextBlock("2.1", nil, "text 2.1"))
		return sb
	}

	for _, tc := range []struct {
		name               string
		blockIds           []string
		wantPagesWithTexts [][]string
	}{
		{
			name:               "undefined block",
			blockIds:           []string{"3.1.1"},
			wantPagesWithTexts: [][]string{},
		},
		{
			name:     "leaf block",
			blockIds: []string{"1.1.1"},
			wantPagesWithTexts: [][]string{
				{"text 1.1.1"},
			},
		},
		{
			name:     "block with one child",
			blockIds: []string{"2"},
			wantPagesWithTexts: [][]string{
				{"text 2", "text 2.1"},
			},
		},
		{
			name:     "block with one child, child id also presents in request",
			blockIds: []string{"2", "2.1"},
			wantPagesWithTexts: [][]string{
				{"text 2", "text 2.1"},
			},
		},
		{
			name:     "block with multiple children",
			blockIds: []string{"1"},
			wantPagesWithTexts: [][]string{
				{
					"text 1",
					"text 1.1", "text 1.1.1",
					"text 1.2",
				},
			},
		},
		{
			name: "multiple blocks, all descendants present in requests",
			blockIds: []string{
				"1", "1.1", "1.1.1", "1.2",
				"2", "2.1",
			},
			wantPagesWithTexts: [][]string{
				// First page
				{
					"text 1",
					"text 1.1", "text 1.1.1",
					"text 1.2",
				},
				// Second page
				{
					"text 2",
					"text 2.1",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := testExtractPages{
				pages: map[string]*smarttest.SmartTest{},
			}

			sb := makeTestPage()
			ts.Add(sb)

			req := pb.RpcBlockListConvertChildrenToPagesRequest{
				ContextId:  "test",
				BlockIds:   tc.blockIds,
				ObjectType: "page",
			}
			linkIds, err := ExtractBlocksToPages(ts, req)
			assert.NoError(t, err)

			var gotBlockIds []string
			for _, b := range sb.Blocks() {
				gotBlockIds = append(gotBlockIds, b.Id)
			}

			// Check that requested blocks are removed from page
			assertNoCommonElements(t, gotBlockIds, req.BlockIds)

			// Check that linked pages has desired text blocks
			require.Len(t, linkIds, len(tc.wantPagesWithTexts))
			for i, wantTexts := range tc.wantPagesWithTexts {
				assertLinkedPageHasTextBlocks(t, ts, sb, linkIds[i], wantTexts)
			}

		})
	}
}
