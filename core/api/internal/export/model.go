package export

type ObjectExportRequest struct {
	Path string `json:"path" example:"/path/to/export"` // The path to export the object to
}

type ObjectExportResponse struct {
	Path string `json:"path" example:"/path/to/export"` // The path the object was exported to
}
