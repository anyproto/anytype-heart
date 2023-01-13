package block

import (
	"github.com/globalsign/mgo/bson"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type Type string

const (
	Paragraph       Type = "paragraph"
	BulletList      Type = "bulleted_list_item"
	NumberList      Type = "numbered_list_item"
	Toggle          Type = "toggle"
	SyncedBlock     Type = "synced_block"
	Template        Type = "template"
	Column          Type = "column"
	ChildPage       Type = "child_page"
	ChildDatabase   Type = "child_database"
	Table           Type = "table"
	Heading1        Type = "heading_1"
	Heading2        Type = "heading_2"
	Heading3        Type = "heading_3"
	ToDo            Type = "to_do"
	Embed           Type = "embed"
	Image           Type = "image"
	Video           Type = "video"
	File            Type = "file"
	Pdf             Type = "pdf"
	Bookmark        Type = "bookmark"
	Callout         Type = "callout"
	Quote           Type = "quote"
	Equation        Type = "equation"
	Divider         Type = "divider"
	TableOfContents Type = "table_of_contents"
	ColumnList      Type = "column_list"
	LinkPreview     Type = "link_preview"
	LinkToPage      Type = "link_to_page"
	TableRow        Type = "table_row"
	Code            Type = "code"
	Unsupported     Type = "unsupported"
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
	Type           Type       `json:"type"`
}

type Identifiable interface {
	GetID() string
}

type ChildSetter interface {
	Identifiable
	HasChild() bool
	SetChildren(children []interface{})
}

type Getter interface {
	GetBlocks(req *MapRequest) *MapResponse
}

const unsupportedBlockMessage = "Unsupported block"

type UnsupportedBlock struct{}

func (*UnsupportedBlock) GetBlocks(req *MapRequest) *MapResponse {
	id := bson.NewObjectId().Hex()
	bl := &model.Block{
		Id: id,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: unsupportedBlockMessage,
			},
		},
	}
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{id},
	}
}
