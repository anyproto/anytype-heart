package core

import (
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/timestamp"
)

type PageVersion struct {
	VersionId string
	PageId    string
	User      string
	Date      *timestamp.Timestamp
	Fields    *structpb.Struct
	Content   *BlockContentPage
}

func (pageVersion *PageVersion) GetVersionId() string {
	return pageVersion.VersionId
}

func (pageVersion *PageVersion) GetBlockId() string {
	return pageVersion.PageId
}

func (pageVersion *PageVersion) GetUser() string {
	return pageVersion.User
}

func (pageVersion *PageVersion) GetDate() *timestamp.Timestamp {
	return pageVersion.Date
}

func (pageVersion *PageVersion) GetName() string {
	if name, exists := pageVersion.Fields.Fields["name"]; exists {
		return name.GetStringValue()
	}

	return ""
}

func (pageVersion *PageVersion) GetIcon() string {
	if icon, exists := pageVersion.Fields.Fields["icon"]; exists {
		return icon.GetStringValue()
	}

	return ""
}

func (pageVersion *PageVersion) GetBlocks() map[string]*Block {
	return pageVersion.Content.Blocks.BlockById
}

func (ver *PageVersion) GetFields() *structpb.Struct {
	return ver.Fields
}

func (ver *PageVersion) GetExternalFields() *structpb.Struct {
	return &structpb.Struct{Fields: map[string]*structpb.Value{
		"name": ver.Fields.Fields["name"],
		"icon": ver.Fields.Fields["icon"],
	}}
}

func (ver *PageVersion) GetSmartBlocksTree(anytype *Anytype) ([]SmartBlock, error) {

	return []SmartBlock{}, nil
	/*if ver == nil {
		return nil
	}

	var blockById = make(map[string]*Block)

	var traverseTree func([]*Block)
	traverseTree = func(list []*Block) {
		for _, block := range list {
			if smartBlock := block.SmartBlock(anytype); smartBlock == nil{

			}
			if _, alreadyExists := blockById[block.Id]; !alreadyExists {
				switch block.Content.(type) {
				case *Block_Page:
					blockById[block.Id] = block
					traverseTree(block.GetPage().Blocks.Blocks)

				}
				traverseTree(block.Ge)
			}
		}
	}

	traverseTree(ver.Blocks)
	return blockById*/
}
