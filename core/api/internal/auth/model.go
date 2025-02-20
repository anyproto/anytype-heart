package auth

type DisplayCodeResponse struct {
	ChallengeId string `json:"challenge_id" example:"67647f5ecda913e9a2e11b26"` // The challenge id associated with the displayed code and needed to solve the challenge for token
}

type TokenResponse struct {
	SessionToken string `json:"session_token" example:"eyJhbGciOeJIRzI1NiIsInR5cCI6IkpXVCJ1.eyJzZWVkIjaiY0dmVndlUnAifQ.Y1EZecYnwmvMkrXKOa2XJnAbaRt34urBabe06tmDQII"` // The ephemeral session token
	AppKey       string `json:"app_key" example:"zhSG/zQRmgADyilWPtgdnfo1qD60oK02/SVgi1GaFt6="`                                                                      // The permanent app key
}
