package core

func (block *Block) ToSmartBlock(anytype *Anytype) SmartBlock {
	switch block.Content.(type) {
	case *Block_Page:
		return &Page{
			thread: anytype.Textile.Node().Thread(block.Id),
			node:   anytype,
		}
	case *Block_Dashboard:
		return &Dashboard{
			thread: anytype.Textile.Node().Thread(block.Id),
			node:   anytype,
		}
	default:
		return nil
	}
}
