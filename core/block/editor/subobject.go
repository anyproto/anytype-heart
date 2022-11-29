package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"strings"
)

func NewSubObject() *SubObject {
	sb := smartblock.New()
	return &SubObject{
		SmartBlock: sb,
		Basic:      basic.NewBasic(sb),
		IHistory:   basic.NewHistory(sb),
		Text:       stext.NewText(sb),
		Clipboard:  clipboard.NewClipboard(sb, nil),
		Dataview:   dataview.NewDataview(sb),
	}
}

type SubObject struct {
	smartblock.SmartBlock
	basic.Basic
	basic.IHistory
	stext.Text
	clipboard.Clipboard
	dataview.Dataview
}

func (o *SubObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = o.SmartBlock.Init(ctx); err != nil {
		return
	}
	ot := pbtypes.GetString(ctx.State.CombinedDetails(), bundle.RelationKeyType.String())

	if strings.HasPrefix(ot, addr.BundledObjectTypeURLPrefix) {
		ot = addr.ObjectTypeKeyToIdPrefix + strings.TrimPrefix(ot, addr.BundledObjectTypeURLPrefix)
	}

	if strings.HasPrefix(ot, addr.BundledRelationURLPrefix) {
		ot = addr.RelationKeyToIdPrefix + strings.TrimPrefix(ot, addr.BundledRelationURLPrefix)
	}

	// temp fix for our internal accounts with inconsistent types (should be removed later)
	// todo: remove after release
	fixTypes := func(s *state.State) {
		if list := pbtypes.GetStringList(s.Details(), bundle.RelationKeyRelationFormatObjectTypes.String()); list != nil {
			list, _ = relationutils.MigrateObjectTypeIds(list)
			s.SetDetail(bundle.RelationKeyRelationFormatObjectTypes.String(), pbtypes.StringList(list))
		}
	}

	return smartblock.ObjectApplyTemplate(o, ctx.State,
		template.WithForcedObjectTypes([]string{ot}),
		fixTypes,
	)
}

func (o *SubObject) SetStruct(st *types.Struct) error {
	o.Lock()
	defer o.Unlock()
	s := o.NewState()
	s.SetDetails(st)
	return o.Apply(s)
}
