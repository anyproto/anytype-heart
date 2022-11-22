package block

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
)

type BlockType string

const (
	Paragraph       BlockType = "paragraph"
	BulletList      BlockType = "bulleted_list_item"
	NumberList      BlockType = "numbered_list_item"
	Toggle          BlockType = "toggle"
	SyncedBlock     BlockType = "synced_block"
	Template        BlockType = "template"
	Column          BlockType = "column"
	ChildPage       BlockType = "child_page"
	ChildDatabase   BlockType = "child_database"
	Table           BlockType = "table"
	Heading1        BlockType = "heading_1"
	Heading2        BlockType = "heading_2"
	Heading3        BlockType = "heading_3"
	ToDo            BlockType = "to_do"
	Embed           BlockType = "embed"
	Image           BlockType = "image"
	Video           BlockType = "video"
	File            BlockType = "file"
	Pdf             BlockType = "pdf"
	Bookmark        BlockType = "bookmark"
	Callout         BlockType = "callout"
	Quote           BlockType = "quote"
	Equation        BlockType = "equation"
	Divider         BlockType = "divider"
	TableOfContents BlockType = "table_of_contents"
	ColumnList      BlockType = "column_list"
	LinkPreview     BlockType = "link_preview"
	LinkToPage      BlockType = "link_to_page"
	TableRow        BlockType = "table_row"
	Code            BlockType = "code"
	Unsupported     BlockType = "unsupported"
)

type Block struct {
	Object         string     `json:"object"`
	ID             string     `json:"id"`
	CreatedTime    string     `json:"created_time"`
	LastEditedTime string     `json:"last_edited_time"`
	CreatedBy      api.User   `json:"created_by,omitempty"`
	LastEditedBy   api.User   `json:"last_edited_by,omitempty"`
	Parent         api.Parent `json:"parent"`
	Archived       bool       `json:"archived"`
	HasChildren    bool       `json:"has_children"`
	Type           BlockType  `json:"type"`
}
