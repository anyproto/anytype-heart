package filesync

import (
	"fmt"
	"time"

	"github.com/anyproto/any-store/anyenc"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/domain"
)

type FileState int

const (
	FileStatePendingUpload FileState = iota // File is scheduled for upload
	FileStateUploading                      // File is in process of uploading. This state should be reset to PendingUpload on application restart
	FileStateLimited                        // File is not fitted in space limits and is waiting for more free space on a file node
	FileStatePendingDeletion
	FileStateDone
	FileStateDeleted
)

// IsUploadingState returns true if a state related to the uploading process, including uploaded (Done) state
func (s FileState) IsUploadingState() bool {
	switch s {
	case FileStatePendingUpload, FileStateUploading, FileStateLimited, FileStateDone:
		return true
	default:
		return false
	}
}

type FileInfo struct {
	FileId      domain.FileId
	SpaceId     string
	ObjectId    string
	State       FileState
	ScheduledAt time.Time
	Variants    []domain.FileId
	AddedByUser bool
	Imported    bool

	BytesToUploadOrBind int
	CidsToBind          map[cid.Cid]struct{}
	CidsToUpload        map[cid.Cid]struct{}
}

func (i FileInfo) FullFileId() domain.FullFileId {
	return domain.FullFileId{
		FileId:  i.FileId,
		SpaceId: i.SpaceId,
	}
}

func (i FileInfo) Reschedule() FileInfo {
	i.ScheduledAt = time.Now().Add(time.Minute)

	return i
}

func (i FileInfo) Key() string {
	return i.ObjectId
}

func marshalFileInfo(arena *anyenc.Arena, info FileInfo) *anyenc.Value {
	obj := arena.NewObject()
	obj.Set("fileId", arena.NewString(info.FileId.String()))
	obj.Set("spaceId", arena.NewString(info.SpaceId))
	obj.Set("id", arena.NewString(info.ObjectId))
	obj.Set("state", arena.NewNumberInt(int(info.State)))
	obj.Set("scheduledAt", arena.NewNumberInt(int(info.ScheduledAt.UTC().Unix())))
	variants := arena.NewArray()
	for i, variant := range info.Variants {
		variants.SetArrayItem(i, arena.NewString(variant.String()))
	}
	obj.Set("variants", variants)
	obj.Set("addedByUser", newBool(arena, info.AddedByUser))
	obj.Set("imported", newBool(arena, info.Imported))
	obj.Set("bytesToUploadOrBind", arena.NewNumberInt(info.BytesToUploadOrBind))

	cidsToUpload := arena.NewArray()
	var i int
	for c := range info.CidsToUpload {
		cidsToUpload.SetArrayItem(i, arena.NewString(c.String()))
	}
	obj.Set("cidsToUpload", cidsToUpload)

	cidsToBind := arena.NewArray()
	i = 0
	for c := range info.CidsToBind {
		cidsToBind.SetArrayItem(i, arena.NewString(c.String()))
	}
	obj.Set("cidsToBind", cidsToBind)
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
	cidsToBind := map[cid.Cid]struct{}{}
	for _, raw := range doc.GetArray("cidsToBind") {
		c, err := cid.Parse(raw.GetString())
		if err != nil {
			return FileInfo{}, fmt.Errorf("parse cid: %w", err)
		}
		cidsToBind[c] = struct{}{}
	}
	fileId := domain.FileId(doc.GetString("fileId"))
	if !fileId.Valid() {
		return FileInfo{}, fmt.Errorf("invalid file id: %q", fileId.String())
	}
	return FileInfo{
		FileId:              fileId,
		SpaceId:             doc.GetString("spaceId"),
		ObjectId:            doc.GetString("id"),
		State:               FileState(doc.GetInt("state")),
		ScheduledAt:         time.Unix(int64(doc.GetInt("scheduledAt")), 0).UTC(),
		Variants:            variants,
		AddedByUser:         doc.GetBool("addedByUser"),
		Imported:            doc.GetBool("imported"),
		BytesToUploadOrBind: doc.GetInt("bytesToUploadOrBind"),
		CidsToBind:          cidsToBind,
		CidsToUpload:        cidsToUpload,
	}, nil
}
