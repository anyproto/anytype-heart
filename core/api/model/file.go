package apimodel

// FileUploadResponse represents the response after uploading a file
type FileUploadResponse struct {
	ObjectId string         `json:"object_id"`         // File object ID
	FileId   string         `json:"file_id"`           // Preload file ID (IPFS CID)
	Details  map[string]any `json:"details,omitempty"` // File metadata
}
