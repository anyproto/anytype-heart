//go:build envproduction

package config

import _ "embed"

//go:embed nodes/production.yml
var nodesConfYmlBytes []byte
