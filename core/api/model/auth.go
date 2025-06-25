package apimodel

// TO BE DEPRECATED
type DisplayCodeResponse struct {
	ChallengeId string `json:"challenge_id" example:"67647f5ecda913e9a2e11b26"` // The challenge id associated with the displayed code and needed to solve the challenge for token
}

// TO BE DEPRECATED
type TokenResponse struct {
	AppKey string `json:"app_key" example:"zhSG/zQRmgADyilWPtgdnfo1qD60oK02/SVgi1GaFt6="` // The app key used to authenticate requests
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
	ApiKey string `json:"api_key" example:"zhSG/zQRmgADyilWPtgdnfo1qD60oK02/SVgi1GaFt6="` // The api key used to authenticate requests
}
