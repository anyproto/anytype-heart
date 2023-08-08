//go:build !nogrpcserver && noauth
// +build !nogrpcserver,noauth

package main

import (
	//nolint: gosec
	_ "net/http/pprof"
)

func init() {
	localAPIAuthDisabled = true
}
