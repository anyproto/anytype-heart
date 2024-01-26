package editor

import (
	"time"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
)

type participant struct {
	smartblock.SmartBlock
}

func (f *ObjectFactory) newParticipant(sb smartblock.SmartBlock) *participant {
	return &participant{
		SmartBlock: sb,
	}
}
func (p *participant) Init(ctx *smartblock.InitContext) (err error) {
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}

	template.InitTemplate(ctx.State,
		template.WithEmpty,
	)
	return nil
}

func (p *participant) TryClose(objectTTL time.Duration) (bool, error) {
	return false, nil
}
