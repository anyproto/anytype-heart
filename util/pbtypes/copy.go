package pbtypes

import (
	"sync"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var bytesPool = &sync.Pool{
	New: func() interface{} {
		return []byte{}
	},
}

func CopyBlock(in *model.Block) (out *model.Block) {
	buf := bytesPool.Get().([]byte)
	size := in.Size()
	if cap(buf) < size {
		buf = make([]byte, 0, size*2)
	}
	size, _ = in.MarshalToSizedBuffer(buf[:size])
	out = &model.Block{}
	_ = out.Unmarshal(buf[:size])
	bytesPool.Put(buf)
	return
}

func CopyStruct(s *types.Struct) *types.Struct {
	if s == nil {
		return nil
	}

	copiedStruct := &types.Struct{
		Fields: make(map[string]*types.Value, len(s.Fields)),
	}

	for key, value := range s.Fields {
		copiedStruct.Fields[key] = CopyVal(value)
	}

	return copiedStruct
}

func CopyVal(v *types.Value) *types.Value {
	if v == nil {
		return nil
	}

	copiedValue := &types.Value{}

	switch kind := v.Kind.(type) {
	case *types.Value_NullValue:
		copiedValue.Kind = &types.Value_NullValue{NullValue: kind.NullValue}
	case *types.Value_NumberValue:
		copiedValue.Kind = &types.Value_NumberValue{NumberValue: kind.NumberValue}
	case *types.Value_StringValue:
		copiedValue.Kind = &types.Value_StringValue{StringValue: kind.StringValue}
	case *types.Value_BoolValue:
		copiedValue.Kind = &types.Value_BoolValue{BoolValue: kind.BoolValue}
	case *types.Value_StructValue:
		copiedValue.Kind = &types.Value_StructValue{StructValue: CopyStruct(kind.StructValue)}
	case *types.Value_ListValue:
		copiedValue.Kind = &types.Value_ListValue{ListValue: CopyListVal(kind.ListValue)}
	}

	return copiedValue
}

func CopyListVal(lv *types.ListValue) *types.ListValue {
	if lv == nil {
		return nil
	}

	copiedListValue := &types.ListValue{
		Values: make([]*types.Value, len(lv.Values)),
	}

	for i, value := range lv.Values {
		copiedListValue.Values[i] = CopyVal(value)
	}

	return copiedListValue
}

func CopyRelation(in *model.Relation) (out *model.Relation) {
	if in == nil {
		return nil
	}
	buf := bytesPool.Get().([]byte)
	size := in.Size()
	if cap(buf) < size {
		buf = make([]byte, 0, size*2)
	}
	size, _ = in.MarshalToSizedBuffer(buf[:size])
	out = &model.Relation{}
	_ = out.Unmarshal(buf[:size])

	bytesPool.Put(buf)
	return out
}

func CopyLayout(in *model.Layout) (out *model.Layout) {
	return &model.Layout{Id: in.Id, Name: in.Name, RequiredRelations: CopyRelations(in.RequiredRelations)}
}

func CopyObjectType(in *model.ObjectType) (out *model.ObjectType) {
	if in == nil {
		return nil
	}

	buf := bytesPool.Get().([]byte)
	size := in.Size()
	if cap(buf) < size {
		buf = make([]byte, 0, size*2)
	}
	size, _ = in.MarshalToSizedBuffer(buf[:size])
	out = &model.ObjectType{}
	_ = out.Unmarshal(buf[:size])

	bytesPool.Put(buf)
	return out
}

func CopyRelations(in []*model.Relation) (out []*model.Relation) {
	if in == nil {
		return nil
	}
	buf := bytesPool.Get().([]byte)
	inWrapped := model.Relations{Relations: in}
	size := inWrapped.Size()
	if cap(buf) < size {
		buf = make([]byte, 0, size*2)
	}
	size, _ = inWrapped.MarshalToSizedBuffer(buf[:size])
	outWrapped := &model.Relations{}
	_ = outWrapped.Unmarshal(buf[:size])

	bytesPool.Put(buf)
	return outWrapped.Relations
}

func CopyFilter(in *model.BlockContentDataviewFilter) (out *model.BlockContentDataviewFilter) {
	buf := bytesPool.Get().([]byte)
	size := in.Size()
	if cap(buf) < size {
		buf = make([]byte, 0, size*2)
	}
	size, _ = in.MarshalToSizedBuffer(buf[:size])
	out = &model.BlockContentDataviewFilter{}
	_ = out.Unmarshal(buf[:size])
	bytesPool.Put(buf)
	return
}
