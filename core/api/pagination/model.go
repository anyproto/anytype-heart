package pagination

type PaginationMeta struct {
	Total   int  `json:"total" example:"1024"`    // the total number of items available on that endpoint
	Offset  int  `json:"offset" example:"0"`      // the current offset
	Limit   int  `json:"limit" example:"100"`     // the current limit
	HasMore bool `json:"has_more" example:"true"` // whether there are more items available
}

type PaginatedResponse[T any] struct {
	Data       []T            `json:"data"`
	Pagination PaginationMeta `json:"pagination"`
}
