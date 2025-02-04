package export

type ObjectExportRequest struct {
	Path string `json:"path" example:"/path/to/export"`
}

type ObjectExportResponse struct {
	Path string `json:"path" example:"/path/to/export"`
}
