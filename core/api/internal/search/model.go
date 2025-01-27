package search

type SearchRequest struct {
	Query string      `json:"query"`
	Types []string    `json:"types"`
	Sort  SortOptions `json:"sort"`
}

type SortOptions struct {
	Direction string `json:"direction" enums:"asc|desc" default:"desc"`
	Timestamp string `json:"timestamp" enums:"created_date|last_modified_date|last_opened_date" default:"last_modified_date"`
}
