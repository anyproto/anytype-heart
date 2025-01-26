package space

type CreateSpaceRequest struct {
	Name string `json:"name"`
}

type CreateSpaceResponse struct {
	Space Space `json:"space"`
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
	GatewayUrl             string `json:"gateway_url" example:"http://127.0.0.1:31006"`
	LocalStoragePath       string `json:"local_storage_path" example:"/Users/johndoe/Library/Application Support/Anytype/data/AAHTtt1wuQEnaYBNZ2Cyfcvs6DqPqxgn8VXDVk4avsUkMuha"`
	Timezone               string `json:"timezone" example:""`
	AnalyticsId            string `json:"analytics_id" example:"624aecdd-4797-4611-9d61-a2ae5f53cf1c"`
	NetworkId              string `json:"network_id" example:"N83gJpVd9MuNRZAuJLZ7LiMntTThhPc6DtzWWVjb1M3PouVU"`
}

type Member struct {
	Type       string `json:"type" example:"member"`
	Id         string `json:"id" example:"_participant_bafyreigyfkt6rbv24sbv5aq2hko1bhmv5xxlf22b4bypdu6j7hnphm3psq_23me69r569oi1_AAjEaEwPF4nkEh9AWkqEnzcQ8HziBB4ETjiTpvRCQvWnSMDZ"`
	Name       string `json:"name" example:"John Doe"`
	Icon       string `json:"icon" example:"http://127.0.0.1:31006/image/bafybeieptz5hvcy6txplcvphjbbh5yjc2zqhmihs3owkh5oab4ezauzqay?width=100"`
	Identity   string `json:"identity" example:"AAjEaEwPF4nkEh7AWkqEnzcQ8HziGB4ETjiTpvRCQvWnSMDZ"`
	GlobalName string `json:"global_name" example:"john.any"`
	Role       string `json:"role" enum:"Reader,Writer,Owner,NoPermission" example:"Owner"`
}
