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

// CopyStructMap copies pb struct map, while reusing map values' pointers
func CopyStructMap(in *types.Struct) (out *types.Struct) {
	if in == nil {
		return nil
	}
	if in.Fields == nil {
		return &types.Struct{}
	}

	out = &types.Struct{Fields: make(map[string]*types.Value, len(in.Fields))}
	for k, v := range in.Fields {
		out.Fields[k] = v
	}

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

func CopyRelationLink(in *model.RelationLink) (out *model.RelationLink) {
	return &model.RelationLink{
		Key:    in.Key,
		Format: in.Format,
	}
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

func CopyOptions(in []*model.RelationOption) (out []*model.RelationOption) {
	if in == nil {
		return nil
	}

	for _, inO := range in {
		inCopy := *inO
		out = append(out, &inCopy)
	}
	return
}

func CopyRelationsToMap(in []*model.Relation) (out map[string]*model.Relation) {
	out = make(map[string]*model.Relation, len(in))
	rels := CopyRelations(in)
	for _, rel := range rels {
		out[rel.Key] = rel
	}

	return
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

func RelationsFilterKeys(in []*model.Relation, keys []string) (out []*model.Relation) {
	for i, inRel := range in {
		if slice.FindPos(keys, inRel.Key) >= 0 {
			out = append(out, in[i])
		}
	}
	return
}

func StructNotNilKeys(st *types.Struct) (keys []string) {
	if st == nil || st.Fields == nil {
		return nil
	}

	for k, v := range st.Fields {
		if v != nil {
			keys = append(keys, k)
		}
	}
	return
}

func EventsToSliceChange(changes []*pb.EventBlockDataviewSliceChange) []slice.Change[slice.ID] {
	var res []slice.Change[slice.ID]
	for _, eventCh := range changes {
		var ch slice.Change[slice.ID]
		switch eventCh.Op {
		case pb.EventBlockDataview_SliceOperationAdd:
			ch = slice.MakeChangeAdd(slice.StringsToIDs(eventCh.Ids), eventCh.AfterId)
		case pb.EventBlockDataview_SliceOperationMove:
			ch = slice.MakeChangeMove[slice.ID](eventCh.Ids, eventCh.AfterId)
		case pb.EventBlockDataview_SliceOperationRemove:
			ch = slice.MakeChangeRemove[slice.ID](eventCh.Ids)
		case pb.EventBlockDataview_SliceOperationReplace:
			// TODO check this out
			// ch = slice.MakeChangeReplace(slice.StringsToIDs(eventCh.Ids), eventCh.AfterId)
		}
		res = append(res, ch)
	}

	return res
}

func SliceChangeToEvents(changes []slice.Change[slice.ID]) []*pb.EventBlockDataviewSliceChange {
	var res []*pb.EventBlockDataviewSliceChange
	for _, sliceCh := range changes {
		if add := sliceCh.Add(); add != nil {
			res = append(res, &pb.EventBlockDataviewSliceChange{
				Op:      pb.EventBlockDataview_SliceOperationAdd,
				Ids:     slice.IDsToStrings(add.Items),
				AfterId: add.AfterId,
			})
		}
		if move := sliceCh.Move(); move != nil {
			res = append(res, &pb.EventBlockDataviewSliceChange{
				Op:      pb.EventBlockDataview_SliceOperationMove,
				Ids:     move.IDs,
				AfterId: move.AfterId,
			})
		}
		if rm := sliceCh.Remove(); rm != nil {
			res = append(res, &pb.EventBlockDataviewSliceChange{
				Op:  pb.EventBlockDataview_SliceOperationRemove,
				Ids: rm.IDs,
			})
		}
		// TODO check this out
		// if replace := sliceCh.Replace(); replace != nil {
		// 	res = append(res, &pb.EventBlockDataviewSliceChange{
		// 		Op:      pb.EventBlockDataview_SliceOperationReplace,
		// 		Ids:     slice.IDsToStrings(replace.Items),
		// 		AfterId: replace.AfterId,
		// 	})
		// }
	}

	return res
}
