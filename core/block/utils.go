package block

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/gogo/protobuf/types"
	"github.com/mohae/deepcopy"
)

func findPosInSlice(s []string, v string) int {
	for i, sv := range s {
		if sv == v {
			return i
		}
	}
	return -1
}

func insertToSlice(s []string, v string, pos int) []string {
	if len(s) <= pos {
		return append(s, v)
	}
	if pos == 0 {
		return append([]string{v}, s[pos:]...)
	}
	return append(s[:pos], append([]string{v}, s[pos:]...)...)
}

func fieldsGetString(field *types.Struct, key string) (value string, ok bool) {
	if field != nil && field.Fields != nil {
		if value, ok := field.Fields[key]; ok {
			if s, ok := value.Kind.(*types.Value_StringValue); ok {
				return s.StringValue, true
			}
		}
	}
	return
}

func blockCopy(b *model.Block) (c *model.Block) {
	if b == nil {
		return nil
	}
	return deepcopy.Copy(b).(*model.Block)
}

type uniqueIds map[string]struct{}

func (u uniqueIds) Add(id string) (exists bool) {
	if _, exists = u[id]; exists {
		return
	}
	u[id] = struct{}{}
	return
}
