//go:build envnetworkcustom

package config

import _ "embed"

//go:embed nodes/custom.yml
var nodesConfYmlBytes []byte
