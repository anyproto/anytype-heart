package filequeue

import (
	"fmt"
	"time"

	"github.com/anyproto/any-store/anyenc"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/domain"
)

type FileState int

const (
	FileStatePendingUpload FileState = iota
	FileStateUploading
	FileStateLimited
	FileStatePendingDeletion
	FileStateDone
	FileStateDeleted
)

type FileInfo struct {
	FileId      domain.FileId
	SpaceId     string
	ObjectId    string
	State       FileState
	ScheduledAt time.Time
	HandledAt   time.Time
	Variants    []domain.FileId
	AddedByUser bool
	Imported    bool

	BytesToUpload int
	CidsToUpload  map[cid.Cid]struct{}
}

func marshalFileInfo(arena *anyenc.Arena, info FileInfo) *anyenc.Value {
	obj := arena.NewObject()
	obj.Set("fileId", arena.NewString(info.FileId.String()))
	obj.Set("spaceId", arena.NewString(info.SpaceId))
	obj.Set("id", arena.NewString(info.ObjectId))
	obj.Set("state", arena.NewNumberInt(int(info.State)))
	obj.Set("addedAt", arena.NewNumberInt(int(info.ScheduledAt.UTC().Unix())))
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
	var variants []domain.FileId
	if len(rawVariants) > 0 {
		variants = make([]domain.FileId, 0, len(rawVariants))
		for _, v := range rawVariants {
			variants = append(variants, domain.FileId(v.GetString()))
		}
	}
	var cidsToUpload map[cid.Cid]struct{}
	rawCidsToUpload := doc.GetArray("cidsToUpload")
	if len(rawCidsToUpload) > 0 {
		cidsToUpload = make(map[cid.Cid]struct{}, len(rawCidsToUpload))
		for _, raw := range rawCidsToUpload {
			c, err := cid.Parse(raw.GetString())
			if err != nil {
				return FileInfo{}, fmt.Errorf("parse cid: %w", err)
			}
			cidsToUpload[c] = struct{}{}
		}
	}
	fileId := domain.FileId(doc.GetString("fileId"))
	return FileInfo{
		FileId:        fileId,
		SpaceId:       doc.GetString("spaceId"),
		ObjectId:      doc.GetString("id"),
		State:         FileState(doc.GetInt("state")),
		ScheduledAt:   time.Unix(int64(doc.GetInt("addedAt")), 0).UTC(),
		HandledAt:     time.Unix(int64(doc.GetInt("handledAt")), 0).UTC(),
		Variants:      variants,
		AddedByUser:   doc.GetBool("addedByUser"),
		Imported:      doc.GetBool("imported"),
		BytesToUpload: doc.GetInt("bytesToUpload"),
		CidsToUpload:  cidsToUpload,
	}, nil
}
