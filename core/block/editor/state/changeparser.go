package state

import (
	"fmt"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/anytype-heart/pb"
	"strings"
	"unicode"
)

type ChangeParser struct {
}

func (c ChangeParser) ParseChange(change *objecttree.Change, isRoot bool) ([]string, error) {
	if change == nil {
		return nil, nil
	}
	if change.Model == nil {
		return nil, nil
	}
	pbChange, ok := change.Model.(*pb.Change)
	if !ok {
		return nil, nil
	}
	return c.parseContent(pbChange.Content)
}

func (c ChangeParser) parseContent(content []*pb.ChangeContent) (chSymbs []string, err error) {
	for _, chc := range content {
		tp := fmt.Sprintf("%T", chc.Value)
		tp = strings.Replace(tp, "ChangeContentValueOf", "", 1)
		res := ""
		for _, ts := range tp {
			if unicode.IsUpper(ts) {
				res += string(ts)
			}
		}
		var target []string
		switch {
		case chc.GetBlockCreate() != nil:
			target = append(target, chc.GetBlockCreate().TargetId)
		case chc.GetBlockDuplicate() != nil:
			target = append(target, chc.GetBlockDuplicate().TargetId)
		case chc.GetBlockMove() != nil:
			target = append(target, chc.GetBlockMove().TargetId)
		case chc.GetBlockRemove() != nil:
			target = append(target, chc.GetBlockRemove().Ids...)
		}
		if len(target) >= 1 {
			res += "->" + strings.Join(target, "/")
		}
		chSymbs = append(chSymbs, res)
	}
	return
}
