package search

type SearchRequest struct {
	Query string      `json:"query"`
	Types []string    `json:"types"`
	Sort  SortOptions `json:"sort"`
}

type SortOptions struct {
	Direction string `json:"direction"` // "asc" or "desc"
	Timestamp string `json:"timestamp"` // "created_date", "last_modified_date" or "last_opened_date"
}
