package auth

type DisplayCodeResponse struct {
	ChallengeId string `json:"challenge_id" example:"67647f5ecda913e9a2e11b26"`
}

type TokenResponse struct {
	SessionToken string `json:"session_token" example:""`
	AppKey       string `json:"app_key" example:""`
}
