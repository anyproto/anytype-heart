package deps

import (
	_ "github.com/golang/mock/sample"
	_ "github.com/vektra/mockery/v2/cmd"
	// _ "github.com/ahmetb/govvv"  // import govvv so it can be installed with make setup-go
	_ "golang.org/x/mobile/bind" // import gomobile so it will be installed with make setup-gomobile
)
