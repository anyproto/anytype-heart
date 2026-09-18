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

// apiAppNameCtxKey is the private carrier type for the authenticated key's
// app name; a typed key cannot collide with other context values.
type apiAppNameCtxKey struct{}

// CtxWithApiAppName stores the authenticated key's app name on the context.
// The auth middleware calls it on the REQUEST context (not only the gin
// context): NewAnalyticsEventForApi only ever sees a context.Context, and
// gin's c.Set values are not reachable through c.Request.Context().
func CtxWithApiAppName(ctx context.Context, appName string) context.Context {
	return context.WithValue(ctx, apiAppNameCtxKey{}, appName)
}

// NewAnalyticsEventForApi creates a new analytics event for api with the app name from the context
func NewAnalyticsEventForApi(ctx context.Context, code string, status int) (string, error) {
	apiAppName, ok := ctx.Value(apiAppNameCtxKey{}).(string)
	if !ok || apiAppName == "" {
		// unauthenticated routes (e.g. the auth flow itself) have no app name
		apiAppName = "api-app"
	}
	return NewAnalyticsEvent(code, "api", apiAppName, status).ToJSON()
}
