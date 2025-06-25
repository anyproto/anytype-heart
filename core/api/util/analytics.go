package util

import (
	"context"
	"encoding/json"
	"fmt"
)

type AnalyticsBroadcastEvent struct {
	Type  string `json:"type"`
	Code  string `json:"code"`
	Param struct {
		Route      string `json:"route"`
		ApiAppName string `json:"apiAppName"`
		Status     int    `json:"status"`
	} `json:"param"`
}

// ToJSON returns the event as a JSON string
func (e *AnalyticsBroadcastEvent) ToJSON() (string, error) {
	eventJSON, err := json.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("error marshalling analytics event: %w", err)
	}
	return string(eventJSON), nil
}

// NewAnalyticsEvent creates a new analytics event with the given code, route and apiAppName
func NewAnalyticsEvent(code, route, apiAppName string, status int) *AnalyticsBroadcastEvent {
	return &AnalyticsBroadcastEvent{
		Type: "analyticsEvent",
		Code: code,
		Param: struct {
			Route      string `json:"route"`
			ApiAppName string `json:"apiAppName"`
			Status     int    `json:"status"`
		}{
			Route:      route,
			ApiAppName: apiAppName,
			Status:     status,
		},
	}
}

// NewAnalyticsEventForApi creates a new analytics event for api with the app name from the context
func NewAnalyticsEventForApi(ctx context.Context, code string, status int) (string, error) {
	// TODO: enable when apiAppName is available in context
	// apiAppName := ctx.Value("apiAppName").(string)
	apiAppName := "api-app"
	return NewAnalyticsEvent(code, "api", apiAppName, status).ToJSON()
}
