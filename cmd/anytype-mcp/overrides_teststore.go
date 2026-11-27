//go:build teststore

package main

import "flag"

// overrides exist only in the test build (tag teststore).
type overrides struct {
	storeDir string
	lockDir  string
	port     int
	tier     string
}

func (o *overrides) register(fs *flag.FlagSet, serve bool) {
	fs.StringVar(&o.storeDir, "store-dir", "", "test build: directory of the file store")
	fs.StringVar(&o.lockDir, "lock-dir", "", "test build: directory of the verb locks")
	if serve {
		fs.IntVar(&o.port, "port", 0, "test build: override the item's port")
		fs.StringVar(&o.tier, "tier", "", "test build: override the item's tier")
	}
}
