//go:build envdev

package config

import _ "embed"

//go:embed nodes/local.yml
var nodesConfYmlBytes []byte
