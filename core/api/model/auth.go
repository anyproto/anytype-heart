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
	// ApiKey is minted in the prefixed+checksummed format
	// `anytype_<body>_<checksum>`; match it with the published pattern
	// `\banytype_[0-9A-Za-z]{40,60}_[0-9a-f]{8}\b` (a length RANGE — never
	// assume a fixed length). Keys issued before the format flip are plain
	// base64 and keep authenticating unchanged.
	ApiKey string `json:"api_key" example:"anytype_amfbcga7eywtio2cjfifoxtfnrzxvamir6lj3jflwk44br6o2xoa_3fe1d4b7"` // The api key used to authenticate requests
}
