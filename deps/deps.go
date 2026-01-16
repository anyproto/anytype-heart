//go:build deps
// +build deps

package deps

import (
	_ "github.com/ahmetb/govvv"
	_ "github.com/awalterschulze/goderive/derive"
	_ "github.com/pseudomuto/protoc-gen-doc/extensions/google_api_http"
	_ "github.com/vektra/mockery/v2/cmd"
	_ "golang.org/x/mobile/bind" // import gomobile so it will be installed with make setup-gomobile
)
