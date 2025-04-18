package list

type View struct {
	Id      string   `json:"id" example:"67bf3f21cda9134102e2422c"`    // The id of the view
	Name    string   `json:"name" example:"All"`                       // The name of the view
	Layout  string   `json:"layout" example:"grid" enums:"grid,table"` // The layout of the view
	Filters []Filter `json:"filters"`                                  // The list of filters
	Sorts   []Sort   `json:"sorts"`                                    // The list of sorts
}

type Filter struct {
	Id          string `json:"id" example:"67bf3f21cda9134102e2422c"`                                                                                                                                                  // The id of the filter
	PropertyKey string `json:"property_key" example:"name"`                                                                                                                                                            // The property key used for filtering
	Format      string `json:"format" example:"text" enum:"text,number,select,multi_select,date,file,checkbox,url,email,phone,object"`                                                                                 // The format of the property used for filtering
	Condition   string `json:"condition" example:"contains" enum:"equal,not_equal,greater,less,greater_or_equal,less_or_equal,like,not_like,in,not_in,empty,not_empty,all_in,not_all_in,exact_in,not_exact_in,exists"` // The filter condition
	Value       string `json:"value" example:"Some value..."`                                                                                                                                                          // The value used for filtering
}

type Sort struct {
	Id          string `json:"id" example:"67bf3f21cda9134102e2422c"`                                                                  // The id of the sort
	PropertyKey string `json:"property_key" example:"name"`                                                                            // The property key used for sorting
	Format      string `json:"format" example:"text" enum:"text,number,select,multi_select,date,file,checkbox,url,email,phone,object"` // The format of the property used for sorting
	SortType    string `json:"sort_type" example:"asc" enum:"asc,desc,custom"`                                                         // The sort direction
}
