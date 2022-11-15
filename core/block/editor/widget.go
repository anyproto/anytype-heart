package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type WidgetObject struct {
	smartblock.SmartBlock
}

func NewWidgetObject() *WidgetObject {
	return &WidgetObject{
		SmartBlock: smartblock.New(),
	}
}

func (w *WidgetObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = w.SmartBlock.Init(ctx); err != nil {
		return
	}
	return smartblock.ObjectApplyTemplate(w, ctx.State, template.WithObjectTypesAndLayout([]string{bundle.TypeKeyWidget.URL()}, model.ObjectType_basic))
}
