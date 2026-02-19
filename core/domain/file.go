package domain

import (
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

// FileId is a CID of the root of file's DAG
type FileId string

func (id FileId) String() string {
	return string(id)
}

func (id FileId) Cid() (cid.Cid, error) {
	return cid.Parse(string(id))
}

func (id FileId) Valid() bool {
	return IsFileId(string(id))
}

type FullFileId struct {
	SpaceId string
	FileId  FileId
}

func (id FullFileId) Valid() bool {
	return id.FileId.Valid()
}

type FileEncryptionKeys struct {
	FileId FileId
	// Encryption key per file variant:
	// "/0/" for ordinary files (only one variant)
	// "/0/original", "/0/large", "/0/small", "/0/thumbnail", "/0/exif" for images
	EncryptionKeys map[string]string
}

// FileContentId is a CID of file variant's content node
type FileContentId string

func (id FileContentId) String() string {
	return string(id)
}

func IsFileId(raw string) bool {
	c, err := cid.Decode(raw)
	if err != nil {
		return false
	}
	return c.Prefix().Codec == cid.DagProtobuf && c.Prefix().MhType == multihash.SHA2_256
}
