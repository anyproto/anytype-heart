package api

type AuthDisplayCodeResponse struct {
	ChallengeId string `json:"challenge_id" example:"67647f5ecda913e9a2e11b26"`
}

type AuthTokenResponse struct {
	SessionToken string `json:"session_token" example:""`
	AppKey       string `json:"app_key" example:""`
}

type Space struct {
	Type                   string `json:"type" example:"space"`
	Id                     string `json:"id" example:"bafyreigyfkt6rbv24sbv5aq2hko3bhmv5xxlf22b4bypdu6j7hnphm3psq.23me69r569oi1"`
	Name                   string `json:"name" example:"Space Name"`
	Icon                   string `json:"icon" example:"http://127.0.0.1:31006/image/bafybeieptz5hvcy6txplcvphjbbh5yjc2zqhmihs3owkh5oab4ezauzqay?width=100"`
	HomeObjectId           string `json:"home_object_id" example:"bafyreie4qcl3wczb4cw5hrfyycikhjyh6oljdis3ewqrk5boaav3sbwqya"`
	ArchiveObjectId        string `json:"archive_object_id" example:"bafyreialsgoyflf3etjm3parzurivyaukzivwortf32b4twnlwpwocsrri"`
	ProfileObjectId        string `json:"profile_object_id" example:"bafyreiaxhwreshjqwndpwtdsu4mtihaqhhmlygqnyqpfyfwlqfq3rm3gw4"`
	MarketplaceWorkspaceId string `json:"marketplace_workspace_id" example:"_anytype_marketplace"`
	WorkspaceObjectId      string `json:"workspace_object_id" example:"bafyreiapey2g6e6za4zfxvlgwdy4hbbfu676gmwrhnqvjbxvrchr7elr3y"`
	DeviceId               string `json:"device_id" example:"12D3KooWGZMJ4kQVyQVXaj7gJPZr3RZ2nvd9M2Eq2pprEoPih9WF"`
	AccountSpaceId         string `json:"account_space_id" example:"bafyreihpd2knon5wbljhtfeg3fcqtg3i2pomhhnigui6lrjmzcjzep7gcy.23me69r569oi1"`
	WidgetsId              string `json:"widgets_id" example:"bafyreialj7pceh53mifm5dixlho47ke4qjmsn2uh4wsjf7xq2pnlo5xfva"`
	SpaceViewId            string `json:"space_view_id" example:"bafyreigzv3vq7qwlrsin6njoduq727ssnhwd6bgyfj6nm4hv3pxoc2rxhy"`
	TechSpaceId            string `json:"tech_space_id" example:"bafyreif4xuwncrjl6jajt4zrrfnylpki476nv2w64yf42ovt7gia7oypii.23me69r569oi1"`
	Timezone               string `json:"timezone" example:""`
	NetworkId              string `json:"network_id" example:"N83gJpVd9MuNRZAuJLZ7LiMntTThhPc6DtzWWVjb1M3PouVU"`
}

type SpaceMember struct {
	Type       string `json:"type" example:"space_member"`
	Id         string `json:"id" example:"_participant_bafyreigyfkt6rbv24sbv5aq2hko1bhmv5xxlf22b4bypdu6j7hnphm3psq_23me69r569oi1_AAjEaEwPF4nkEh9AWkqEnzcQ8HziBB4ETjiTpvRCQvWnSMDZ"`
	Name       string `json:"name" example:"John Doe"`
	Icon       string `json:"icon" example:"http://127.0.0.1:31006/image/bafybeieptz5hvcy6txplcvphjbbh5yjc2zqhmihs3owkh5oab4ezauzqay?width=100"`
	Identity   string `json:"identity" example:"AAjEaEwPF4nkEh7AWkqEnzcQ8HziGB4ETjiTpvRCQvWnSMDZ"`
	GlobalName string `json:"global_name" example:"john.any"`
	Role       string `json:"role" enum:"Reader,Writer,Owner,NoPermission" example:"Owner"`
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
	Layout          Layout   `json:"layout"`
	Text            Text     `json:"text"`
	File            File     `json:"file"`
}

type Layout struct {
	Style string `json:"style"`
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
	State          int    `json:"state"`
	Style          int    `json:"style"`
}

type Detail struct {
	Id      string                 `json:"id"`
	Details map[string]interface{} `json:"details"`
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

type ChatMessage struct {
	Type             string         `json:"chat_message"`
	Id               string         `json:"id"`       // Unique message identifier
	OrderId          string         `json:"order_id"` // Used for subscriptions
	Creator          string         `json:"creator"`  // Identifier for the message creator
	CreatedAt        int64          `json:"created_at"`
	ModifiedAt       int64          `json:"modified_at"`
	ReplyToMessageId string         `json:"reply_to_message_id"` // Identifier for the message being replied to
	Message          MessageContent `json:"message"`             // Message content
	Attachments      []Attachment   `json:"attachments"`         // Attachments slice
	Reactions        Reactions      `json:"reactions"`           // Reactions to the message
}

type MessageContent struct {
	Text  string   `json:"text"`  // The text content of the message part
	Style string   `json:"style"` // The style/type of the message part
	Marks []string `json:"marks"` // List of marks applied to the text
}

type Attachment struct {
	Target string `json:"target"` // Identifier for the attachment object
	Type   string `json:"type"`   // Type of attachment
}

type Reactions struct {
	ReactionsMap map[string]IdentityList `json:"reactions"` // Map of emoji to list of user IDs
}

type IdentityList struct {
	Ids []string `json:"ids"` // List of user IDs
}

type ServerError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

type ValidationError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

type UnauthorizedError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

type NotFoundError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}
