package pagination

type PaginationMeta struct {
	Total   int  `json:"total" example:"1024"`    // The total number of items available for the endpoint
	Offset  int  `json:"offset" example:"0"`      // The number of items skipped before starting to collect the result set
	Limit   int  `json:"limit" example:"100"`     // The maximum number of items returned in the result set
	HasMore bool `json:"has_more" example:"true"` // Indicates if there are more items available beyond the current result set
}

type PaginatedResponse[T any] struct {
	Data       []T            `json:"data"`       // The list of items in the current result set
	Pagination PaginationMeta `json:"pagination"` // The pagination metadata for the response
}
