//go:build !teststore

package main

import "flag"

// overrides are empty in every shipped binary: serve takes its port and
// tier from the item and nothing else, and no directory can be redirected.
type overrides struct {
	storeDir string
	lockDir  string
	port     int
	tier     string
}

func (*overrides) register(*flag.FlagSet, bool) {}
