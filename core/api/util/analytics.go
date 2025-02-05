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

func (e *AnalyticsBroadcastEvent) ToJSON() string {
	eventJSON, err := json.Marshal(e)
	if err != nil {
		log.Println("Error marshaling event:", err)
		return "{}"
	}
	return string(eventJSON)
}

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

func NewAnalyticsEventForApi(ctx context.Context, code string) string {
	apiAppName := ctx.Value("apiAppName").(string)
	return NewAnalyticsEvent(code, "api", apiAppName).ToJSON()
}
