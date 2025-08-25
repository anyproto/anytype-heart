package filesync

import (
	"fmt"
	"time"

	"github.com/anyproto/any-store/anyenc"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/domain"
)

type anystoreFileRepository struct {
}

func marshalFileInfo(info FileInfo, arena *anyenc.Arena) *anyenc.Value {
	obj := arena.NewObject()
	obj.Set("fileId", arena.NewString(info.FileId.String()))
	obj.Set("spaceId", arena.NewString(info.SpaceId))
	obj.Set("objectId", arena.NewString(info.ObjectId))
	obj.Set("state", arena.NewNumberInt(int(info.State)))
	obj.Set("addedAt", arena.NewNumberInt(int(info.AddedAt.UTC().Unix())))
	obj.Set("handledAt", arena.NewNumberInt(int(info.HandledAt.UTC().Unix())))
	variants := arena.NewArray()
	for i, variant := range info.Variants {
		variants.SetArrayItem(i, arena.NewString(variant.String()))
	}
	obj.Set("variants", variants)
	obj.Set("addedByUser", newBool(arena, info.AddedByUser))
	obj.Set("imported", newBool(arena, info.Imported))
	obj.Set("bytesToUpload", arena.NewNumberInt(info.BytesToUpload))
	cidsToUpload := arena.NewArray()
	var i int
	for c := range info.CidsToUpload {
		cidsToUpload.SetArrayItem(i, arena.NewString(c.String()))
	}
	obj.Set("cidsToUpload", cidsToUpload)
	return obj
}

func newBool(arena *anyenc.Arena, val bool) *anyenc.Value {
	if val {
		return arena.NewTrue()
	}
	return arena.NewFalse()
}

func unmarshalFileInfo(doc *anyenc.Value) (FileInfo, error) {
	rawVariants := doc.GetArray("variants")
	variants := make([]domain.FileId, 0, len(rawVariants))
	for _, v := range rawVariants {
		variants = append(variants, domain.FileId(v.GetString()))
	}
	cidsToUpload := map[cid.Cid]struct{}{}
	for _, raw := range doc.GetArray("cidsToUpload") {
		c, err := cid.Parse(raw.GetString())
		if err != nil {
			return FileInfo{}, fmt.Errorf("parse cid: %w", err)
		}
		cidsToUpload[c] = struct{}{}
	}
	fileId := domain.FileId(doc.GetString("fileId"))
	if !fileId.Valid() {
		return FileInfo{}, fmt.Errorf("invalid file id")
	}
	return FileInfo{
		FileId:        fileId,
		SpaceId:       doc.GetString("spaceId"),
		ObjectId:      doc.GetString("objectId"),
		State:         FileState(doc.GetInt("state")),
		AddedAt:       time.Unix(int64(doc.GetInt("addedAt")), 0).UTC(),
		HandledAt:     time.Unix(int64(doc.GetInt("handledAt")), 0).UTC(),
		Variants:      variants,
		AddedByUser:   doc.GetBool("addedByUser"),
		Imported:      doc.GetBool("imported"),
		BytesToUpload: doc.GetInt("bytesToUpload"),
		CidsToUpload:  cidsToUpload,
	}, nil
}

/*
queue behavior:
	- get next item, handle it OR start timer to wait for it
	- subscribe for all changes
	- wait for next item's timer OR for subscription change
*/
