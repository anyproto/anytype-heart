package pbtypes

import (
	"sync"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
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

func EventsToSliceChange(changes []*pb.EventBlockDataviewSliceChange) []slice.Change {
	sliceOpMap := map[pb.EventBlockDataviewSliceOperation]slice.DiffOperation{
		pb.EventBlockDataview_SliceOperationNone:    slice.OperationNone,
		pb.EventBlockDataview_SliceOperationAdd:     slice.OperationAdd,
		pb.EventBlockDataview_SliceOperationMove:    slice.OperationMove,
		pb.EventBlockDataview_SliceOperationRemove:  slice.OperationRemove,
		pb.EventBlockDataview_SliceOperationReplace: slice.OperationReplace,
	}

	var res []slice.Change
	for _, eventCh := range changes {
		res = append(res, slice.Change{Op: sliceOpMap[eventCh.Op], Ids: eventCh.Ids, AfterId: eventCh.AfterId})
	}

	return res
}

func SliceChangeToEvents(changes []slice.Change) []*pb.EventBlockDataviewSliceChange {
	eventsOpMap := map[slice.DiffOperation]pb.EventBlockDataviewSliceOperation{
		slice.OperationNone:    pb.EventBlockDataview_SliceOperationNone,
		slice.OperationAdd:     pb.EventBlockDataview_SliceOperationAdd,
		slice.OperationMove:    pb.EventBlockDataview_SliceOperationMove,
		slice.OperationRemove:  pb.EventBlockDataview_SliceOperationRemove,
		slice.OperationReplace: pb.EventBlockDataview_SliceOperationReplace,
	}

	var res []*pb.EventBlockDataviewSliceChange
	for _, sliceCh := range changes {
		res = append(res, &pb.EventBlockDataviewSliceChange{Op: eventsOpMap[sliceCh.Op], Ids: sliceCh.Ids, AfterId: sliceCh.AfterId})
	}

	return res
}
