//go:build !envdev && !envproduction

package config

import _ "embed"

//go:embed nodes/staging.yml
var nodesConfYmlBytes []byte
