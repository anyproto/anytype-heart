package block

type ColumnListBlock struct {
	Block
	ColumnList interface{} `json:"column_list"`
}

func (c *ColumnListBlock) GetID() string {
	return c.ID
}

func (c *ColumnListBlock) HasChild() bool {
	return c.HasChildren
}

func (c *ColumnListBlock) SetChildren(children []interface{}) {
	c.ColumnList = children
}

func (c *ColumnListBlock) GetBlocks(req *MapRequest) *MapResponse {
	req.Blocks = c.ColumnList.([]interface{})
	resp := MapBlocks(req)
	return resp
}

type ColumnBlock struct {
	Block
	Column *ColumnObject `json:"column"`
}

type ColumnObject struct {
	Children []interface{} `json:"children"`
}

func (c *ColumnBlock) GetBlocks(req *MapRequest) *MapResponse {
	req.Blocks = c.Column.Children
	resp := MapBlocks(req)
	return resp
}

func (c *ColumnBlock) GetID() string {
	return c.ID
}

func (c *ColumnBlock) HasChild() bool {
	return c.HasChildren
}

func (c *ColumnBlock) SetChildren(children []interface{}) {
	c.Column.Children = children
}
