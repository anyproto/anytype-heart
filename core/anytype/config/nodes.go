//go:build !localnode

package config

import _ "embed"

//go:embed nodes/nodes.yml
var nodesConfYmlBytes []byte
