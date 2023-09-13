package block

import (
	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type Type string

const (
	TypeParagraph       Type = "paragraph"
	TypeBulletList      Type = "bulleted_list_item"
	TypeNumberList      Type = "numbered_list_item"
	TypeToggle          Type = "toggle"
	TypeSyncedBlock     Type = "synced_block"
	TypeTemplate        Type = "template"
	TypeColumn          Type = "column"
	TypeChildPage       Type = "child_page"
	TypeChildDatabase   Type = "child_database"
	TypeTable           Type = "table"
	TypeHeading1        Type = "heading_1"
	TypeHeading2        Type = "heading_2"
	TypeHeading3        Type = "heading_3"
	TypeToDo            Type = "to_do"
	TypeEmbed           Type = "embed"
	TypeImage           Type = "image"
	TypeVideo           Type = "video"
	TypeAudio           Type = "audio"
	TypeFile            Type = "file"
	TypePdf             Type = "pdf"
	TypeBookmark        Type = "bookmark"
	TypeCallout         Type = "callout"
	TypeQuote           Type = "quote"
	TypeEquation        Type = "equation"
	TypeDivider         Type = "divider"
	TypeTableOfContents Type = "table_of_contents"
	TypeColumnList      Type = "column_list"
	TypeLinkPreview     Type = "link_preview"
	TypeLinkToPage      Type = "link_to_page"
	TypeTableRow        Type = "table_row"
	TypeCode            Type = "code"
	TypeUnsupported     Type = "unsupported"
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
	GetBlocks(req *api.NotionImportContext, pageID string) *MapResponse
}

const unsupportedBlockMessage = "Unsupported block"

type UnsupportedBlock struct{}

func (*UnsupportedBlock) GetBlocks(*api.NotionImportContext, string) *MapResponse {
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
