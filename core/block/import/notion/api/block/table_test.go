package block

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/stretchr/testify/assert"
)

func Test_TableWithOneColumnAndRow(t *testing.T) {
	
	tb := &TableBlock{
		Table: TableObject{
			Width:           1,
			HasColumnHeader: false,
			HasRowHeader:    true,
			Children:        []*TableRowBlock{
				{
					TableRowObject: TableRowObject{},
				},
			},
		},
	}

	resp := tb.GetBlocks(&MapRequest{})

	assert.NotNil(t, resp)
	assert.Len(t, resp.Blocks, 5) // table block + column block + row block + 2 empty text blocks
}

func Test_TableWithoutContent(t *testing.T) {
	
	tb := &TableBlock{
		Table: TableObject{
			Width:           3,
			HasColumnHeader: false,
			HasRowHeader:    true,
			Children:        []*TableRowBlock{
				{
					TableRowObject: TableRowObject{},
				},
				{
					TableRowObject: TableRowObject{},
				},
			},
		},
	}


	assert.Len(t, tb.Table.Children, 2)

	resp := tb.GetBlocks(&MapRequest{})

	assert.NotNil(t, resp)
	assert.Len(t, resp.Blocks, 8) // table block + 3 * column block + 1 column layout + 1 row layout + 3 * row block 
}

func Test_TableWithDifferentText(t *testing.T) {
	
	tb := &TableBlock{
		Table: TableObject{
			Width:           3,
			HasColumnHeader: false,
			HasRowHeader:    true,
			Children:        []*TableRowBlock{
				{
					TableRowObject: TableRowObject{
						Cells: [][]api.RichText{
							{
								{
									Type: api.Text,
									PlainText: "Text",
								},
							},
						},
					},
				},
				{
					TableRowObject: TableRowObject{},
				},
			},
		},
	}


	assert.Len(t, tb.Table.Children, 2)

	resp := tb.GetBlocks(&MapRequest{})

	assert.NotNil(t, resp)
	assert.Len(t, resp.Blocks, 9) // table block + 3 * column block + 1 column layout + 1 row layout + 3 * row block + 1 text block
}