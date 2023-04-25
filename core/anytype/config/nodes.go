//go:build !localnode

package config

import _ "embed"

//go:embed nodes/staging.yml
var nodesConfYmlBytes []byte
