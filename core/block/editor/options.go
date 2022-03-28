package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/subobject"
)

type SubObjectCreator interface {
	NewSubObject(subId string, parent subobject.ParentObject) (s *subobject.SubObject, err error)
}

func NewOptions(sc SubObjectCreator) *Options {
	return &Options{
		SmartBlock: smartblock.New(),
		sc:         sc,
	}
}

type Options struct {
	smartblock.SmartBlock
	opened []*Option
	sc     SubObjectCreator
}

func (o *Options) Open(id string) (sb smartblock.SmartBlock, err error) {
	return
}

func (o *Options) Locked() bool {
	return o.SmartBlock.Locked() || len(o.opened) > 0
}

func NewOption(opts *Options) (*Option, error) {
	return nil, nil
}

type Option struct {
	id     string
	parent *Options
	*subobject.SubObject
}

func (o *Option) Close() (err error) {
	return o.SubObject.Close()
}
