//go:build !nogrpcserver && !_test && windows
// +build !nogrpcserver,!_test,windows

package main

func ignoreBrokenPipeSignal() {}
