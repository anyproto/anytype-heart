package apimodel

// TO BE DEPRECATED
type DisplayCodeResponse struct {
	ChallengeId string `json:"challenge_id" example:"67647f5ecda913e9a2e11b26"` // The challenge id associated with the displayed code and needed to solve the challenge for token
}

// TO BE DEPRECATED
type TokenResponse struct {
	AppKey string `json:"app_key" example:"anytype_amfbcga7eywtio2cjfifoxtfnrzxvamir6lj3jflwk44br6o2xoa_3fe1d4b7"` // The app key used to authenticate requests
}

type CreateChallengeRequest struct {
	AppName string `json:"app_name" example:"anytype_mcp"` // The name of the app that is requesting the challenge
}

type CreateChallengeResponse struct {
	ChallengeId string `json:"challenge_id" example:"67647f5ecda913e9a2e11b26"` // The challenge id associated with the displayed code and needed to solve the challenge for api_key
}

type CreateApiKeyRequest struct {
	ChallengeId string `json:"challenge_id" example:"67647f5ecda913e9a2e11b26"` // The challenge id associated with the previously displayed code
	Code        string `json:"code" example:"1234"`                             // The 4-digit code retrieved from Anytype Desktop app
}

type CreateApiKeyResponse struct {
	// ApiKey is an opaque bearer key in the format `anytype_<body>_<checksum>`.
	// New keys match `\banytype_[0-9A-Za-z]{40,60}_[0-9a-f]{8}\b`; the body
	// length varies. Previously issued unprefixed base64 keys remain valid.
	ApiKey string       `json:"api_key" example:"anytype_amfbcga7eywtio2cjfifoxtfnrzxvamir6lj3jflwk44br6o2xoa_3fe1d4b7"` // The api key used to authenticate requests
	Grant  *ApiKeyGrant `json:"grant"`                                                                                   // The grant approved by the user and persisted with this key; null for an unscoped key.
}

// ApiKeyGrant is the approved access boundary of an issued API key.
type ApiKeyGrant struct {
	AllSpaces  bool     `json:"all_spaces"`                        // Covers all current and future user spaces when true; space_ids is then empty.
	SpaceIds   []string `json:"space_ids"`                         // Full IDs of the granted spaces. An empty list alone never means all spaces.
	Permission string   `json:"permission" enums:"read,readwrite"` // The access approved by the user, which may differ from what the app requested.
}
