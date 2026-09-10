package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/publish"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestSingleFileHtmlBuilder_BuildSPA(t *testing.T) {
	tempDir := t.TempDir()
	objectsDir := filepath.Join(tempDir, "objects")
	filesDir := filepath.Join(tempDir, "files")
	require.NoError(t, os.MkdirAll(objectsDir, 0755))
	require.NoError(t, os.MkdirAll(filesDir, 0755))

	rootPageID := "root_page_1"
	subPageID := "sub_page_2"

	// 1. Create root page snapshot
	rootSnap := &pb.SnapshotWithType{
		SbType: model.SmartBlockType_Page,
		Snapshot: &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{
					Fields: map[string]*types.Value{
						"name": {Kind: &types.Value_StringValue{StringValue: "Root Welcome Page"}},
					},
				},
				Blocks: []*model.Block{
					{
						Id:          "root_block",
						Content:     &model.BlockContentOfSmartblock{},
						ChildrenIds: []string{"header_block", "text_block", "callout_block", "link_block", "img_block"},
					},
					{
						Id: "header_block",
						Content: &model.BlockContentOfText{
							Text: &model.BlockContentText{
								Text:  "Welcome to Anytype Channel",
								Style: model.BlockContentText_Header1,
							},
						},
					},
					{
						Id: "text_block",
						Content: &model.BlockContentOfText{
							Text: &model.BlockContentText{
								Text:  "This is a paragraph with bold and italic text.",
								Style: model.BlockContentText_Paragraph,
								Marks: &model.BlockContentTextMarks{
									Marks: []*model.BlockContentTextMark{
										{
											Type:  model.BlockContentTextMark_Bold,
											Range: &model.Range{From: 10, To: 19},
										},
										{
											Type:  model.BlockContentTextMark_Italic,
											Range: &model.Range{From: 34, To: 40},
										},
									},
								},
							},
						},
					},
					{
						Id: "callout_block",
						Content: &model.BlockContentOfText{
							Text: &model.BlockContentText{
								Text:      "Important Callout Info",
								Style:     model.BlockContentText_Callout,
								IconEmoji: "💡",
							},
						},
					},
					{
						Id: "link_block",
						Content: &model.BlockContentOfText{
							Text: &model.BlockContentText{
								Text:  "Click here to view Sub Page",
								Style: model.BlockContentText_Paragraph,
								Marks: &model.BlockContentTextMarks{
									Marks: []*model.BlockContentTextMark{
										{
											Type:  model.BlockContentTextMark_Link,
											Param: "anytype://spaceId/" + subPageID,
											Range: &model.Range{From: 11, To: 27},
										},
									},
								},
							},
						},
					},
					{
						Id: "img_block",
						Content: &model.BlockContentOfFile{
							File: &model.BlockContentFile{
								Type:           model.BlockContentFile_Image,
								Name:           "sample.png",
								TargetObjectId: "img_obj_1",
							},
						},
					},
				},
			},
		},
	}

	// 2. Create sub page snapshot
	subSnap := &pb.SnapshotWithType{
		SbType: model.SmartBlockType_Page,
		Snapshot: &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{
					Fields: map[string]*types.Value{
						"name": {Kind: &types.Value_StringValue{StringValue: "Sub Page Details"}},
					},
				},
				Blocks: []*model.Block{
					{
						Id:          "sub_root_block",
						Content:     &model.BlockContentOfSmartblock{},
						ChildrenIds: []string{"sub_text"},
					},
					{
						Id: "sub_text",
						Content: &model.BlockContentOfText{
							Text: &model.BlockContentText{
								Text:  "Content of the sub-page.",
								Style: model.BlockContentText_Paragraph,
							},
						},
					},
				},
			},
		},
	}

	rootBytes, err := proto.Marshal(rootSnap)
	require.NoError(t, err)
	subBytes, err := proto.Marshal(subSnap)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(objectsDir, rootPageID+".pb"), rootBytes, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(objectsDir, subPageID+".pb"), subBytes, 0644))

	// Create sample image file
	require.NoError(t, os.WriteFile(filepath.Join(filesDir, "sample.png"), []byte("pngdata"), 0644))

	// Run SPA builder
	builder := publish.NewSingleFileHtmlBuilder()
	htmlData, err := builder.BuildSPA(tempDir, rootPageID, "https://invite.anytype.io/123#456")
	require.NoError(t, err)

	htmlStr := string(htmlData)

	// Verifications:
	// 1. Root page rendered with active class and correct data-path
	assert.Contains(t, htmlStr, `id="route-root_page_1"`)
	assert.Contains(t, htmlStr, `data-path="/"`)
	assert.Contains(t, htmlStr, `class="page active"`)

	// 2. Sub page rendered with data-path="/sub_page_2"
	assert.Contains(t, htmlStr, `id="route-sub_page_2"`)
	assert.Contains(t, htmlStr, `data-path="/sub_page_2"`)

	// 3. Headings, callout, bold, italic
	assert.Contains(t, htmlStr, "<h2>Welcome to Anytype Channel</h2>")
	assert.Contains(t, htmlStr, "<b>paragraph</b>")
	assert.Contains(t, htmlStr, "<i>italic</i>")
	assert.Contains(t, htmlStr, `class="callout"`)
	assert.Contains(t, htmlStr, "💡")

	// 4. Client-side link navigation transformed to navigate()
	assert.Contains(t, htmlStr, `href="/sub_page_2" onclick="navigate(event, '/sub_page_2')"`)

	// 5. Embedded script tag and CSS styling
	assert.Contains(t, htmlStr, "<script>")
	assert.Contains(t, htmlStr, "function navigate(e, path)")
	assert.Contains(t, htmlStr, "function renderRoute(path)")
	assert.Contains(t, htmlStr, "function toggleTheme()")
	assert.Contains(t, htmlStr, ".page {")
	assert.Contains(t, htmlStr, ".page.active {")

	// 6. Invite banner link
	assert.Contains(t, htmlStr, `href="https://invite.anytype.io/123#456"`)

	// 7. Image file src reference
	assert.True(t, strings.Contains(htmlStr, "files/sample.png") || strings.Contains(htmlStr, "data:image/png;base64,"))
}
