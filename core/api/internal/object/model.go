package object

type CreateObjectRequest struct {
	Name                string `json:"name"`
	Icon                string `json:"icon"`
	Description         string `json:"description"`
	Body                string `json:"body"`
	Source              string `json:"source"`
	TemplateId          string `json:"template_id"`
	ObjectTypeUniqueKey string `json:"object_type_unique_key"`
}

type ObjectResponse struct {
	Object Object `json:"object"`
}

type Object struct {
	Type    string   `json:"type" example:"Page"`
	Id      string   `json:"id" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"`
	Name    string   `json:"name" example:"Object Name"`
	Icon    string   `json:"icon" example:"📄"`
	Snippet string   `json:"snippet" example:"The beginning of the object body..."`
	Layout  string   `json:"layout" example:"basic"`
	SpaceId string   `json:"space_id" example:"bafyreigyfkt6rbv24sbv5aq2hko3bhmv5xxlf22b4bypdu6j7hnphm3psq.23me69r569oi1"`
	RootId  string   `json:"root_id"`
	Blocks  []Block  `json:"blocks"`
	Details []Detail `json:"details"`
}

type Block struct {
	Id              string   `json:"id"`
	ChildrenIds     []string `json:"children_ids"`
	BackgroundColor string   `json:"background_color"`
	Align           string   `json:"align" example:"AlignLeft"`
	VerticalAlign   string   `json:"vertical_align" example:"VerticalAlignTop"`
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

type TypeResponse struct {
	Type Type `json:"type"`
}

type Type struct {
	Type              string `json:"type" example:"type"`
	Id                string `json:"id" example:"bafyreigyb6l5szohs32ts26ku2j42yd65e6hqy2u3gtzgdwqv6hzftsetu"`
	UniqueKey         string `json:"unique_key" example:"ot-page"`
	Name              string `json:"name" example:"Page"`
	Icon              string `json:"icon" example:"📄"`
	RecommendedLayout string `json:"recommended_layout" example:"todo"`
}

type TemplateResponse struct {
	Template Template `json:"template"`
}

type Template struct {
	Type string `json:"type" example:"template"`
	Id   string `json:"id" example:"bafyreictrp3obmnf6dwejy5o4p7bderaaia4bdg2psxbfzf44yya5uutge"`
	Name string `json:"name" example:"Template Name"`
	Icon string `json:"icon" example:"📄"`
}
