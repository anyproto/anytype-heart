package apimodel

// FileType represents the type of file being uploaded
type FileType string

const (
	FileTypeNone  FileType = ""
	FileTypeFile  FileType = "file"
	FileTypeImage FileType = "image"
	FileTypeVideo FileType = "video"
	FileTypeAudio FileType = "audio"
	FileTypePDF   FileType = "pdf"
)

// FileUploadResponse represents a successful file upload response
type FileUploadResponse struct {
	Object   string `json:"object" example:"file"`                                                         // The data model of the object
	ObjectId string `json:"object_id" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"` // The object ID of the uploaded file
}
