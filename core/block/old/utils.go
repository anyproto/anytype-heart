package old

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/gogo/protobuf/types"
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

func removeFromSlice(s []string, v string) []string {
	var n int
	for _, x := range s {
		if x != v {
			s[n] = x
			n++
		}
	}
	return s[:n]
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

func fieldsGetFloat(field *types.Struct, key string) (value float64, ok bool) {
	if field != nil && field.Fields != nil {
		if value, ok := field.Fields[key]; ok {
			if s, ok := value.Kind.(*types.Value_NumberValue); ok {
				return s.NumberValue, true
			}
		}
	}
	return
}

func isSmartBlock(m *model.Block) bool {
	if m == nil {
		return false
	}
	switch m.Content.(type) {
	case *model.BlockContentOfPage, *model.BlockContentOfDashboard, *model.BlockContentOfDataview:
		return true
	}
	return false
}
