package storestate

import (
	"encoding/json"

	"github.com/anyproto/any-store/anyenc"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/pb"
)

type Builder struct {
	*pb.StoreChange
}

func (b *Builder) init() {
	if b.StoreChange == nil {
		b.StoreChange = &pb.StoreChange{}
	}
}

func (b *Builder) Create(collection, id string, doc any) (err error) {
	jsonString, err := b.toJSONString(doc)
	if err != nil {
		return
	}
	b.init()
	b.ChangeSet = append(b.ChangeSet, &pb.StoreChangeContent{
		Change: &pb.StoreChangeContentChangeOfCreate{
			Create: &pb.DocumentCreate{
				Collection: collection,
				DocumentId: id,
				Value:      jsonString,
			},
		},
	})
	return
}

func (b *Builder) Modify(collection, id string, keyPath []string, op pb.ModifyOp, val any) (err error) {
	jsonString, err := b.toJSONString(val)
	if err != nil {
		return
	}
	b.init()

	keyMod := &pb.KeyModify{
		KeyPath:     keyPath,
		ModifyOp:    op,
		ModifyValue: jsonString,
	}

	for _, ch := range b.ChangeSet {
		if mod := ch.GetModify(); mod != nil {
			if mod.Collection == collection && mod.DocumentId == id {
				mod.Keys = append(mod.Keys, keyMod)
				return
			}
		}
	}

	b.ChangeSet = append(b.ChangeSet, &pb.StoreChangeContent{
		Change: &pb.StoreChangeContentChangeOfModify{
			Modify: &pb.DocumentModify{
				Collection: collection,
				DocumentId: id,
				Keys:       []*pb.KeyModify{keyMod},
			},
		},
	})

	return
}

func (b *Builder) Delete(collection, id string) {
	b.init()
	b.ChangeSet = append(b.ChangeSet, &pb.StoreChangeContent{
		Change: &pb.StoreChangeContentChangeOfDelete{
			Delete: &pb.DocumentDelete{
				Collection: collection,
				DocumentId: id,
			},
		},
	})
}

func (b *Builder) toJSONString(doc any) (res string, err error) {
	if str, ok := doc.(string); ok {
		return str, nil
	}
	if fj, ok := doc.(*fastjson.Value); ok {
		return fj.String(), nil
	}
	if anyEnc, ok := doc.(*anyenc.Value); ok {
		return anyEnc.String(), nil
	}
	resBytes, err := json.Marshal(doc)
	if err != nil {
		return
	}
	return string(resBytes), nil
}
