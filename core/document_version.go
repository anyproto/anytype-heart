package core

import (
	"encoding/json"

	"github.com/requilence/go-anytype/pb"
)

func (ver *DocumentVersion) ChildrenIds() []string {
	var children []string
for _, block := range ver.BlocksFlattenMap(){
	if block.Type == pb.DocumentBlockType_NEW_PAGE {
		var content DocumentBlockContentNewPage

		json.Unmarshal([]byte(block.Content), &content)

		if content.Id != "" {
			children = append(children, content.Id)
		}
	}
}
return children
}

func (ver *DocumentVersion) BlocksFlattenMap() map[string]*pb.DocumentBlock {
	var blockById = make(map[string]*pb.DocumentBlock)

	if ver == nil {
		return blockById
	}

	var traverseTree func([]*pb.DocumentBlock)
	traverseTree = func(list []*pb.DocumentBlock) {
		for _, block := range list {
			if _, alreadyExists := blockById[block.Id]; !alreadyExists {
				blockById[block.Id] = block
				traverseTree(block.Children)
			}
		}
	}

	traverseTree(ver.Blocks)
	return blockById
}

