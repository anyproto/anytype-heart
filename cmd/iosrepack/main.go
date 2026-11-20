// Command iosrepack merges the tantivy static library into the Lib.xcframework that gomobile bind
// wrote to dist/ios. Every slice of the framework (the device one, the simulator one) holds one Lib
// with one or more architectures; for each architecture it combines the Go objects with the objects
// of the tantivy library built for that platform and architecture, then reassembles the slice's Lib
// with lipo. It works from the slices and architectures the framework has, so a framework without
// the x86_64 simulator is repacked the same way as one with it.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	xcframework = "dist/ios/Lib.xcframework"
	repack      = "repack"
	combined    = "libcombined.a"
)

// tantivyLibs maps a slice kind and an architecture to the tantivy library built for them
var tantivyLibs = map[string]string{
	"device/arm64":     "deps/libs/ios-arm64/libtantivy_go.a",
	"simulator/arm64":  "deps/libs/ios-arm64-sim/libtantivy_go.a",
	"simulator/x86_64": "deps/libs/ios-amd64/libtantivy_go.a",
}

func main() {
	if err := run(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(-1)
	}
}

func run() error {
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(filepath.Join(root, repack)) }()

	slices, err := filepath.Glob(filepath.Join(root, xcframework, "ios-*"))
	if err != nil {
		return fmt.Errorf("list the slices of %s: %w", xcframework, err)
	}
	if len(slices) == 0 {
		return fmt.Errorf("no slices in %s", xcframework)
	}
	for _, slice := range slices {
		if err := repackSlice(root, slice); err != nil {
			return fmt.Errorf("repack %s: %w", filepath.Base(slice), err)
		}
	}
	return nil
}

func repackSlice(root, slice string) error {
	kind := "device"
	if strings.HasSuffix(filepath.Base(slice), "-simulator") {
		kind = "simulator"
	}
	lib := filepath.Join(slice, "Lib.framework", "Lib")

	archs, err := output("lipo", "-archs", lib)
	if err != nil {
		return fmt.Errorf("read the architectures of %s: %w", lib, err)
	}
	fat, err := output("lipo", "-info", lib)
	if err != nil {
		return fmt.Errorf("read %s: %w", lib, err)
	}

	var repacked []string
	for _, arch := range strings.Fields(archs) {
		tantivy, ok := tantivyLibs[kind+"/"+arch]
		if !ok {
			return fmt.Errorf("no tantivy library for %s %s", kind, arch)
		}
		dir := filepath.Join(root, repack, filepath.Base(slice), arch)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}

		goLib := filepath.Join(dir, "go.a")
		if strings.HasPrefix(fat, "Non-fat file") {
			err = execute("", "cp", lib, goLib)
		} else {
			err = execute("", "lipo", lib, "-thin", arch, "-output", goLib)
		}
		if err != nil {
			return fmt.Errorf("extract the %s objects: %w", arch, err)
		}

		// The tantivy objects first, then the Go ones, both into the same directory
		if err := execute(dir, "ar", "-x", filepath.Join(root, tantivy)); err != nil {
			return fmt.Errorf("extract %s: %w", tantivy, err)
		}
		if err := execute(dir, "ar", "-x", goLib); err != nil {
			return fmt.Errorf("extract the Go library: %w", err)
		}
		objects, err := filepath.Glob(filepath.Join(dir, "*.o"))
		if err != nil {
			return fmt.Errorf("list the objects: %w", err)
		}
		for i := range objects {
			objects[i] = filepath.Base(objects[i])
		}
		if err := execute(dir, "ar", append([]string{"-qc", combined}, objects...)...); err != nil {
			return fmt.Errorf("combine the %s objects: %w", arch, err)
		}
		repacked = append(repacked, filepath.Join(dir, combined))
	}

	newLib := filepath.Join(root, repack, filepath.Base(slice), "Lib")
	if err := execute("", "lipo", append(append([]string{"-create"}, repacked...), "-output", newLib)...); err != nil {
		return fmt.Errorf("create the repacked library: %w", err)
	}
	if err := os.Rename(newLib, lib); err != nil {
		return fmt.Errorf("replace %s: %w", lib, err)
	}
	return nil
}

// execute runs a command in dir (the working directory when empty), with its output shown
func execute(dir, name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(arg, " "), err)
	}
	return nil
}

// output runs a command and returns its trimmed standard output
func output(name string, arg ...string) (string, error) {
	out, err := exec.Command(name, arg...).Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(arg, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}
