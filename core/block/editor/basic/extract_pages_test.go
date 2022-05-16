package basic

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
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

func (t testExtractPages) CreatePageFromState(ctx *state.Context, _ smartblock.SmartBlock, _ string, req pb.RpcBlockCreatePageRequest, state *state.State) (linkId string, pageId string, err error) {
	id := bson.NewObjectId().Hex()
	page := smarttest.New(id)
	t.pages[id] = page

	state.SetRootId(id)
	page.Doc = state

	return "", id, nil
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
		sb.AddBlock(newTextBlock("test", "", []string{"1", "2", "3"}))
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
		name               string
		blockIds           []string
		wantPagesWithTexts [][]string
	}{
		{
			name:               "undefined block",
			blockIds:           []string{"4.1.1"},
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
			name: "two blocks, all descendants present in requests",
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
		{
			name: "two blocks, not all descendants present in requests",
			blockIds: []string{
				"1.1", "1.1.1",
				"3", "3.1.1",
			},
			wantPagesWithTexts: [][]string{
				// First page
				{
					"text 1.1",
					"text 1.1.1",
				},
				// Second page
				{
					"text 3",
					"text 3.1",
					"text 3.1.1",
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
			ctx := state.NewContext(nil)
			linkIds, err := NewBasic(sb).ExtractBlocksToPages(ctx, ts, req)
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
