package filequeue

import (
	"time"

	"github.com/anyproto/any-store/anyenc"

	"github.com/anyproto/anytype-heart/core/domain"
)

// Code just for tests

type fileState int

const (
	fileStateUploading fileState = iota
	fileStatePendingDeletion
	fileStateDeleted
)

type fileInfo struct {
	FileId      domain.FileId
	ObjectId    string
	State       fileState
	ScheduledAt time.Time
	Imported    bool

	BytesToUpload int
}

func marshalFileInfo(arena *anyenc.Arena, info fileInfo) *anyenc.Value {
	obj := arena.NewObject()
	obj.Set("fileId", arena.NewString(info.FileId.String()))
	obj.Set("id", arena.NewString(info.ObjectId))
	obj.Set("state", arena.NewNumberInt(int(info.State)))
	obj.Set("addedAt", arena.NewNumberInt(int(info.ScheduledAt.UTC().Unix())))
	obj.Set("bytesToUpload", arena.NewNumberInt(info.BytesToUpload))
	obj.Set("imported", newBool(arena, info.Imported))
	return obj
}

func newBool(arena *anyenc.Arena, val bool) *anyenc.Value {
	if val {
		return arena.NewTrue()
	}
	return arena.NewFalse()
}

func unmarshalFileInfo(doc *anyenc.Value) (fileInfo, error) {
	fileId := domain.FileId(doc.GetString("fileId"))
	return fileInfo{
		FileId:        fileId,
		ObjectId:      doc.GetString("id"),
		State:         fileState(doc.GetInt("state")),
		ScheduledAt:   time.Unix(int64(doc.GetInt("addedAt")), 0).UTC(),
		BytesToUpload: doc.GetInt("bytesToUpload"),
		Imported:      doc.GetBool("imported"),
	}, nil
}
