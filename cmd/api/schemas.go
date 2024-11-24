package api

type Space struct {
	Type                   string `json:"type" example:"space"`
	ID                     string `json:"id" example:"bafyreigyfkt6rbv24sbv5aq2hko3bhmv5xxlf22b4bypdu6j7hnphm3psq.23me69r569oi1"`
	Name                   string `json:"name" example:"Space Name"`
	Icon                   string `json:"icon" example:"data:image/png;base64, <base64-encoded-image>"`
	HomeObjectID           string `json:"home_object_id" example:"bafyreie4qcl3wczb4cw5hrfyycikhjyh6oljdis3ewqrk5boaav3sbwqya"`
	ArchiveObjectID        string `json:"archive_object_id" example:"bafyreialsgoyflf3etjm3parzurivyaukzivwortf32b4twnlwpwocsrri"`
	ProfileObjectID        string `json:"profile_object_id" example:"bafyreiaxhwreshjqwndpwtdsu4mtihaqhhmlygqnyqpfyfwlqfq3rm3gw4"`
	MarketplaceWorkspaceID string `json:"marketplace_workspace_id" example:"_anytype_marketplace"`
	DeviceID               string `json:"device_id" example:"12D3KooWGZMJ4kQVyQVXaj7gJPZr3RZ2nvd9M2Eq2pprEoPih9WF"`
	AccountSpaceID         string `json:"account_space_id" example:"bafyreihpd2knon5wbljhtfeg3fcqtg3i2pomhhnigui6lrjmzcjzep7gcy.23me69r569oi1"`
	WidgetsID              string `json:"widgets_id" example:"bafyreialj7pceh53mifm5dixlho47ke4qjmsn2uh4wsjf7xq2pnlo5xfva"`
	SpaceViewID            string `json:"space_view_id" example:"bafyreigzv3vq7qwlrsin6njoduq727ssnhwd6bgyfj6nm4hv3pxoc2rxhy"`
	TechSpaceID            string `json:"tech_space_id" example:"bafyreif4xuwncrjl6jajt4zrrfnylpki476nv2w64yf42ovt7gia7oypii.23me69r569oi1"`
	Timezone               string `json:"timezone" example:""`
	NetworkID              string `json:"network_id" example:"N83gJpVd9MuNRZAuJLZ7LiMntTThhPc6DtzWWVjb1M3PouVU"`
}

type SpaceMember struct {
	Type     string `json:"type" example:"space_member"`
	ID       string `json:"id" example:"_participant_bafyreigyfkt6rbv24sbv5aq2hko1bhmv5xxlf22b4bypdu6j7hnphm3psq_23me69r569oi1_AAjEaEwPF4nkEh9AWkqEnzcQ8HziBB4ETjiTpvRCQvWnSMDZ"`
	Name     string `json:"name" example:"Space Member Name"`
	Identity string `json:"identity" example:"AAjEaEwPF4nkEh7AWkqEnzcQ8HziGB4ETjiTpvRCQvWnSMDZ"`
	Role     string `json:"role" enum:"editor,viewer,owner" example:"editor"`
}

type Object struct {
	Type       string   `json:"type" example:"object"`
	ID         string   `json:"id" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"`
	Name       string   `json:"name" example:"Object Name"`
	IconEmoji  string   `json:"icon_emoji" example:"📄"`
	ObjectType string   `json:"object_type" example:"Page"`
	SpaceID    string   `json:"space_id" example:"bafyreigyfkt6rbv24sbv5aq2hko3bhmv5xxlf22b4bypdu6j7hnphm3psq.23me69r569oi1"`
	RootID     string   `json:"root_id"`
	Blocks     []Block  `json:"blocks"`
	Details    []Detail `json:"details"`
}

type Block struct {
	ID              string   `json:"id"`
	ChildrenIDs     []string `json:"children_ids"`
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
	Text      string `json:"text"`
	Style     string `json:"style"`
	Checked   bool   `json:"checked"`
	Color     string `json:"color"`
	IconEmoji string `json:"icon_emoji"`
	IconImage string `json:"icon_image"`
}

type File struct {
	Hash           string `json:"hash"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	Mime           string `json:"mime"`
	Size           int    `json:"size"`
	AddedAt        int    `json:"added_at"`
	TargetObjectID string `json:"target_object_id"`
	State          int    `json:"state"`
	Style          int    `json:"style"`
}

type Detail struct {
	ID      string                 `json:"id"`
	Details map[string]interface{} `json:"details"`
}

type RelationLink struct {
	Key    string `json:"key"`
	Format string `json:"format"`
}

type ObjectType struct {
	Type      string `json:"type" example:"object_type"`
	ID        string `json:"id" example:"bafyreigyb6l5szohs32ts26ku2j42yd65e6hqy2u3gtzgdwqv6hzftsetu"`
	UniqueKey string `json:"unique_key" example:"ot-page"`
	Name      string `json:"name" example:"Page"`
	IconEmoji string `json:"icon_emoji" example:"📄"`
}

type ObjectTemplate struct {
	Type      string `json:"type" example:"object_template"`
	ID        string `json:"id" example:"bafyreictrp3obmnf6dwejy5o4p7bderaaia4bdg2psxbfzf44yya5uutge"`
	Name      string `json:"name" example:"Object Template Name"`
	IconEmoji string `json:"icon_emoji" example:"📄"`
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
