package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
)

func NewOptions() *Options {
	return &Options{
		SmartBlock: smartblock.New(),
	}
}

type Options struct {
	smartblock.SmartBlock
	options []*Option
}

func (o *Options) Open(id string) (sb smartblock.SmartBlock, err error) {
	return
}

func (o *Options) Locked() bool {
	return len(o.options) > 0
}

type Option struct {
	id string
	*Options
}

func (o *Option) Id() string {
	return o.Options.Id() + "/" + o.id
}
