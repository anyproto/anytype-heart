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

// FileContentId is a CID of file variant's content node
type FileContentId string

func (id FileContentId) String() string {
	return string(id)
}
