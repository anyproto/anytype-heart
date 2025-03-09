package search

type SearchRequest struct {
	Query string      `json:"query" example:"test"`                                                                                                    // The search term to look for in object names and snippets
	Types []string    `json:"types" example:"ot-note,ot-page,ot-678043f0cda9133be777049f,bafyreightzrdts2ymxyaeyzspwdfo2juspyam76ewq6qq7ixnw3523gs7q"` // The types of objects to search for, specified by unique key or ID
	Sort  SortOptions `json:"sort"`                                                                                                                    // The sorting criteria and direction for the search results
}

type SortOptions struct {
	Direction string `json:"direction" enums:"asc,desc" default:"desc"`                                                       // The direction to sort the search results
	Timestamp string `json:"timestamp" enums:"created_date,last_modified_date,last_opened_date" default:"last_modified_date"` // The timestamp to sort the search results by
}
