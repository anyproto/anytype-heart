package smartblock

import (
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func marshalBlock(a *fastjson.Arena, b *model.Block) *fastjson.Value {
	obj := a.NewObject()
	if b != nil {
		obj.Set("id", a.NewString(b.Id))
		obj.Set("content", marshalContent(a, b.Content))
		obj.Set("childrenIds", marshalChildrenIds(a, b.ChildrenIds))
	}
	return obj
}

func marshalChildrenIds(a *fastjson.Arena, childrenIds []string) *fastjson.Value {
	arr := a.NewArray()
	for i, id := range childrenIds {
		arr.SetArrayItem(i, a.NewString(id))
	}
	return arr
}

func marshalContent(a *fastjson.Arena, c model.IsBlockContent) *fastjson.Value {
	var contentType string
	var val *fastjson.Value

	switch c := c.(type) {
	case *model.BlockContentOfText:
		contentType, val = marshalContentText(a, c)
	}

	if val == nil {
		contentType = "unknown"
		val = a.NewObject()
	}

	obj := a.NewObject()
	obj.Set(contentType, val)
	return obj
}

func marshalContentText(a *fastjson.Arena, c *model.BlockContentOfText) (string, *fastjson.Value) {
	obj := a.NewObject()
	obj.Set("text", a.NewString(c.Text.Text))
	return "text", obj
}
