package state

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (s *State) applyEvent(ev *pb.EventMessage) (err error) {
	var apply = func(id string, f func(b simple.Block) error) (err error) {
		if b := s.Get(id); b != nil {
			return f(b)
		}
		return fmt.Errorf("can't apply change: block not found")
	}
	switch o := ev.Value.(type) {
	case *pb.EventMessageValueOfBlockSetAlign:
		if err = apply(o.BlockSetAlign.Id, func(b simple.Block) error {
			b.Model().Align = o.BlockSetAlign.Align
			return nil
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetBackgroundColor:
		if err = apply(o.BlockSetBackgroundColor.Id, func(b simple.Block) error {
			b.Model().BackgroundColor = o.BlockSetBackgroundColor.BackgroundColor
			return nil
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetBookmark:
		if err = apply(o.BlockSetBookmark.Id, func(b simple.Block) error {
			if bm, ok := b.(bookmark.Block); ok {
				return bm.ApplyEvent(o.BlockSetBookmark)
			}
			return fmt.Errorf("not a bookmark block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetDiv:
		if err = apply(o.BlockSetDiv.Id, func(b simple.Block) error {
			if d, ok := b.(base.DivBlock); ok {
				return d.ApplyEvent(o.BlockSetDiv)
			}
			return fmt.Errorf("not a div block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetText:
		if err = apply(o.BlockSetText.Id, func(b simple.Block) error {
			if t, ok := b.(text.Block); ok {
				return t.ApplyEvent(o.BlockSetText)
			}
			return fmt.Errorf("not a text block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetFields:
		if err = apply(o.BlockSetFields.Id, func(b simple.Block) error {
			b.Model().Fields = o.BlockSetFields.Fields
			return nil
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetFile:
		if err = apply(o.BlockSetFile.Id, func(b simple.Block) error {
			if f, ok := b.(file.Block); ok {
				return f.ApplyEvent(o.BlockSetFile)
			}
			return fmt.Errorf("not a file block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetLink:
		if err = apply(o.BlockSetLink.Id, func(b simple.Block) error {
			if f, ok := b.(link.Block); ok {
				return f.ApplyEvent(o.BlockSetLink)
			}
			return fmt.Errorf("not a link block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockSetDataviewView:
		if err = apply(o.BlockSetDataviewView.Id, func(b simple.Block) error {
			if f, ok := b.(dataview.Block); ok && o.BlockSetDataviewView.View != nil {
				if f.SetView(o.BlockSetDataviewView.Id, *o.BlockSetDataviewView.View) != nil {
					f.AddView(*o.BlockSetDataviewView.View)
				}
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}
	case *pb.EventMessageValueOfBlockDeleteDataviewView:
		if err = apply(o.BlockDeleteDataviewView.Id, func(b simple.Block) error {
			if f, ok := b.(dataview.Block); ok {
				f.DeleteView(o.BlockDeleteDataviewView.Id)
			}
			return fmt.Errorf("not a dataview block")
		}); err != nil {
			return
		}
	}
	return nil
}
