package apimodel

// FileUploadResponse represents the response after uploading a file
type FileUploadResponse struct {
	ObjectId    string `json:"object_id"`               // File object ID
	Name        string `json:"name"`                    // Original file name as stored
	Media       string `json:"media"`                   // MIME type (e.g. "image/png")
	Extension   string `json:"extension,omitempty"`     // File extension without dot, when known
	SizeInBytes int64  `json:"size_in_bytes,omitempty"` // Size of the uploaded file
}
