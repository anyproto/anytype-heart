package apimodel

type AddObjectsToListRequest struct {
	Objects []string `json:"objects" example:"[\"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ\"]"` // The list of object IDs to add to the list
}

type View struct {
	Id      string   `json:"id" example:"67bf3f21cda9134102e2422c"`    // The id of the view
	Name    string   `json:"name" example:"All"`                       // The name of the view
	Layout  string   `json:"layout" example:"grid" enums:"grid,table"` // The layout of the view
	Filters []Filter `json:"filters"`                                  // The list of filters
	Sorts   []Sort   `json:"sorts"`                                    // The list of sorts
}

type Filter struct {
	Id          string          `json:"id" example:"67bf3f21cda9134102e2422c"`                                                                     // The id of the filter
	PropertyKey string          `json:"property_key" example:"name"`                                                                               // The property key used for filtering
	Format      PropertyFormat  `json:"format" example:"text" enums:"text,number,select,multi_select,date,files,checkbox,url,email,phone,objects"` // The format of the property used for filtering
	Condition   FilterCondition `json:"condition" example:"contains" enums:"eq,ne,gt,gte,lt,lte,contains,ncontains,in,nin,all,empty,nempty"`       // The filter condition
	Value       string          `json:"value" example:"Some value..."`                                                                             // The value used for filtering
}

type Sort struct {
	Id          string         `json:"id" example:"67bf3f21cda9134102e2422c"`                                                                     // The id of the sort
	PropertyKey string         `json:"property_key" example:"name"`                                                                               // The property key used for sorting
	Format      PropertyFormat `json:"format" example:"text" enums:"text,number,select,multi_select,date,files,checkbox,url,email,phone,objects"` // The format of the property used for sorting
	SortType    string         `json:"sort_type" example:"asc" enums:"asc,desc,custom"`                                                           // The sort direction
}
