//go:build !nogrpcserver && !_test && !windows
// +build !nogrpcserver,!_test,!windows

package main

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
)

const lifelineSIGPIPEChildEnv = "ANYTYPE_LIFELINE_SIGPIPE_TEST_CHILD"

func TestIgnoreBrokenPipeSignal(t *testing.T) {
	if os.Getenv(lifelineSIGPIPEChildEnv) == "1" {
		runBrokenPipeTestChild()
		return
	}

	child := exec.Command(os.Args[0], "-test.run=^TestIgnoreBrokenPipeSignal$")
	child.Env = append(os.Environ(), lifelineSIGPIPEChildEnv+"=1")
	if err := child.Run(); err != nil {
		t.Fatalf("process terminated while writing to closed stdout: %v", err)
	}
}

func runBrokenPipeTestChild() {
	ignoreBrokenPipeSignal()
	reader, writer, err := os.Pipe()
	if err != nil {
		os.Exit(2)
	}
	_ = reader.Close()
	if err = syscall.Dup2(int(writer.Fd()), int(os.Stdout.Fd())); err != nil {
		os.Exit(2)
	}
	_ = writer.Close()

	if _, err = os.Stdout.Write([]byte("closed pipe")); err == nil {
		os.Exit(3)
	}
	os.Exit(0)
}
