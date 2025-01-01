package object

type GetObjectResponse struct {
	Object Object `json:"object"`
}

type CreateObjectRequest struct {
	Name                string `json:"name"`
	Icon                string `json:"icon"`
	TemplateId          string `json:"template_id"`
	ObjectTypeUniqueKey string `json:"object_type_unique_key"`
	WithChat            bool   `json:"with_chat"`
}

type CreateObjectResponse struct {
	Object Object `json:"object"`
}

type UpdateObjectRequest struct {
	Object Object `json:"object"`
}

type UpdateObjectResponse struct {
	Object Object `json:"object"`
}

type Object struct {
	Type       string   `json:"type" example:"object"`
	Id         string   `json:"id" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"`
	Name       string   `json:"name" example:"Object Name"`
	Icon       string   `json:"icon" example:"📄"`
	ObjectType string   `json:"object_type" example:"Page"`
	SpaceId    string   `json:"space_id" example:"bafyreigyfkt6rbv24sbv5aq2hko3bhmv5xxlf22b4bypdu6j7hnphm3psq.23me69r569oi1"`
	RootId     string   `json:"root_id"`
	Blocks     []Block  `json:"blocks"`
	Details    []Detail `json:"details"`
}

type Block struct {
	Id              string   `json:"id"`
	ChildrenIds     []string `json:"children_ids"`
	BackgroundColor string   `json:"background_color"`
	Align           string   `json:"align"`
	VerticalAlign   string   `json:"vertical_align"`
	Text            *Text    `json:"text,omitempty"`
	File            *File    `json:"file,omitempty"`
}

type Text struct {
	Text    string `json:"text"`
	Style   string `json:"style"`
	Checked bool   `json:"checked"`
	Color   string `json:"color"`
	Icon    string `json:"icon"`
}

type File struct {
	Hash           string `json:"hash"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	Mime           string `json:"mime"`
	Size           int    `json:"size"`
	AddedAt        int    `json:"added_at"`
	TargetObjectId string `json:"target_object_id"`
	State          string `json:"state"`
	Style          string `json:"style"`
}

type Detail struct {
	Id      string                 `json:"id"`
	Details map[string]interface{} `json:"details"`
}

type Tag struct {
	Id    string `json:"id" example:"bafyreiaixlnaefu3ci22zdenjhsdlyaeeoyjrsid5qhfeejzlccijbj7sq"`
	Name  string `json:"name" example:"Tag Name"`
	Color string `json:"color" example:"yellow"`
}

type ObjectType struct {
	Type      string `json:"type" example:"object_type"`
	Id        string `json:"id" example:"bafyreigyb6l5szohs32ts26ku2j42yd65e6hqy2u3gtzgdwqv6hzftsetu"`
	UniqueKey string `json:"unique_key" example:"ot-page"`
	Name      string `json:"name" example:"Page"`
	Icon      string `json:"icon" example:"📄"`
}

type ObjectTemplate struct {
	Type string `json:"type" example:"object_template"`
	Id   string `json:"id" example:"bafyreictrp3obmnf6dwejy5o4p7bderaaia4bdg2psxbfzf44yya5uutge"`
	Name string `json:"name" example:"Object Template Name"`
	Icon string `json:"icon" example:"📄"`
}
