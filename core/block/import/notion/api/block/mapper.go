package block

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type Mapper struct{}

func (m *Mapper) MapBlocks(blocks []interface{}, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID map[string]string) ([]*model.Block, []string) {
	var (
		anytypeBlocks = make([]*model.Block, 0)
		ids           = make([]string, 0)
	)
	for _, bl := range blocks {
		switch block := bl.(type) {
		case *ParagraphBlock:
			childIds := make([]string, 0)
			var childBlocks []*model.Block
			if block.HasChildren {
				childBlocks, childIds = m.MapBlocks(block.Paragraph.Children, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			}
			allBlocks, blockIDs := block.Paragraph.GetTextBlocks(model.BlockContentText_Paragraph, childIds, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			anytypeBlocks = append(anytypeBlocks, allBlocks...)
			anytypeBlocks = append(anytypeBlocks, childBlocks...)
			ids = append(ids, blockIDs...)
		case *Heading1Block:
			allBlocks, blockIDs := block.Heading1.GetTextBlocks(model.BlockContentText_Header1, []string{}, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			anytypeBlocks = append(anytypeBlocks, allBlocks...)
			ids = append(ids, blockIDs...)
		case *Heading2Block:
			allBlocks, blockIDs := block.Heading2.GetTextBlocks(model.BlockContentText_Header2, []string{}, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			anytypeBlocks = append(anytypeBlocks, allBlocks...)
			ids = append(ids, blockIDs...)
		case *Heading3Block:
			allBlocks, blockIDs := block.Heading3.GetTextBlocks(model.BlockContentText_Header3, []string{}, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			anytypeBlocks = append(anytypeBlocks, allBlocks...)
			ids = append(ids, blockIDs...)
		case *CalloutBlock:
			childIds := make([]string, 0)
			var childBlocks []*model.Block
			if block.HasChildren {
				childBlocks, childIds = m.MapBlocks(block.Callout.Children, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			}
			calloutBlocks, blockIDs := block.Callout.GetCalloutBlocks(childIds)
			anytypeBlocks = append(anytypeBlocks, calloutBlocks...)
			anytypeBlocks = append(anytypeBlocks, childBlocks...)
			ids = append(ids, blockIDs...)
		case *QuoteBlock:
			childIds := make([]string, 0)
			var childBlocks []*model.Block
			if block.HasChildren {
				childBlocks, childIds = m.MapBlocks(block.Quote.Children, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			}
			allBlocks, blockIDs := block.Quote.GetTextBlocks(model.BlockContentText_Quote, childIds, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			anytypeBlocks = append(anytypeBlocks, allBlocks...)
			anytypeBlocks = append(anytypeBlocks, childBlocks...)
			ids = append(ids, blockIDs...)
		case *BulletedListBlock:
			childIds := make([]string, 0)
			var childBlocks []*model.Block
			if block.HasChildren {
				childBlocks, childIds = m.MapBlocks(block.BulletedList.Children, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			}
			allBlocks, blockIDs := block.BulletedList.GetTextBlocks(model.BlockContentText_Marked, childIds, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			anytypeBlocks = append(anytypeBlocks, allBlocks...)
			anytypeBlocks = append(anytypeBlocks, childBlocks...)
			ids = append(ids, blockIDs...)
		case *NumberedListBlock:
			childIds := make([]string, 0)
			var childBlocks []*model.Block
			if block.HasChildren {
				childBlocks, childIds = m.MapBlocks(block.NumberedList.Children, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			}
			allBlocks, blockIDs := block.NumberedList.GetTextBlocks(model.BlockContentText_Numbered, childIds, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			anytypeBlocks = append(anytypeBlocks, allBlocks...)
			anytypeBlocks = append(anytypeBlocks, childBlocks...)
			ids = append(ids, blockIDs...)
		case *ToggleBlock:
			childIds := make([]string, 0)
			var childBlocks []*model.Block
			if block.HasChildren {
				childBlocks, childIds = m.MapBlocks(block.Toggle.Children, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			}
			allBlocks, blockIDs := block.Toggle.GetTextBlocks(model.BlockContentText_Toggle, childIds, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			anytypeBlocks = append(anytypeBlocks, allBlocks...)
			anytypeBlocks = append(anytypeBlocks, childBlocks...)
			ids = append(ids, blockIDs...)
		case *CodeBlock:
			c := bl.(*CodeBlock)
			anytypeBlocks = append(anytypeBlocks, c.Code.GetCodeBlock())
		case *EquationBlock:
			e := bl.(*EquationBlock)
			anytypeBlocks = append(anytypeBlocks, e.Equation.HandleEquation())
		case *ToDoBlock:
			childIds := make([]string, 0)
			var childBlocks []*model.Block
			if block.HasChildren {
				childBlocks, childIds = m.MapBlocks(block.ToDo.Children, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			}
			allBlocks, blockIDs := block.ToDo.GetTextBlocks(model.BlockContentText_Checkbox, childIds, notionPageIdsToAnytype, notionDatabaseIdsToAnytype, pageNameToID, databaseNameToID)
			anytypeBlocks = append(anytypeBlocks, allBlocks...)
			anytypeBlocks = append(anytypeBlocks, childBlocks...)
			ids = append(ids, blockIDs...)
		case *FileBlock:
			fileBlock, id := block.File.GetFileBlock(model.BlockContentFile_File)
			anytypeBlocks = append(anytypeBlocks, fileBlock)
			ids = append(ids, id)
		case *ImageBlock:
			fileBlock, id := block.File.GetFileBlock(model.BlockContentFile_Image)
			anytypeBlocks = append(anytypeBlocks, fileBlock)
			ids = append(ids, id)
		case *VideoBlock:
			fileBlock, id := block.File.GetFileBlock(model.BlockContentFile_Video)
			anytypeBlocks = append(anytypeBlocks, fileBlock)
			ids = append(ids, id)
		case *PdfBlock:
			fileBlock, id := block.File.GetFileBlock(model.BlockContentFile_PDF)
			anytypeBlocks = append(anytypeBlocks, fileBlock)
			ids = append(ids, id)
		case *DividerBlock:
			db, id := block.GetDivBlock()
			anytypeBlocks = append(anytypeBlocks, db)
			ids = append(ids, id)
		case *TableOfContentsBlock:
			db, id := block.GetTableOfContentsBlock()
			anytypeBlocks = append(anytypeBlocks, db)
			ids = append(ids, id)
		}
	}
	return anytypeBlocks, ids
}
