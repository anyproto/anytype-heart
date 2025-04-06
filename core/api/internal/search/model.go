package search

type SortDirection string

const (
	Asc  SortDirection = "asc"
	Desc SortDirection = "desc"
)

type SortProperty string

const (
	CreatedDate      SortProperty = "created_date"
	LastModifiedDate SortProperty = "last_modified_date"
	LastOpenedDate   SortProperty = "last_opened_date"
	Name             SortProperty = "name"
)

type SearchRequest struct {
	Query string      `json:"query" example:"test"`                                                                                            // The search term to look for in object names and snippets
	Types []string    `json:"types" example:"ot-page,ot-678043f0cda9133be777049f,bafyreightzrdts2ymxyaeyzspwdfo2juspyam76ewq6qq7ixnw3523gs7q"` // The types of objects to search for, specified by key or ID
	Sort  SortOptions `json:"sort"`                                                                                                            // The sorting criteria and direction for the search results
}

type SortOptions struct {
	Property  SortProperty  `json:"property" enums:"created_date,last_modified_date,last_opened_date,name" default:"last_modified_date"` // The property to sort the search results by
	Direction SortDirection `json:"direction" enums:"asc,desc" default:"desc"`                                                           // The direction to sort the search results
}
