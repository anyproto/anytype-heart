package domain

// FileId is a CID of the root of file's DAG
type FileId string

func (h FileId) String() string {
	return string(h)
}

type FullFileId struct {
	SpaceId string
	FileId  FileId
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
