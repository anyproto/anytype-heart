package apimodel

type TagResponse struct {
	Tag Tag `json:"tag"` // The tag
}

type CreateTagRequest struct {
	Name  string `json:"name" binding:"required" example:"In progress"`                                                           // The name of the tag
	Color Color  `json:"color" binding:"required" example:"yellow" enums:"grey,yellow,orange,red,pink,purple,blue,ice,teal,lime"` // The color of the tag
}

type UpdateTagRequest struct {
	Name  *string `json:"name,omitempty"  example:"In progress"`                                                           // The name to set for the tag
	Color *Color  `json:"color,omitempty"  example:"yellow" enums:"grey,yellow,orange,red,pink,purple,blue,ice,teal,lime"` // The color to set for the tag
}

type Tag struct {
	Object string `json:"object" example:"tag"`                                                                 // The data model of the object
	Id     string `json:"id" example:"bafyreiaixlnaefu3ci22zdenjhsdlyaeeoyjrsid5qhfeejzlccijbj7sq"`             // The id of the tag
	Key    string `json:"key" example:"67b0d3e3cda913b84c1299b1"`                                               // The key of the tag
	Name   string `json:"name" example:"in-progress"`                                                           // The name of the tag
	Color  Color  `json:"color" example:"yellow" enums:"grey,yellow,orange,red,pink,purple,blue,ice,teal,lime"` // The color of the tag
}
