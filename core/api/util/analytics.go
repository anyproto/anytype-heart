package util

import (
	"encoding/json"
	"log"

	"golang.org/x/net/context"
)

type AnalyticsBroadcastEvent struct {
	Type  string `json:"type"`
	Code  string `json:"code"`
	Param struct {
		Origin     string `json:"origin"`
		ApiAppName string `json:"apiAppName"`
	}
}

// ToJSON returns the event as a JSON string
func (e *AnalyticsBroadcastEvent) ToJSON() string {
	eventJSON, err := json.Marshal(e)
	if err != nil {
		log.Println("Error marshaling event:", err)
		return "{}"
	}
	return string(eventJSON)
}

// NewAnalyticsEvent creates a new analytics event with the given code, origin and apiAppName
func NewAnalyticsEvent(code, origin, apiAppName string) *AnalyticsBroadcastEvent {
	return &AnalyticsBroadcastEvent{
		Type: "analyticsEvent",
		Code: code,
		Param: struct {
			Origin     string `json:"origin"`
			ApiAppName string `json:"apiAppName"`
		}{
			Origin:     origin,
			ApiAppName: apiAppName,
		},
	}
}

// NewAnalyticsEventForApi creates a new analytics event for api with app name from the context
func NewAnalyticsEventForApi(ctx context.Context, code string) string {
	// TODO: enable when apiAppName is available in context
	// apiAppName := ctx.Value("apiAppName").(string)
	apiAppName := "api-app"
	return NewAnalyticsEvent(code, "api", apiAppName).ToJSON()
}
