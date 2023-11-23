package domain

// FileId is a CID of the root of file's IPLD tree
type FileId string

func (h FileId) String() string {
	return string(h)
}

type FullFileId struct {
	SpaceId string
	FileId  FileId
}

type FileKeys struct {
	FileId         FileId
	EncryptionKeys map[string]string
}

type ChildFileId string

func (id ChildFileId) String() string {
	return string(id)
}
