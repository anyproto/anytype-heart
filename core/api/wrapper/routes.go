package wrapper

// routes.go — the /v2 route templates the tools call, stated once so the
// server suite can assert every one is registered (core/api/server's
// TestWrapperRoutesRegistered): the wrapper's own tests stub these paths
// themselves, so without the server-side check a renamed route would leave
// the wrapper suite green while every tool 404s in production.

// RouteTemplate is one method + gin route template the wrapper depends on.
type RouteTemplate struct {
	Method string
	Path   string
}

// RouteTemplates lists every server route the tool executors call.
func RouteTemplates() []RouteTemplate {
	return []RouteTemplate{
		{"GET", "/v2/spaces"},                                   // spaces
		{"POST", "/v2/spaces/:space_id/search"},                 // find
		{"GET", "/v2/spaces/:space_id/objects/:object_id"},      // read, the ambiguity re-read
		{"PATCH", "/v2/spaces/:space_id/objects/:object_id"},    // every editing tool
		{"POST", "/v2/spaces/:space_id/objects"},                // create
		{"GET", "/v2/spaces/:space_id/types/:type"},             // describe
		{"GET", "/v2/spaces/:space_id/types"},                   // the type-name fold, create's receipt label
		{"POST", "/v2/spaces/:space_id/types"},                  // create_type
		{"GET", "/v2/spaces/:space_id/properties"},              // the format index
		{"POST", "/v2/spaces/:space_id/properties"},             // create_type's option-bearing properties
		{"GET", "/v2/spaces/:space_id/properties/:key/options"}, // the A2 guard, describe
		{"GET", "/v2/spaces/:space_id/members/me"},              // @me resolution
	}
}
