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

func CopyStruct(in *types.Struct) (out *types.Struct) {
	if in == nil {
		return nil
	}
	buf := bytesPool.Get().([]byte)
	size := in.Size()
	if cap(buf) < size {
		buf = make([]byte, 0, size*2)
	}
	size, _ = in.MarshalToSizedBuffer(buf[:size])
	out = &types.Struct{}
	_ = out.Unmarshal(buf[:size])
	if out.Fields == nil && in.Fields != nil {
		out.Fields = make(map[string]*types.Value)
	}
	bytesPool.Put(buf)
	return
}

func CopyVal(in *types.Value) (out *types.Value) {
	if in == nil {
		return nil
	}
	buf := bytesPool.Get().([]byte)
	size := in.Size()
	if cap(buf) < size {
		buf = make([]byte, 0, size*2)
	}
	size, _ = in.MarshalToSizedBuffer(buf[:size])
	out = &types.Value{}
	_ = out.Unmarshal(buf[:size])

	bytesPool.Put(buf)
	return
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

func CopyRelationOptions(in []*model.RelationOption) (out []*model.RelationOption) {
	out = make([]*model.RelationOption, len(in))
	for i := range in {
		out[i] = &*in[i]
	}
	return
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
